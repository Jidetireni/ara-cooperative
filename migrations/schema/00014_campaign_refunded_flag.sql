-- +goose Up
-- Keep this separate: campaign management was already applied locally before
-- the final decision to track refunds with a simple flag.
ALTER TABLE campaigns
    ADD COLUMN updated_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN is_refunded BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE campaigns DROP COLUMN is_refunded, DROP COLUMN updated_by;
