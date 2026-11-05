-- +goose Up
ALTER TABLE administrators ADD COLUMN totp_encrypted text NOT NULL DEFAULT '';
ALTER TABLE administrators ADD COLUMN totp_pending text NOT NULL DEFAULT '';
ALTER TABLE administrators ADD COLUMN totp_last_counter bigint NOT NULL DEFAULT -1;
ALTER TABLE administrators ADD COLUMN totp_recovery text[] NOT NULL DEFAULT '{}';
-- +goose Down
ALTER TABLE administrators DROP COLUMN totp_encrypted, DROP COLUMN totp_pending, DROP COLUMN totp_last_counter, DROP COLUMN totp_recovery;
