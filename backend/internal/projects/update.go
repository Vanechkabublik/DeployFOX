package projects

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"deployfox/internal/auth"
	"deployfox/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func reserveDomain(ctx context.Context, tx pgx.Tx, p platform.Project) error {
	if p.Domain == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, "INSERT INTO project_domains(domain,project_id) VALUES($1,$2) ON CONFLICT DO NOTHING", p.Domain, p.ID); err != nil {
		return err
	}
	var owner string
	if err := tx.QueryRow(ctx, "SELECT project_id::text FROM project_domains WHERE domain=$1", p.Domain).Scan(&owner); err != nil {
		return err
	}
	if owner != p.ID {
		return fmt.Errorf("domain is reserved by another project")
	}
	return nil
}
func (h Handler) panelDomain(domain string) bool {
	origin, err := url.Parse(h.Store.Config.Origin)
	return err == nil && domain != "" && domain == origin.Hostname()
}
func (h Handler) Update(w http.ResponseWriter, r *http.Request) {
	tx, err := h.Store.DB.Begin(r.Context())
	if err != nil {
		auth.Error(w, 500, "Database unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	id := chi.URLParam(r, "id")
	p, err := platform.ScanProject(tx.QueryRow(r.Context(), "SELECT "+platform.ProjectColumns+" FROM projects WHERE id=$1 FOR UPDATE", id))
	if err != nil {
		auth.Error(w, 404, "Project not found")
		return
	}
	if !auth.Decode(w, r, &p) {
		return
	}
	p.ID = id
	if err = Validate(p); err != nil {
		auth.Error(w, 400, err.Error())
		return
	}
	if h.panelDomain(p.Domain) {
		auth.Error(w, 400, "Panel domain is reserved")
		return
	}
	var active bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM deployments WHERE project_id=$1 AND status IN ('QUEUED','CLONING','BUILDING','DEPLOYING','HEALTHCHECK'))", id).Scan(&active); err != nil || active {
		auth.Error(w, 409, "Finish or cancel the active deployment before editing")
		return
	}
	if err = reserveDomain(r.Context(), tx, p); err != nil {
		auth.Error(w, 409, "Domain is reserved by another project")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE projects SET name=$2,source_type=$3,source=$4,branch=$5,build_method=$6,dockerfile=$7,build_context=$8,port=$9,domain=NULLIF($10,''),health_path=$11,auto_deploy=$12,memory_mb=$13,cpu=$14,service_type=$15 WHERE id=$1`, id, p.Name, p.SourceType, p.Source, p.Branch, p.BuildMethod, p.Dockerfile, p.BuildContext, p.Port, p.Domain, p.HealthPath, p.AutoDeploy, p.MemoryMB, p.CPU, p.ServiceType)
	if err != nil {
		auth.Error(w, 409, "Name or domain conflict")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		auth.Error(w, 500, "Cannot save project")
		return
	}
	h.Store.Audit(r.Context(), "project.updated", id)
	auth.JSON(w, 200, p)
}
