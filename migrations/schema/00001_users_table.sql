-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    phone VARCHAR(20) NOT NULL UNIQUE,
    address VARCHAR(255),
    email_confirmed_at TIMESTAMPTZ,
    password_hash VARCHAR(255),
    next_of_kin_name VARCHAR(255),
    next_of_kin_phone VARCHAR(20),
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_users_email ON users (email, (deleted_at IS NULL))
WHERE
  deleted_at IS NULL;

CREATE UNIQUE INDEX idx_users_phone ON users (phone, (deleted_at IS NULL))
WHERE
  deleted_at IS NULL;

CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE INDEX idx_users_created_at_id ON users (created_at, id);

-- Improve name-based search performance
CREATE INDEX idx_users_names ON users (first_name, last_name);

-- Improve performance for email confirmation queries
CREATE INDEX idx_users_email_confirmed_at ON users (email_confirmed_at)
WHERE
  email_confirmed_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_email;

DROP INDEX IF EXISTS idx_users_phone;

DROP INDEX IF EXISTS idx_users_deleted_at;

DROP INDEX IF EXISTS idx_users_created_at_id;

DROP INDEX IF EXISTS idx_users_names;

DROP INDEX IF EXISTS idx_users_email_confirmed_at;

DROP TABLE IF EXISTS users;