-- +goose Up
CREATE TABLE administrators (
    id integer PRIMARY KEY CHECK (id = 1),
    email text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    csrf text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE projects (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    source_type text NOT NULL CHECK (source_type IN ('image','git')),
    source text NOT NULL,
    branch text NOT NULL DEFAULT 'main',
    build_method text NOT NULL DEFAULT 'dockerfile' CHECK (build_method IN ('dockerfile','buildpack')),
    dockerfile text NOT NULL DEFAULT 'Dockerfile',
    build_context text NOT NULL DEFAULT '.',
    port integer NOT NULL DEFAULT 3000 CHECK (port BETWEEN 1 AND 65535),
    domain text UNIQUE,
    health_path text NOT NULL DEFAULT '/',
    auto_deploy boolean NOT NULL DEFAULT false,
    memory_mb integer NOT NULL DEFAULT 512 CHECK (memory_mb BETWEEN 64 AND 32768),
    cpu numeric NOT NULL DEFAULT 1 CHECK (cpu > 0 AND cpu <= 32),
    active_container text NOT NULL DEFAULT '',
    active_image text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'STOPPED',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE environment_variables (
    project_id uuid NOT NULL REFERENCES projects ON DELETE CASCADE,
    key text NOT NULL,
    encrypted_value text NOT NULL,
    PRIMARY KEY (project_id, key)
);
CREATE TABLE deployments (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'QUEUED' CHECK (status IN ('QUEUED','CLONING','BUILDING','DEPLOYING','HEALTHCHECK','SUCCESS','FAILED','CANCELLED')),
    image text NOT NULL DEFAULT '',
    commit_sha text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    cancel_requested boolean NOT NULL DEFAULT false,
    attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE UNIQUE INDEX one_active_deployment ON deployments(project_id)
    WHERE status IN ('QUEUED','CLONING','BUILDING','DEPLOYING','HEALTHCHECK');
CREATE INDEX deployments_queue ON deployments(created_at) WHERE status = 'QUEUED';
CREATE TABLE deployment_logs (
    id bigserial PRIMARY KEY,
    deployment_id uuid NOT NULL REFERENCES deployments ON DELETE CASCADE,
    line text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX deployment_logs_cursor ON deployment_logs(deployment_id,id);
CREATE TABLE webhook_deliveries (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    action text NOT NULL,
    target text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE managed_databases (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    engine text NOT NULL CHECK(engine IN ('postgres','redis','mysql')),
    version text NOT NULL,
    encrypted_password text NOT NULL,
    status text NOT NULL DEFAULT 'CREATING',
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE managed_databases, audit_events, webhook_deliveries, deployment_logs, deployments, environment_variables, projects, sessions, administrators;
