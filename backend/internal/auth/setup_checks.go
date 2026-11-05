package auth

import (
	"encoding/json"
	"net/http"

	"deployfox/internal/platform"
)

// SetupChecks reports control-plane state; host operations are handled by the monitoring endpoint.
func (h Handler) SetupChecks(w http.ResponseWriter, r *http.Request) {
	var completed bool
	var projects int
	var connected bool
	err := h.Store.DB.QueryRow(r.Context(), `SELECT coalesce((SELECT value::boolean FROM settings WHERE key='setup.completed'),true),(SELECT count(*) FROM projects),EXISTS(SELECT 1 FROM settings WHERE key='github.active')`).Scan(&completed, &projects, &connected)
	if err != nil {
		Error(w, 500, "Cannot load setup state")
		return
	}
	JSON(w, 200, map[string]any{"completed": completed, "projects": projects, "github_connected": connected, "public_url": h.Store.Config.Origin, "provider": h.Store.Config.Provider})
}
func (h Handler) FinishSetup(w http.ResponseWriter, r *http.Request) {
	var exists bool
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM projects)").Scan(&exists); err != nil {
		Error(w, 500, "Cannot check projects")
		return
	}
	if !exists {
		Error(w, 409, "Create the first project before completing setup")
		return
	}
	if err := saveSetupState(r, h.Store, true); err != nil {
		Error(w, 500, "Cannot complete setup")
		return
	}
	h.Store.Audit(r.Context(), "setup.wizard_completed", "")
	JSON(w, 200, map[string]bool{"ok": true})
}
func saveSetupState(r *http.Request, s *platform.Store, completed bool) error {
	data, _ := json.Marshal(completed)
	_, err := s.DB.Exec(r.Context(), "INSERT INTO settings(key,value) VALUES('setup.completed',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=now()", data)
	return err
}
