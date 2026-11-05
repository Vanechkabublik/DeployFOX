-- +goose Up
CREATE TABLE project_domains (
    domain text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects ON DELETE CASCADE
);
CREATE INDEX project_domains_owner ON project_domains(project_id);
INSERT INTO project_domains(domain,project_id) SELECT domain,id FROM projects WHERE domain IS NOT NULL;
-- +goose Down
DROP TABLE project_domains;
