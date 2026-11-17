-- +goose Up
ALTER TABLE members ADD COLUMN is_existing_member BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE members DROP COLUMN is_existing_member;
