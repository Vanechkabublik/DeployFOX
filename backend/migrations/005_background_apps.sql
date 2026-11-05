-- +goose Up
ALTER TABLE projects ADD COLUMN service_type text NOT NULL DEFAULT 'http' CHECK (service_type IN ('http','worker'));
-- +goose Down
ALTER TABLE projects DROP COLUMN service_type;
