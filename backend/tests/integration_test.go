package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"deployfox/internal/config"
	"deployfox/internal/deployments"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"deployfox/internal/server"
)

// Uses real PostgreSQL and Redis. Mock is only the explicitly selected local deployment provider.
func TestControlPlane(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required (real PostgreSQL)")
	}
	parsedDSN, parseErr := url.Parse(dsn)
	if parseErr != nil || parsedDSN.Path != "/deployfox_test" {
		t.Fatal("integration tests require a disposable deployfox_test database")
	}
	ctx := context.Background()
	c := config.Config{DatabaseURL: dsn, RedisURL: os.Getenv("TEST_REDIS_URL"), EncryptionKey: make([]byte, 32), SetupToken: strings.Repeat("s", 64), Origin: "http://localhost:3000", Provider: "mock", WebhookSecret: "integration-secret"}
	s, err := platform.Open(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.DB.Exec(ctx, "TRUNCATE administrators,sessions,projects,managed_databases,audit_events,webhook_deliveries,settings CASCADE"); err != nil {
		t.Fatal(err)
	}
	handler := server.Router(s)
	var cookie *http.Cookie
	csrf := ""
	runAddress := security.ID() + ":1000"
	request := func(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.RemoteAddr = runAddress
		r.Header.Set("Origin", c.Origin)
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request("GET", "/api/v1/projects", nil, nil).Code; got != 401 {
		t.Fatalf("unauthenticated projects: %d", got)
	}
	credentials := map[string]string{"email": "owner@example.com", "password": "correct horse battery staple"}
	if got := request("POST", "/api/v1/setup", credentials, nil).Code; got != 403 {
		t.Fatalf("setup without token: %d", got)
	}
	setup := request("POST", "/api/v1/setup", credentials, map[string]string{"X-Setup-Token": c.SetupToken})
	if setup.Code != 200 {
		t.Fatalf("setup: %s", setup.Body)
	}
	cookie = setup.Result().Cookies()[0]
	var session map[string]string
	json.Unmarshal(setup.Body.Bytes(), &session)
	csrf = session["csrf"]
	if got := request("POST", "/api/v1/setup/complete", nil, nil).Code; got != 409 {
		t.Fatalf("wizard completed without project: %d", got)
	}
	if got := request("GET", "/api/v1/github/callback?state="+strings.Repeat("0", 64)+"&installation_id=123", nil, nil).Code; got != 403 {
		t.Fatalf("forged GitHub callback accepted: %d", got)
	}
	if got := request("POST", "/api/v1/setup", credentials, map[string]string{"X-Setup-Token": c.SetupToken}).Code; got != 404 {
		t.Fatalf("second setup: %d", got)
	}
	p := map[string]any{"name": "test-project", "source_type": "image", "source": "nginx:alpine", "port": 80}
	if got := request("POST", "/api/v1/projects", p, map[string]string{"X-CSRF-Token": "wrong"}).Code; got != 403 {
		t.Fatalf("CSRF: %d", got)
	}
	created := request("POST", "/api/v1/projects", p, nil)
	if created.Code != 201 {
		t.Fatalf("create: %s", created.Body)
	}
	var project platform.Project
	json.Unmarshal(created.Body.Bytes(), &project)
	if got := request("POST", "/api/v1/setup/complete", nil, nil).Code; got != 200 {
		t.Fatalf("wizard completion: %d", got)
	}
	if got := request("POST", "/api/v1/github/confirm", map[string]string{"id": "123"}, nil).Code; got != 409 {
		t.Fatalf("unverified GitHub installation accepted: %d", got)
	}
	stored := request("PUT", "/api/v1/projects/"+project.ID+"/environment", map[string]any{"values": map[string]string{"SECRET": "integration-value"}}, nil)
	if stored.Code != 200 {
		t.Fatalf("environment: %s", stored.Body)
	}
	var encrypted string
	if err = s.DB.QueryRow(ctx, "SELECT encrypted_value FROM environment_variables WHERE project_id=$1", project.ID).Scan(&encrypted); err != nil || strings.Contains(encrypted, "integration-value") {
		t.Fatal("secret storage failed", err)
	}
	listed := request("GET", "/api/v1/projects/"+project.ID+"/environment", nil, nil)
	if strings.Contains(listed.Body.String(), "integration-value") {
		t.Fatal("secret disclosed")
	}
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- request("POST", "/api/v1/projects/"+project.ID+"/deploy", nil, nil).Code
		}()
	}
	wg.Wait()
	close(codes)
	accepted := 0
	for code := range codes {
		if code == 202 {
			accepted++
		} else if code != 409 {
			t.Errorf("concurrent queue returned %d", code)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d concurrent deploys", accepted)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- (deployments.Worker{Store: s}).Run(workerCtx) }()
	deadline := time.Now().Add(15 * time.Second)
	success := false
	for time.Now().Before(deadline) {
		var status string
		s.DB.QueryRow(ctx, "SELECT status FROM deployments WHERE project_id=$1", project.ID).Scan(&status)
		if status == "SUCCESS" {
			success = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done
	if !success {
		t.Fatal("mock lifecycle did not finish")
	}
	updated := request("PUT", "/api/v1/projects/"+project.ID, map[string]any{"memory_mb": 256, "domain": "reserved.example.org"}, nil)
	if updated.Code != 200 {
		t.Fatalf("project settings: %s", updated.Body)
	}
	updated = request("PUT", "/api/v1/projects/"+project.ID, map[string]any{"domain": "next.example.org"}, nil)
	if updated.Code != 200 {
		t.Fatalf("project domain update: %s", updated.Body)
	}
	conflict := request("POST", "/api/v1/projects", map[string]any{"name": "conflicting-domain", "source_type": "image", "source": "nginx:alpine", "domain": "reserved.example.org"}, nil)
	if conflict.Code != 409 {
		t.Fatalf("old domain reservation lost before deployment: %d", conflict.Code)
	}
	_, err = s.DB.Exec(ctx, "UPDATE projects SET source_type='git',source='https://github.com/owner/repo',auto_deploy=true WHERE id=$1", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"ref": "refs/heads/main", "after": strings.Repeat("a", 40), "repository": map[string]string{"html_url": "https://github.com/owner/repo"}}
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, []byte(c.WebhookSecret))
	mac.Write(body)
	headers := map[string]string{"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-GitHub-Event": "push", "X-GitHub-Delivery": "integration-delivery"}
	if got := request("POST", "/api/v1/github/webhook", payload, headers).Code; got != 202 {
		t.Fatalf("webhook: %d", got)
	}
	if got := request("POST", "/api/v1/github/webhook", payload, headers).Code; got != 200 {
		t.Fatalf("duplicate webhook: %d", got)
	}
	var count int
	s.DB.QueryRow(ctx, "SELECT count(*) FROM deployments WHERE project_id=$1", project.ID).Scan(&count)
	if count != 2 {
		t.Fatalf("webhook dedup: %d deployments", count)
	}
	payload["after"] = strings.Repeat("b", 40)
	body, _ = json.Marshal(payload)
	mac = hmac.New(sha256.New, []byte(c.WebhookSecret))
	mac.Write(body)
	headers["X-Hub-Signature-256"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	headers["X-GitHub-Delivery"] = "integration-pending-delivery"
	if got := request("POST", "/api/v1/github/webhook", payload, headers).Code; got != 202 {
		t.Fatalf("pending push: %d", got)
	}
	var pending string
	if err = s.DB.QueryRow(ctx, "SELECT pending_commit FROM projects WHERE id=$1", project.ID).Scan(&pending); err != nil || pending != strings.Repeat("b", 40) {
		t.Fatal("latest push was not retained", err)
	}
	secondCtx, secondCancel := context.WithCancel(ctx)
	secondDone := make(chan error, 1)
	go func() { secondDone <- (deployments.Worker{Store: s}).Run(secondCtx) }()
	deadline = time.Now().Add(15 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		var successes int
		s.DB.QueryRow(ctx, "SELECT count(*) FROM deployments WHERE project_id=$1 AND status='SUCCESS'", project.ID).Scan(&successes)
		if successes == 3 {
			completed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	secondCancel()
	<-secondDone
	if !completed {
		t.Fatal("pending commit was not deployed after the active job")
	}
	start := request("POST", "/api/v1/auth/totp/start", map[string]string{"password": credentials["password"]}, nil)
	if start.Code != 200 {
		t.Fatalf("TOTP start: %s", start.Body)
	}
	var enrollment map[string]string
	json.Unmarshal(start.Body.Bytes(), &enrollment)
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment["secret"])
	if err != nil {
		t.Fatal(err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(time.Now().Unix()/30))
	otp := hmac.New(sha1.New, key)
	otp.Write(counter[:])
	digest := otp.Sum(nil)
	offset := digest[len(digest)-1] & 15
	code := fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
	enabled := request("POST", "/api/v1/auth/totp/enable", map[string]string{"code": code}, nil)
	if enabled.Code != 200 {
		t.Fatalf("TOTP enable: %s", enabled.Body)
	}
	var recovery struct {
		Codes []string `json:"recovery_codes"`
	}
	json.Unmarshal(enabled.Body.Bytes(), &recovery)
	if len(recovery.Codes) != 8 {
		t.Fatal("missing recovery codes")
	}
	login := map[string]string{"email": credentials["email"], "password": credentials["password"], "code": code}
	if got := request("POST", "/api/v1/auth/login", login, nil).Code; got != 401 {
		t.Fatalf("TOTP replay accepted: %d", got)
	}
	login["code"] = recovery.Codes[0]
	if got := request("POST", "/api/v1/auth/login", login, nil).Code; got != 200 {
		t.Fatalf("recovery login: %d", got)
	}
	if got := request("POST", "/api/v1/auth/login", login, nil).Code; got != 401 {
		t.Fatalf("recovery replay accepted: %d", got)
	}
	if got := request("POST", "/api/v1/auth/totp/disable", map[string]string{"password": credentials["password"], "code": recovery.Codes[1]}, nil).Code; got != 200 {
		t.Fatalf("TOTP disable: %d", got)
	}
	var streamDeployment string
	if err = s.DB.QueryRow(ctx, "SELECT id::text FROM deployments WHERE project_id=$1 ORDER BY created_at DESC LIMIT 1", project.ID).Scan(&streamDeployment); err != nil {
		t.Fatal(err)
	}
	streamServer := httptest.NewServer(handler)
	defer streamServer.Close()
	streamCtx, streamCancel := context.WithTimeout(ctx, 20*time.Second)
	defer streamCancel()
	streamRequest, err := http.NewRequestWithContext(streamCtx, "GET", streamServer.URL+"/api/v1/deployments/"+streamDeployment+"/logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamRequest.AddCookie(cookie)
	streamResponse, err := (&http.Client{Timeout: 20 * time.Second}).Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer streamResponse.Body.Close()
	if streamResponse.StatusCode != http.StatusOK {
		t.Fatalf("SSE authentication: %d", streamResponse.StatusCode)
	}
	streamClosed := make(chan error, 1)
	go func() { _, e := io.Copy(io.Discard, streamResponse.Body); streamClosed <- e }()
	if got := request("POST", "/api/v1/auth/logout", nil, nil).Code; got != 200 {
		t.Fatalf("logout: %d", got)
	}
	if got := request("GET", "/api/v1/projects", nil, nil).Code; got != 401 {
		t.Fatalf("revoked session: %d", got)
	}
	select {
	case err = <-streamClosed:
		if err != nil {
			t.Fatal("revoked SSE did not close cleanly", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("SSE remained active after session revocation")
	}
}
