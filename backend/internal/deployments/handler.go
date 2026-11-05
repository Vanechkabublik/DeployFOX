package deployments

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB.Query(r.Context(), `SELECT d.id::text,d.project_id::text,p.name,d.status,d.image,d.commit_sha,d.error,d.created_at,d.started_at,d.finished_at FROM deployments d JOIN projects p ON p.id=d.project_id WHERE ($1='' OR d.project_id::text=$1) ORDER BY d.created_at DESC LIMIT 100`, r.URL.Query().Get("project_id"))
	if err != nil {
		auth.Error(w, 500, "Cannot load deployments")
		return
	}
	defer rows.Close()
	items := []platform.Deployment{}
	for rows.Next() {
		var d platform.Deployment
		if err = rows.Scan(&d.ID, &d.ProjectID, &d.ProjectName, &d.Status, &d.Image, &d.CommitSHA, &d.Error, &d.CreatedAt, &d.StartedAt, &d.FinishedAt); err != nil {
			auth.Error(w, 500, "Cannot read deployment")
			return
		}
		items = append(items, d)
	}
	if rows.Err() != nil {
		auth.Error(w, 500, "Cannot read deployments")
		return
	}
	auth.JSON(w, 200, items)
}
func (h Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	tag, err := h.Store.DB.Exec(r.Context(), "UPDATE deployments SET cancel_requested=true WHERE id=$1 AND status IN ('QUEUED','CLONING','BUILDING')", chi.URLParam(r, "id"))
	if err != nil || tag.RowsAffected() == 0 {
		auth.Error(w, 409, "Deployment cannot be cancelled after traffic transition begins")
		return
	}
	h.Store.Audit(r.Context(), "deployment.cancel_requested", chi.URLParam(r, "id"))
	auth.JSON(w, 202, map[string]bool{"ok": true})
}
func (h Handler) Logs(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		auth.Error(w, 500, "Streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	cursor, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if c, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil && c > cursor {
		cursor = c
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	end := time.NewTimer(10 * time.Minute)
	defer end.Stop()
	nextSessionCheck := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(nextSessionCheck) {
			if !(auth.Handler{Store: h.Store}).SessionActive(r) {
				return
			}
			nextSessionCheck = time.Now().Add(10 * time.Second)
		}
		rows, err := h.Store.DB.Query(r.Context(), "SELECT id,line FROM deployment_logs WHERE deployment_id=$1 AND id>$2 ORDER BY id LIMIT 200", chi.URLParam(r, "id"), cursor)
		if err != nil {
			return
		}
		for rows.Next() {
			var id int64
			var line string
			if rows.Scan(&id, &line) != nil {
				rows.Close()
				return
			}
			data, _ := json.Marshal(map[string]string{"line": line})
			if _, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", id, data); err != nil {
				rows.Close()
				return
			}
			cursor = id
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return
		}
		fmt.Fprint(w, ": heartbeat\n\n")
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-end.C:
			return
		case <-tick.C:
		}
	}
}
