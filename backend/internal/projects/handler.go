package projects

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"deployfox/internal/auth"
	"deployfox/internal/docker"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

func Validate(p platform.Project) error {
	if p.ServiceType != "http" && p.ServiceType != "worker" {
		return fmt.Errorf("service_type must be http or worker")
	}
	if p.ServiceType == "worker" && p.Domain != "" {
		return fmt.Errorf("background workers cannot have an HTTP domain")
	}
	if len(p.Name) < 2 || len(p.Name) > 64 {
		return fmt.Errorf("name must contain 2–64 characters")
	}
	switch p.SourceType {
	case "image":
		if !security.ImagePattern.MatchString(p.Source) || strings.HasPrefix(p.Source, "-") {
			return fmt.Errorf("invalid image reference")
		}
	case "git":
		if err := security.Repository(p.Source); err != nil {
			return err
		}
	default:
		return fmt.Errorf("source_type must be image or git")
	}
	if !security.RelativePath(p.Dockerfile) || !security.RelativePath(p.BuildContext) {
		return fmt.Errorf("build paths must stay within repository")
	}
	if p.BuildMethod != "dockerfile" && p.BuildMethod != "buildpack" {
		return fmt.Errorf("unsupported build method")
	}
	if p.Port < 1 || p.Port > 65535 || p.MemoryMB < 64 || p.MemoryMB > 32768 || p.CPU <= 0 || p.CPU > 32 {
		return fmt.Errorf("invalid port or resource limits")
	}
	if p.Domain != "" && !security.Domain(p.Domain) {
		return fmt.Errorf("invalid public domain")
	}
	if !strings.HasPrefix(p.HealthPath, "/") || strings.ContainsAny(p.HealthPath, "\r\n") || len(p.HealthPath) > 512 {
		return fmt.Errorf("invalid healthcheck path")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./-]{0,127}$`).MatchString(p.Branch) || strings.Contains(p.Branch, "..") {
		return fmt.Errorf("invalid branch")
	}
	return nil
}
func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB.Query(r.Context(), "SELECT "+platform.ProjectColumns+" FROM projects ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		auth.Error(w, 500, "Cannot load projects")
		return
	}
	defer rows.Close()
	items := []platform.Project{}
	for rows.Next() {
		p, err := platform.ScanProject(rows)
		if err != nil {
			auth.Error(w, 500, "Cannot read project")
			return
		}
		items = append(items, p)
	}
	if rows.Err() != nil {
		auth.Error(w, 500, "Cannot read projects")
		return
	}
	auth.JSON(w, 200, items)
}
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	p := platform.Project{ServiceType: "http", Branch: "main", BuildMethod: "dockerfile", Dockerfile: "Dockerfile", BuildContext: ".", Port: 3000, HealthPath: "/", MemoryMB: 512, CPU: 1}
	if !auth.Decode(w, r, &p) {
		return
	}
	if err := Validate(p); err != nil {
		auth.Error(w, 400, err.Error())
		return
	}
	p.ID = security.ID()
	if h.panelDomain(p.Domain) {
		auth.Error(w, 400, "Panel domain is reserved")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO projects(id,name,source_type,source,branch,build_method,dockerfile,build_context,port,domain,health_path,auto_deploy,memory_mb,cpu,service_type) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13,$14,$15)`, p.ID, p.Name, p.SourceType, p.Source, p.Branch, p.BuildMethod, p.Dockerfile, p.BuildContext, p.Port, p.Domain, p.HealthPath, p.AutoDeploy, p.MemoryMB, p.CPU, p.ServiceType)
	if err != nil {
		auth.Error(w, 409, "Cannot create project: name or domain already used")
		return
	}
	if err = reserveDomain(r.Context(), tx, p); err != nil {
		auth.Error(w, 409, "Domain is reserved by another project")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Cannot commit project creation")
		return
	}
	h.Store.Audit(r.Context(), "project.created", p.ID)
	auth.JSON(w, 201, p)
}
func (h Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.Project(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		auth.Error(w, 404, "Project not found")
		return
	}
	auth.JSON(w, 200, p)
}

func (h Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.Store.Project(r.Context(), id)
	if err != nil {
		auth.Error(w, 404, "Project not found")
		return
	}
	did := security.ID()
	image := ""
	if strings.HasSuffix(r.URL.Path, "/rollback") {
		err = h.Store.DB.QueryRow(r.Context(), "SELECT image FROM deployments WHERE project_id=$1 AND status='SUCCESS' AND image<>$2 ORDER BY finished_at DESC LIMIT 1", id, p.ActiveImage).Scan(&image)
		if err != nil {
			auth.Error(w, 409, "No previous successful image available")
			return
		}
	}
	_, err = h.Store.DB.Exec(r.Context(), "INSERT INTO deployments(id,project_id,image) VALUES($1,$2,$3)", did, id, image)
	if err != nil {
		auth.Error(w, 409, "A deployment is already active for this project")
		return
	}
	h.Store.Audit(r.Context(), "deployment.queued", did)
	h.Store.Wake(r.Context())
	auth.JSON(w, 202, map[string]string{"id": did})
}
func (h Handler) Action(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	action := chi.URLParam(r, "action")
	if action != "start" && action != "stop" && action != "restart" {
		auth.Error(w, 400, "Unsupported action")
		return
	}
	p, err := h.Store.Project(r.Context(), id)
	if err != nil || p.ActiveContainer == "" {
		auth.Error(w, 409, "No deployed container")
		return
	}
	var active bool
	if err = h.Store.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM deployments WHERE project_id=$1 AND status IN ('QUEUED','CLONING','BUILDING','DEPLOYING','HEALTHCHECK'))", id).Scan(&active); err != nil || active {
		auth.Error(w, 409, "Project has an active deployment")
		return
	}
	_, err = (docker.Provider{Config: h.Store.Config}).Call(r.Context(), docker.Request{Action: action, Project: p})
	if err != nil {
		auth.Error(w, 502, err.Error())
		return
	}
	status := "RUNNING"
	if action == "stop" {
		status = "STOPPED"
	}
	if _, err = h.Store.DB.Exec(r.Context(), "UPDATE projects SET status=$2 WHERE id=$1", id, status); err != nil {
		auth.Error(w, 500, "Cannot save container status")
		return
	}
	h.Store.Audit(r.Context(), "project."+action, id)
	auth.JSON(w, 200, map[string]bool{"ok": true})
}
func (h Handler) Delete(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.Project(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		auth.Error(w, 404, "Project not found")
		return
	}
	if r.Header.Get("X-Confirm-Delete") != p.Name {
		auth.Error(w, 400, "Confirm deletion using exact project name")
		return
	}
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SELECT id FROM projects WHERE id=$1 FOR UPDATE", p.ID); err != nil {
		auth.Error(w, 500, "Cannot lock project")
		return
	}
	var active bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM deployments WHERE project_id=$1 AND status IN ('QUEUED','CLONING','BUILDING','DEPLOYING','HEALTHCHECK'))", p.ID).Scan(&active); err != nil || active {
		auth.Error(w, 409, "Cancel or finish the active deployment first")
		return
	}
	if _, err = (docker.Provider{Config: h.Store.Config}).Call(r.Context(), docker.Request{Action: "delete", Project: p}); err != nil {
		auth.Error(w, 502, "Cannot remove project containers and route; retry safely")
		return
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM projects WHERE id=$1", p.ID); err != nil {
		auth.Error(w, 409, "Project deletion failed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Cannot commit project deletion")
		return
	}
	h.Store.Audit(r.Context(), "project.deleted", p.ID)
	auth.JSON(w, 200, map[string]bool{"ok": true})
}
