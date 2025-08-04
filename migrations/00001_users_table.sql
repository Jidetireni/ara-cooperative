-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    email_confirmed_at TIMESTAMPTZ,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);

CREATE INDEX idx_users_email_confirmed ON users(email_confirmed_at);

-- +goose Down
DROP INDEX idx_users_email;

DROP INDEX idx_users_email_confirmed;

DROP TABLE users;