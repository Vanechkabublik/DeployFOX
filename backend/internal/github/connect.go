package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/jackc/pgx/v5"
)

var installationPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)

type Connection struct {
	ID      string `json:"id"`
	Account string `json:"account"`
}

func TokenFromStore(ctx context.Context, s *platform.Store) (string, error) {
	var raw []byte
	err := s.DB.QueryRow(ctx, "SELECT value FROM settings WHERE key='github.active'").Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return InstallationToken(ctx)
	}
	if err != nil {
		return "", err
	}
	var connected Connection
	if err = json.Unmarshal(raw, &connected); err != nil {
		return "", err
	}
	return installationToken(ctx, connected.ID)
}
func (h Handler) Connection(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"configured": os.Getenv("GITHUB_APP_ID") != "" && os.Getenv("GITHUB_PRIVATE_KEY_PATH") != "", "slug": os.Getenv("GITHUB_APP_SLUG"), "active": nil, "pending": nil}
	rows, err := h.Store.DB.Query(r.Context(), "SELECT key,value FROM settings WHERE key IN ('github.active','github.pending')")
	if err != nil {
		auth.Error(w, 500, "Cannot load GitHub connection")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if rows.Scan(&key, &raw) != nil {
			auth.Error(w, 500, "Cannot read connection")
			return
		}
		var c Connection
		if json.Unmarshal(raw, &c) != nil {
			auth.Error(w, 500, "Invalid connection state")
			return
		}
		if key == "github.active" {
			result["active"] = c
		} else {
			result["pending"] = c
		}
	}
	if rows.Err() != nil {
		auth.Error(w, 500, "Cannot read connection")
		return
	}
	auth.JSON(w, 200, result)
}
func (h Handler) Connect(w http.ResponseWriter, r *http.Request) {
	slug := os.Getenv("GITHUB_APP_SLUG")
	if !slugPattern.MatchString(slug) || os.Getenv("GITHUB_APP_ID") == "" {
		auth.Error(w, 503, "Configure GITHUB_APP_SLUG, App ID and private key first")
		return
	}
	if _, err := appJWT(); err != nil {
		auth.Error(w, 503, "GitHub App private key is unavailable or invalid")
		return
	}
	state := security.Token()
	if err := h.Store.Redis.Set(r.Context(), "github:connect:"+security.Hash(state), "pending", 10*time.Minute).Err(); err != nil {
		auth.Error(w, 503, "Connection state unavailable")
		return
	}
	auth.JSON(w, 200, map[string]string{"url": "https://github.com/apps/" + slug + "/installations/new?state=" + state})
}
func verifyInstallation(ctx context.Context, id string) (Connection, error) {
	if !installationPattern.MatchString(id) {
		return Connection{}, errors.New("invalid installation")
	}
	jwt, err := appJWT()
	if err != nil {
		return Connection{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/app/installations/"+id, nil)
	if err != nil {
		return Connection{}, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Connection{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Connection{}, fmt.Errorf("installation does not belong to the configured GitHub App")
	}
	var data struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&data); err != nil {
		return Connection{}, err
	}
	if strconv.FormatInt(data.ID, 10) != id || data.Account.Login == "" {
		return Connection{}, errors.New("invalid installation response")
	}
	return Connection{ID: id, Account: data.Account.Login}, nil
}
func (h Handler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if len(state) != 64 {
		auth.Error(w, 403, "Invalid or expired connection state")
		return
	}
	if value, err := h.Store.Redis.GetDel(r.Context(), "github:connect:"+security.Hash(state)).Result(); err != nil || value != "pending" {
		auth.Error(w, 403, "Invalid or expired connection state")
		return
	}
	connection, err := verifyInstallation(r.Context(), r.URL.Query().Get("installation_id"))
	if err != nil {
		auth.Error(w, 400, "Cannot verify this GitHub App installation")
		return
	}
	data, err := json.Marshal(connection)
	if err != nil {
		auth.Error(w, 500, "Cannot encode connection")
		return
	}
	if _, err = h.Store.DB.Exec(r.Context(), "INSERT INTO settings(key,value) VALUES('github.pending',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=now()", data); err != nil {
		auth.Error(w, 500, "Cannot save pending installation")
		return
	}
	// The redirect alone never activates credentials. Local admin must confirm the verified account with CSRF.
	http.Redirect(w, r, h.Store.Config.Origin+"/?github=pending", http.StatusSeeOther)
}
func (h Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !auth.Decode(w, r, &body) {
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var raw []byte
	if err = tx.QueryRow(r.Context(), "SELECT value FROM settings WHERE key='github.pending' FOR UPDATE").Scan(&raw); err != nil {
		auth.Error(w, 409, "No pending installation")
		return
	}
	var pending Connection
	if json.Unmarshal(raw, &pending) != nil || pending.ID != body.ID {
		auth.Error(w, 409, "Pending installation changed; reload and verify the GitHub account")
		return
	}
	verified, err := verifyInstallation(r.Context(), pending.ID)
	if err != nil || verified.Account != pending.Account {
		auth.Error(w, 400, "Installation could not be reverified")
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO settings(key,value) VALUES('github.active',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=now()", raw); err == nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM settings WHERE key='github.pending'")
	}
	if err != nil {
		auth.Error(w, 500, "Cannot activate installation")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Cannot commit installation")
		return
	}
	h.Store.Audit(r.Context(), "github.connected", verified.Account)
	auth.JSON(w, 200, map[string]bool{"ok": true})
}
