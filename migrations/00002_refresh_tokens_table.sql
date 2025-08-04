-- +goose Up
CREATE TABLE refresh_tokens (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    hash_token VARCHAR(255) NOT NULL UNIQUE,
    is_valid BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, hash_token)
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

CREATE INDEX idx_refresh_tokens_is_valid ON refresh_tokens(is_valid);


-- +goose Down
DROP INDEX idx_refresh_tokens_user_id;

DROP INDEX idx_refresh_tokens_expires_at;

DROP INDEX idx_refresh_tokens_is_valid;

DROP TABLE refresh_tokens;
