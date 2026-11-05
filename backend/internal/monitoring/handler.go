package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/docker"
	"deployfox/internal/platform"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func (h Handler) Caddy(w http.ResponseWriter, r *http.Request) {
	if h.Store.Config.Provider == "mock" {
		auth.JSON(w, 200, map[string]bool{"available": false})
		return
	}
	h.call(w, r, docker.Request{Action: "caddy-check"})
}

func (h Handler) Host(w http.ResponseWriter, r *http.Request) {
	if h.Store.Config.Provider == "mock" {
		auth.JSON(w, 200, map[string]bool{"available": false})
		return
	}
	h.call(w, r, docker.Request{Action: "host-metrics"})
}
func (h Handler) Project(w http.ResponseWriter, r *http.Request) {
	if h.Store.Config.Provider == "mock" {
		auth.JSON(w, 200, map[string]bool{"available": false})
		return
	}
	p, err := h.Store.Project(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.ActiveContainer == "" {
		auth.Error(w, 404, "No application container")
		return
	}
	h.call(w, r, docker.Request{Action: "metrics", Project: p})
}
func (h Handler) call(w http.ResponseWriter, r *http.Request, req docker.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := (docker.Provider{Config: h.Store.Config}).Call(ctx, req)
	if err != nil {
		auth.Error(w, 502, "Monitoring agent unavailable")
		return
	}
	var body any
	if json.Unmarshal([]byte(result.Output), &body) != nil {
		auth.Error(w, 502, "Invalid monitoring result")
		return
	}
	auth.JSON(w, 200, body)
}
