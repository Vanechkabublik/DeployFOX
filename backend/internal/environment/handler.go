package environment

import (
	"net/http"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB.Query(r.Context(), "SELECT key FROM environment_variables WHERE project_id=$1 ORDER BY key", chi.URLParam(r, "id"))
	if err != nil {
		auth.Error(w, 500, "Cannot read variables")
		return
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if rows.Scan(&key) != nil {
			auth.Error(w, 500, "Cannot read variable")
			return
		}
		keys = append(keys, key)
	}
	if rows.Err() != nil {
		auth.Error(w, 500, "Cannot read variables")
		return
	}
	auth.JSON(w, 200, keys)
}
func (h Handler) Save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values map[string]string `json:"values"`
		Delete []string          `json:"delete"`
	}
	if !auth.Decode(w, r, &body) {
		return
	}
	if len(body.Values) > 200 || len(body.Delete) > 200 {
		auth.Error(w, 400, "Maximum 200 variables per request")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	for key, value := range body.Values {
		if !security.EnvPattern.MatchString(key) || len(value) > 65536 {
			auth.Error(w, 400, "Invalid variable key or value")
			return
		}
		cipher, err := security.Encrypt(h.Store.Config.EncryptionKey, value)
		if err != nil {
			auth.Error(w, 500, "Encryption failed")
			return
		}
		if _, err = tx.Exec(r.Context(), "INSERT INTO environment_variables(project_id,key,encrypted_value) VALUES($1,$2,$3) ON CONFLICT(project_id,key) DO UPDATE SET encrypted_value=excluded.encrypted_value", chi.URLParam(r, "id"), key, cipher); err != nil {
			auth.Error(w, 400, "Cannot save variable")
			return
		}
	}
	for _, key := range body.Delete {
		if _, err = tx.Exec(r.Context(), "DELETE FROM environment_variables WHERE project_id=$1 AND key=$2", chi.URLParam(r, "id"), key); err != nil {
			auth.Error(w, 500, "Cannot remove variable")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Cannot save variables")
		return
	}
	h.Store.Audit(r.Context(), "environment.updated", chi.URLParam(r, "id"))
	auth.JSON(w, 200, map[string]string{"message": "Saved. Deploy again to apply the new environment."})
}
