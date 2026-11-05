-- +goose Up
ALTER TABLE projects ADD COLUMN pending_commit text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE projects DROP COLUMN pending_commit;
