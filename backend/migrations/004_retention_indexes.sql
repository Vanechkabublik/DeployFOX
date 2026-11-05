-- +goose Up
CREATE INDEX deployment_logs_retention ON deployment_logs(created_at);
CREATE INDEX audit_events_retention ON audit_events(created_at);
CREATE INDEX sessions_expiry ON sessions(expires_at);
-- +goose Down
DROP INDEX deployment_logs_retention, audit_events_retention, sessions_expiry;
