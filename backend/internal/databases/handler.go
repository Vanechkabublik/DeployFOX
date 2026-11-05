package databases

import (
	"net/http"
	"strings"

	"deployfox/internal/auth"
	"deployfox/internal/docker"
	"deployfox/internal/platform"
	"deployfox/internal/security"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *platform.Store }

var Versions = map[string][]string{"postgres": {"16", "17"}, "redis": {"7"}, "mysql": {"8.4"}}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB.Query(r.Context(), "SELECT id::text,name,engine,version,status FROM managed_databases ORDER BY created_at DESC")
	if err != nil {
		auth.Error(w, 500, "Cannot load databases")
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, name, engine, version, status string
		if rows.Scan(&id, &name, &engine, &version, &status) != nil {
			auth.Error(w, 500, "Cannot read database")
			return
		}
		items = append(items, map[string]string{"id": id, "name": name, "engine": engine, "version": version, "status": status})
	}
	if rows.Err() != nil {
		auth.Error(w, 500, "Cannot read databases")
		return
	}
	auth.JSON(w, 200, items)
}
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name    string `json:"name"`
		Engine  string `json:"engine"`
		Version string `json:"version"`
	}
	if !auth.Decode(w, r, &b) {
		return
	}
	valid := false
	for _, v := range Versions[b.Engine] {
		if b.Version == v {
			valid = true
		}
	}
	if !valid || len(b.Name) < 2 || len(b.Name) > 64 {
		auth.Error(w, 400, "Unsupported engine/version or invalid name")
		return
	}
	id, password := security.ID(), security.Token()
	cipher, err := security.Encrypt(h.Store.Config.EncryptionKey, password)
	if err != nil {
		auth.Error(w, 500, "Encryption failed")
		return
	}
	_, err = h.Store.DB.Exec(r.Context(), "INSERT INTO managed_databases(id,name,engine,version,encrypted_password) VALUES($1,$2,$3,$4,$5)", id, b.Name, b.Engine, b.Version, cipher)
	if err != nil {
		auth.Error(w, 409, "Database name already exists")
		return
	}
	_, err = (docker.Provider{Config: h.Store.Config}).Call(r.Context(), docker.Request{Action: "database-create", Project: platform.Project{ID: id}, Database: docker.DatabaseSpec{Engine: b.Engine, Version: b.Version, Password: password}})
	status := "RUNNING"
	if err != nil {
		status = "FAILED"
	}
	if _, saveErr := h.Store.DB.Exec(r.Context(), "UPDATE managed_databases SET status=$2 WHERE id=$1", id, status); saveErr != nil {
		auth.Error(w, 500, "Cannot persist database state")
		return
	}
	h.Store.Audit(r.Context(), "database.created", id)
	if err != nil {
		auth.Error(w, 502, "Database creation failed; persistent volume retained")
		return
	}
	auth.JSON(w, 201, map[string]string{"id": id, "status": status})
}
func (h Handler) Action(w http.ResponseWriter, r *http.Request) {
	id, action := chi.URLParam(r, "id"), chi.URLParam(r, "action")
	var engine, version, cipher, name string
	if err := h.Store.DB.QueryRow(r.Context(), "SELECT engine,version,encrypted_password,name FROM managed_databases WHERE id=$1", id).Scan(&engine, &version, &cipher, &name); err != nil {
		auth.Error(w, 404, "Database not found")
		return
	}
	password, err := security.Decrypt(h.Store.Config.EncryptionKey, cipher)
	if err != nil {
		auth.Error(w, 500, "Cannot decrypt database credential")
		return
	}
	if action == "connection" {
		if h.Store.Config.Provider == "mock" {
			auth.Error(w, 409, "Mock databases have no real connection string")
			return
		}
		host := "dfdb-" + id
		uri := ""
		switch engine {
		case "postgres":
			uri = "postgresql://app:" + password + "@" + host + ":5432/app"
		case "redis":
			uri = "redis://:" + password + "@" + host + ":6379/0"
		case "mysql":
			uri = "mysql://app:" + password + "@" + host + ":3306/app"
		}
		h.Store.Audit(r.Context(), "database.credential_revealed", id)
		auth.JSON(w, 200, map[string]string{"uri": uri})
		return
	}
	if action != "start" && action != "stop" && action != "restart" && action != "backup" && action != "restore" {
		auth.Error(w, 400, "Unsupported database action")
		return
	}
	backup := ""
	if action == "restore" {
		var b struct {
			Backup       string `json:"backup"`
			Confirmation string `json:"confirmation"`
		}
		if !auth.Decode(w, r, &b) {
			return
		}
		if b.Confirmation != name {
			auth.Error(w, 400, "Exact database name confirmation required; restore replaces data")
			return
		}
		backup = b.Backup
		h.Store.Audit(r.Context(), "database.restore_requested", id)
	}
	result, err := (docker.Provider{Config: h.Store.Config}).Call(r.Context(), docker.Request{Action: "database-" + action, Project: platform.Project{ID: id}, Backup: backup, Database: docker.DatabaseSpec{Engine: engine, Version: version, Password: password}})
	if err != nil {
		auth.Error(w, 502, err.Error())
		return
	}
	if action != "backup" && action != "restore" {
		status := "RUNNING"
		if action == "stop" {
			status = "STOPPED"
		}
		if _, err = h.Store.DB.Exec(r.Context(), "UPDATE managed_databases SET status=$2 WHERE id=$1", id, status); err != nil {
			auth.Error(w, 500, "Cannot save database state")
			return
		}
	}
	h.Store.Audit(r.Context(), "database."+action, id)
	auth.JSON(w, 200, map[string]string{"message": strings.TrimSpace(result.Output)})
}
