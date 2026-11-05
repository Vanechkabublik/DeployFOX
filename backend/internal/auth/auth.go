package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"deployfox/internal/platform"
	"deployfox/internal/security"
)

type Handler struct{ Store *platform.Store }
type sessionKey struct{}

// Streams periodically recheck revocation instead of keeping a logged-out session alive.
func (h Handler) SessionActive(r *http.Request) bool {
	cookie, err := r.Cookie("deployfox_session")
	if err != nil {
		return false
	}
	var active bool
	err = h.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>now())", security.Hash(cookie.Value)).Scan(&active)
	return err == nil && active
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		Error(w, 400, "Invalid JSON request")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		Error(w, 400, "Exactly one JSON value required")
		return false
	}
	return true
}

func (h Handler) Status(w http.ResponseWriter, r *http.Request) {
	var ready bool
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM administrators)").Scan(&ready); err != nil {
		Error(w, 503, "Database unavailable")
		return
	}
	JSON(w, 200, map[string]any{"initialized": ready, "provider": h.Store.Config.Provider})
}

func (h Handler) Setup(w http.ResponseWriter, r *http.Request) {
	var initialized bool
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM administrators)").Scan(&initialized); err != nil {
		Error(w, 503, "Database unavailable")
		return
	}
	if initialized {
		http.NotFound(w, r)
		return
	}
	if !security.Equal(r.Header.Get("X-Setup-Token"), h.Store.Config.SetupToken) {
		Error(w, 403, "Invalid initialization token")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !Decode(w, r, &body) {
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil || len(body.Password) < 12 || len(body.Password) > 256 {
		Error(w, 400, "Valid email and password of 12–256 characters required")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		Error(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), "INSERT INTO administrators(id,email,password_hash) VALUES(1,$1,$2)", strings.ToLower(body.Email), security.Password(body.Password))
	if err != nil {
		Error(w, 409, "Initialization unavailable or already completed")
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO settings(key,value) VALUES('setup.completed','false') ON CONFLICT(key) DO UPDATE SET value='false',updated_at=now()"); err != nil {
		Error(w, 500, "Cannot initialize setup wizard")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		Error(w, 500, "Cannot initialize account")
		return
	}
	h.Store.Audit(r.Context(), "setup.completed", "")
	h.createSession(w, r)
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !Decode(w, r, &body) {
		return
	}
	if len(body.Password) > 256 {
		Error(w, 401, "Invalid credentials")
		return
	}
	var email, hash, totp string
	err := h.Store.DB.QueryRow(r.Context(), "SELECT email,password_hash,totp_encrypted FROM administrators WHERE id=1").Scan(&email, &hash, &totp)
	if err != nil || !security.VerifyPassword(body.Password, hash) || !security.Equal(strings.ToLower(body.Email), email) {
		h.Store.Audit(r.Context(), "login.failed", "")
		Error(w, 401, "Invalid credentials")
		return
	}
	if totp != "" && !h.verifySecondFactor(r.Context(), totp, body.Code) {
		h.Store.Audit(r.Context(), "login.second_factor_failed", "")
		Error(w, 401, "TOTP or recovery code required/invalid")
		return
	}
	h.createSession(w, r)
}

func (h Handler) createSession(w http.ResponseWriter, r *http.Request) {
	token, csrf := security.Token(), security.Token()
	_, err := h.Store.DB.Exec(r.Context(), "INSERT INTO sessions(token_hash,csrf,expires_at) VALUES($1,$2,$3)", security.Hash(token), csrf, time.Now().Add(24*time.Hour))
	if err != nil {
		Error(w, 500, "Cannot create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "deployfox_session", Value: token, Path: "/", HttpOnly: true, Secure: h.Store.Config.Production, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	h.Store.Audit(r.Context(), "login.success", "")
	JSON(w, 200, map[string]string{"csrf": csrf})
}

func (h Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("deployfox_session")
		if err != nil {
			Error(w, 401, "Authentication required")
			return
		}
		var csrf string
		err = h.Store.DB.QueryRow(r.Context(), "SELECT csrf FROM sessions WHERE token_hash=$1 AND expires_at>now()", security.Hash(cookie.Value)).Scan(&csrf)
		if err != nil {
			Error(w, 401, "Session expired")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && (!security.Equal(r.Header.Get("X-CSRF-Token"), csrf) || r.Header.Get("Origin") != h.Store.Config.Origin) {
			Error(w, 403, "CSRF validation failed")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, csrf)))
	})
}

func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	var email string
	var totp bool
	var needsSetup bool
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT email,totp_encrypted<>'',NOT coalesce((SELECT value::boolean FROM settings WHERE key='setup.completed'),true) FROM administrators WHERE id=1").Scan(&email, &totp, &needsSetup); err != nil {
		Error(w, 500, "Account unavailable")
		return
	}
	JSON(w, 200, map[string]any{"email": email, "csrf": r.Context().Value(sessionKey{}), "provider": h.Store.Config.Provider, "totp_enabled": totp, "needs_setup": needsSetup})
}
func (h Handler) Logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("deployfox_session")
	if _, err := h.Store.DB.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", security.Hash(c.Value)); err != nil {
		Error(w, 500, "Cannot revoke session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "deployfox_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.Store.Config.Production, SameSite: http.SameSiteStrictMode})
	h.Store.Audit(r.Context(), "logout", "")
	JSON(w, 200, map[string]bool{"ok": true})
}
func (h Handler) Password(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !Decode(w, r, &b) {
		return
	}
	var hash string
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT password_hash FROM administrators WHERE id=1").Scan(&hash); err != nil || !security.VerifyPassword(b.Current, hash) {
		Error(w, 403, "Current password incorrect")
		return
	}
	if len(b.Password) < 12 || len(b.Password) > 256 {
		Error(w, 400, "Password must contain 12–256 characters")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "UPDATE administrators SET password_hash=$1 WHERE id=1", security.Password(b.Password)); err == nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM sessions")
	}
	if err != nil {
		Error(w, 500, "Password update failed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		Error(w, 500, "Password update failed")
		return
	}
	h.Store.Audit(r.Context(), "password.changed", "")
	JSON(w, 200, map[string]bool{"ok": true})
}
