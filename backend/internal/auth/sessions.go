package auth

import (
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if raw, err := hex.DecodeString(id); err != nil || len(raw) != 32 {
		Error(w, 400, "Invalid session identifier")
		return
	}
	if _, err := h.Store.DB.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", id); err != nil {
		Error(w, 500, "Cannot revoke session")
		return
	}
	h.Store.Audit(r.Context(), "session.revoked", "")
	JSON(w, 200, map[string]bool{"ok": true})
}
