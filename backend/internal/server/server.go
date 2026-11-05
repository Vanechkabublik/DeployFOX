package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"deployfox/internal/auth"
	"deployfox/internal/databases"
	"deployfox/internal/deployments"
	"deployfox/internal/domains"
	"deployfox/internal/environment"
	"deployfox/internal/github"
	"deployfox/internal/logs"
	"deployfox/internal/monitoring"
	"deployfox/internal/openapi"
	"deployfox/internal/platform"
	"deployfox/internal/projects"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
func Serve(ctx context.Context, address string, handler http.Handler) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return serveListener(ctx, listener, handler)
}
func serveListener(ctx context.Context, listener net.Listener, handler http.Handler) error {
	s := &http.Server{Handler: handler, ReadTimeout: 30 * time.Second, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20, BaseContext: func(net.Listener) context.Context { return ctx }}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdown); err != nil {
			slog.Warn("graceful shutdown deadline exceeded", "error", err)
			s.Close()
		}
	}()
	slog.Info("listening", "address", listener.Addr().String())
	err := s.Serve(listener)
	if err == http.ErrServerClosed {
		<-shutdownDone
		return nil
	}
	return err
}
func Router(s *platform.Store) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if s.DB.Ping(r.Context()) != nil || s.Redis.Ping(r.Context()).Err() != nil {
			auth.Error(w, 503, "Dependency unavailable")
			return
		}
		auth.JSON(w, 200, map[string]bool{"ok": true})
	})
	a := auth.Handler{Store: s}
	p := projects.Handler{Store: s}
	d := deployments.Handler{Store: s}
	e := environment.Handler{Store: s}
	g := github.Handler{Store: s}
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/setup/status", a.Status)
		r.With(limit(s, 5)).Post("/setup", a.Setup)
		r.With(limit(s, 5)).Post("/auth/login", a.Login)
		r.With(limit(s, 120)).Post("/github/webhook", g.Webhook)
		r.With(limit(s, 30)).Get("/github/callback", g.Callback)
		r.Group(func(r chi.Router) {
			r.Use(a.Middleware, limit(s, 300))
			r.Get("/openapi.json", openapi.Serve)
			dbHandler := databases.Handler{Store: s}
			r.Get("/databases", dbHandler.List)
			r.Post("/databases", dbHandler.Create)
			r.Post("/databases/{id}/{action}", dbHandler.Action)
			r.Get("/auth/me", a.Me)
			r.Get("/setup/checks", a.SetupChecks)
			r.Post("/setup/complete", a.FinishSetup)
			r.Post("/auth/logout", a.Logout)
			r.Post("/auth/password", a.Password)
			r.Post("/auth/totp/start", a.TOTPStart)
			r.Post("/auth/totp/enable", a.TOTPEnable)
			r.Post("/auth/totp/disable", a.TOTPDisable)
			r.Get("/auth/sessions", a.Sessions)
			r.Delete("/auth/sessions/{id}", a.RevokeSession)
			r.Get("/projects", p.List)
			r.Post("/projects", p.Create)
			r.Get("/projects/{id}", p.Get)
			r.Put("/projects/{id}", p.Update)
			r.Get("/projects/{id}/domain-check", (domains.Handler{Store: s}).Check)
			r.Get("/projects/{id}/logs", (logs.Handler{Store: s}).Application)
			r.Get("/monitoring/host", (monitoring.Handler{Store: s}).Host)
			r.Get("/monitoring/caddy", (monitoring.Handler{Store: s}).Caddy)
			r.Get("/projects/{id}/metrics", (monitoring.Handler{Store: s}).Project)
			r.Delete("/projects/{id}", p.Delete)
			r.Post("/projects/{id}/deploy", p.Deploy)
			r.Post("/projects/{id}/rollback", p.Deploy)
			r.Post("/projects/{id}/actions/{action}", p.Action)
			r.Get("/projects/{id}/environment", e.List)
			r.Put("/projects/{id}/environment", e.Save)
			r.Get("/deployments", d.List)
			r.Post("/deployments/{id}/cancel", d.Cancel)
			r.Get("/deployments/{id}/logs", d.Logs)
			r.Get("/github/repositories", g.Repositories)
			r.Get("/github/branches", g.Branches)
			r.Get("/github/connection", g.Connection)
			r.Post("/github/connect", g.Connect)
			r.Post("/github/confirm", g.Confirm)
			r.Get("/system", func(w http.ResponseWriter, r *http.Request) {
				var projects, running, failed, queued int
				err := s.DB.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM projects),(SELECT count(*) FROM projects WHERE status='RUNNING'),(SELECT count(*) FROM deployments WHERE status='FAILED'),(SELECT count(*) FROM deployments WHERE status='QUEUED')`).Scan(&projects, &running, &failed, &queued)
				if err != nil {
					auth.Error(w, 500, "Cannot load statistics")
					return
				}
				auth.JSON(w, 200, map[string]any{"projects": projects, "running": running, "failed": failed, "queued": queued, "provider": s.Config.Provider})
			})
			r.Get("/audit", func(w http.ResponseWriter, r *http.Request) {
				rows, err := s.DB.Query(r.Context(), "SELECT action,target,created_at FROM audit_events ORDER BY id DESC LIMIT 100")
				if err != nil {
					auth.Error(w, 500, "Cannot read audit")
					return
				}
				defer rows.Close()
				items := []map[string]any{}
				for rows.Next() {
					var action, target string
					var at time.Time
					if rows.Scan(&action, &target, &at) != nil {
						auth.Error(w, 500, "Cannot read audit")
						return
					}
					items = append(items, map[string]any{"action": action, "target": target, "created_at": at})
				}
				if rows.Err() != nil {
					auth.Error(w, 500, "Cannot read audit")
					return
				}
				auth.JSON(w, 200, items)
			})
		})
	})
	return r
}

func limit(s *platform.Store, max int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// RemoteAddr comes from the private Next proxy; never trust arbitrary forwarding headers.
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			key := "rate:" + host + ":" + r.URL.Path + ":" + strconv.FormatInt(time.Now().Unix()/60, 10)
			n, err := s.Redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],65) end; return n`, []string{key}).Int()
			if err != nil {
				auth.Error(w, 503, "Rate limiter unavailable")
				return
			}
			if n > max {
				w.Header().Set("Retry-After", "60")
				auth.Error(w, 429, "Too many requests; retry in one minute")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
