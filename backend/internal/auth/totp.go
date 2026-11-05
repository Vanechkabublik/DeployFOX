package auth

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"deployfox/internal/security"
)

func (h Handler) verifySecondFactor(ctx context.Context, encrypted, code string) bool {
	secret, err := security.Decrypt(h.Store.Config.EncryptionKey, encrypted)
	if err != nil {
		return false
	}
	if counter, valid := security.TOTPCounter(secret, code, time.Now()); valid {
		tag, err := h.Store.DB.Exec(ctx, "UPDATE administrators SET totp_last_counter=$1 WHERE id=1 AND totp_last_counter<$1", counter)
		return err == nil && tag.RowsAffected() == 1
	}
	if len(code) != 20 {
		return false
	}
	tag, err := h.Store.DB.Exec(ctx, "UPDATE administrators SET totp_recovery=array_remove(totp_recovery,$1) WHERE id=1 AND $1=ANY(totp_recovery)", security.Hash(code))
	return err == nil && tag.RowsAffected() == 1
}
func (h Handler) TOTPStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !Decode(w, r, &body) {
		return
	}
	var email, hash, enabled string
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT email,password_hash,totp_encrypted FROM administrators WHERE id=1").Scan(&email, &hash, &enabled); err != nil || len(body.Password) > 256 || !security.VerifyPassword(body.Password, hash) {
		Error(w, 403, "Password incorrect")
		return
	}
	if enabled != "" {
		Error(w, 409, "TOTP already enabled")
		return
	}
	secret := security.TOTPSecret()
	encrypted, err := security.Encrypt(h.Store.Config.EncryptionKey, secret)
	if err != nil {
		Error(w, 500, "Encryption failed")
		return
	}
	if _, err = h.Store.DB.Exec(r.Context(), "UPDATE administrators SET totp_pending=$1 WHERE id=1", encrypted); err != nil {
		Error(w, 500, "Cannot start enrollment")
		return
	}
	JSON(w, 200, map[string]string{"secret": secret, "uri": "otpauth://totp/DeployFox:" + url.PathEscape(email) + "?secret=" + secret + "&issuer=DeployFox&algorithm=SHA1&digits=6&period=30"})
}
func (h Handler) TOTPEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !Decode(w, r, &body) {
		return
	}
	var encrypted string
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT totp_pending FROM administrators WHERE id=1 AND totp_encrypted='' AND totp_pending<>''").Scan(&encrypted); err != nil {
		Error(w, 409, "Start enrollment first")
		return
	}
	secret, err := security.Decrypt(h.Store.Config.EncryptionKey, encrypted)
	if err != nil {
		Error(w, 500, "Cannot decrypt enrollment")
		return
	}
	counter, valid := security.TOTPCounter(secret, body.Code, time.Now())
	if !valid {
		Error(w, 400, "Invalid TOTP code")
		return
	}
	codes, hashes := []string{}, []string{}
	for i := 0; i < 8; i++ {
		code := security.Token()[:20]
		codes = append(codes, code)
		hashes = append(hashes, security.Hash(code))
	}
	tag, err := h.Store.DB.Exec(r.Context(), "UPDATE administrators SET totp_encrypted=totp_pending,totp_pending='',totp_last_counter=$1,totp_recovery=$2 WHERE id=1 AND totp_encrypted='' AND totp_pending=$3", counter, hashes, encrypted)
	if err != nil || tag.RowsAffected() != 1 {
		Error(w, 409, "Enrollment changed; retry")
		return
	}
	h.Store.Audit(r.Context(), "totp.enabled", "")
	JSON(w, 200, map[string]any{"recovery_codes": codes})
}
func (h Handler) TOTPDisable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !Decode(w, r, &body) {
		return
	}
	var hash, encrypted string
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT password_hash,totp_encrypted FROM administrators WHERE id=1").Scan(&hash, &encrypted); err != nil || len(body.Password) > 256 || !security.VerifyPassword(body.Password, hash) || !h.verifySecondFactor(r.Context(), encrypted, body.Code) {
		Error(w, 403, "Password or TOTP/recovery code incorrect")
		return
	}
	if _, err := h.Store.DB.Exec(r.Context(), "UPDATE administrators SET totp_encrypted='',totp_pending='',totp_last_counter=-1,totp_recovery='{}' WHERE id=1"); err != nil {
		Error(w, 500, "Cannot disable TOTP")
		return
	}
	h.Store.Audit(r.Context(), "totp.disabled", "")
	JSON(w, 200, map[string]bool{"ok": true})
}
func (h Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB.Query(r.Context(), "SELECT token_hash,created_at,expires_at FROM sessions WHERE expires_at>now() ORDER BY created_at DESC LIMIT 100")
	if err != nil {
		Error(w, 500, "Cannot list sessions")
		return
	}
	defer rows.Close()
	current, _ := r.Cookie("deployfox_session")
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var created, expires time.Time
		if rows.Scan(&id, &created, &expires) != nil {
			Error(w, 500, "Cannot read session")
			return
		}
		items = append(items, map[string]any{"id": id, "created_at": created, "expires_at": expires, "current": security.Equal(id, security.Hash(current.Value))})
	}
	if rows.Err() != nil {
		Error(w, 500, "Cannot read sessions")
		return
	}
	JSON(w, 200, items)
}
