-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255),
    email_confirmed_at TIMESTAMPTZ,
    -- last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- Partial unique constraint for email (only for non-deleted users)
CREATE UNIQUE INDEX unique_user_email ON users (email) 
WHERE deleted_at IS NULL;

-- Auth-specific indexes
CREATE INDEX idx_users_email ON users (email, (deleted_at IS NULL))
WHERE deleted_at IS NULL;

-- Login performance index
CREATE INDEX idx_users_email_password ON users (email, password_hash)
WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS unique_user_email;
DROP INDEX IF EXISTS idx_users_email;
DROP INDEX IF EXISTS idx_users_email_password;
DROP TABLE IF EXISTS users;