package logs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/docker"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func (h Handler) Application(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.Project(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.ActiveContainer == "" {
		auth.Error(w, 404, "No active application container")
		return
	}
	secrets := []string{}
	rows, err := h.Store.DB.Query(r.Context(), "SELECT encrypted_value FROM environment_variables WHERE project_id=$1", p.ID)
	if err != nil {
		auth.Error(w, 500, "Cannot load redaction rules")
		return
	}
	for rows.Next() {
		var value string
		if rows.Scan(&value) != nil {
			rows.Close()
			auth.Error(w, 500, "Cannot load redaction rules")
			return
		}
		plain, e := security.Decrypt(h.Store.Config.EncryptionKey, value)
		if e != nil {
			rows.Close()
			auth.Error(w, 500, "Cannot decrypt redaction rules")
			return
		}
		if plain != "" {
			secrets = append(secrets, plain)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		auth.Error(w, 500, "Cannot load redaction rules")
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		auth.Error(w, 500, "Streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	previous := ""
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	nextSessionCheck := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(nextSessionCheck) {
			if !(auth.Handler{Store: h.Store}).SessionActive(r) {
				return
			}
			nextSessionCheck = time.Now().Add(10 * time.Second)
		}
		result, e := (docker.Provider{Config: h.Store.Config}).Call(r.Context(), docker.Request{Action: "logs", Project: p})
		if e != nil {
			data, _ := json.Marshal(map[string]string{"line": "Log provider unavailable; retrying"})
			fmt.Fprintf(w, "data: %s\n\n", data)
		} else if result.Output != previous {
			output := result.Output
			for _, secret := range secrets {
				output = strings.ReplaceAll(output, secret, "[REDACTED]")
			}
			// A bounded snapshot event avoids duplicates and unbounded browser memory during polling.
			data, _ := json.Marshal(map[string]string{"line": output, "mode": "snapshot"})
			fmt.Fprintf(w, "data: %s\n\n", data)
			previous = result.Output
		}
		fmt.Fprint(w, ": heartbeat\n\n")
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			return
		case <-tick.C:
		}
	}
}
