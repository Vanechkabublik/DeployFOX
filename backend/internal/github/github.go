package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"deployfox/internal/security"
)

type Handler struct{ Store *platform.Store }

func appJWT() (string, error) {
	appID, keyPath := os.Getenv("GITHUB_APP_ID"), os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	if appID == "" || keyPath == "" {
		return "", errors.New("GitHub App is not configured")
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", errors.New("invalid GitHub App key")
	}
	var key *rsa.PrivateKey
	key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
		if e != nil {
			return "", e
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("RSA private key required")
		}
	}
	claims, _ := json.Marshal(map[string]any{"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(8 * time.Minute).Unix(), "iss": appID})
	enc := base64.RawURLEncoding.EncodeToString
	unsigned := enc([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + enc(signature), nil
}

func InstallationToken(ctx context.Context) (string, error) {
	return installationToken(ctx, os.Getenv("GITHUB_INSTALLATION_ID"))
}

func installationToken(ctx context.Context, installation string) (string, error) {
	if !installationPattern.MatchString(installation) {
		return "", errors.New("GitHub installation is not connected")
	}
	jwt, err := appJWT()
	if err != nil {
		return "", err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/app/installations/"+installation+"/access_tokens", nil)
	if err != nil {
		return "", err
	}
	r.Header.Set("Authorization", "Bearer "+jwt)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return "", fmt.Errorf("GitHub installation authentication failed: HTTP %d", resp.StatusCode)
	}
	var b struct {
		Token string `json:"token"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&b)
	return b.Token, err
}
func (h Handler) Repositories(w http.ResponseWriter, r *http.Request) {
	token, err := TokenFromStore(r.Context(), h.Store)
	if err != nil {
		auth.Error(w, 503, "Configure GitHub App ID, installation ID and private key on the server")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.github.com/installation/repositories?per_page=100", nil)
	if err != nil {
		auth.Error(w, 500, "Cannot create GitHub request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		auth.Error(w, 502, "GitHub unavailable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		auth.Error(w, 502, "GitHub rejected repository request")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}
func (h Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		auth.Error(w, 413, "Payload too large")
		return
	}
	if !security.Webhook(h.Store.Config.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		auth.Error(w, 401, "Invalid webhook signature")
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		auth.JSON(w, 200, map[string]bool{"ignored": true})
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	if delivery == "" || len(delivery) > 128 {
		auth.Error(w, 400, "Invalid delivery ID")
		return
	}
	var event struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			HTMLURL string `json:"html_url"`
		} `json:"repository"`
	}
	if json.Unmarshal(body, &event) != nil {
		auth.Error(w, 400, "Invalid payload")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), "INSERT INTO webhook_deliveries(id) VALUES($1) ON CONFLICT DO NOTHING", delivery)
	if err != nil {
		auth.Error(w, 500, "Delivery persistence failed")
		return
	}
	if tag.RowsAffected() == 0 {
		auth.JSON(w, 200, map[string]bool{"duplicate": true})
		return
	}
	if !event.Deleted && strings.HasPrefix(event.Ref, "refs/heads/") {
		_, err = tx.Exec(r.Context(), `UPDATE projects SET pending_commit=$3 WHERE auto_deploy AND source_type='git' AND regexp_replace(source,'\.git$','')=regexp_replace($1,'\.git$','') AND branch=$2`, event.Repository.HTMLURL, strings.TrimPrefix(event.Ref, "refs/heads/"), event.After)
		if err != nil {
			auth.Error(w, 500, "Cannot persist pending commit")
			return
		}
		_, err = tx.Exec(r.Context(), `WITH queued AS (INSERT INTO deployments(id,project_id,commit_sha) SELECT gen_random_uuid(),id,pending_commit FROM projects WHERE auto_deploy AND source_type='git' AND regexp_replace(source,'\.git$','')=regexp_replace($1,'\.git$','') AND branch=$2 ON CONFLICT DO NOTHING RETURNING project_id) UPDATE projects SET pending_commit='' WHERE id IN (SELECT project_id FROM queued)`, event.Repository.HTMLURL, strings.TrimPrefix(event.Ref, "refs/heads/"))
		if err != nil {
			auth.Error(w, 500, "Cannot queue deployment")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Delivery commit failed")
		return
	}
	h.Store.Wake(r.Context())
	auth.JSON(w, 202, map[string]bool{"accepted": true})
}
