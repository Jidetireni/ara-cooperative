-- +goose Up
CREATE TABLE
  tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token VARCHAR(255) NOT NULL UNIQUE,
    is_valid BOOLEAN NOT NULL,
    token_type VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

CREATE INDEX idx_tokens_user_id ON tokens (user_id);

CREATE INDEX idx_tokens_valid ON tokens (is_valid)
WHERE
  is_valid = TRUE;

CREATE INDEX idx_tokens_expires_at ON tokens (expires_at);

CREATE INDEX idx_tokens_token_type ON tokens (token_type);

CREATE INDEX idx_tokens_deleted_at ON tokens (deleted_at);

-- +goose Down
DROP INDEX IF EXISTS idx_tokens_user_id;

DROP INDEX IF EXISTS idx_tokens_valid;

DROP INDEX IF EXISTS idx_tokens_expires_at;

DROP INDEX IF EXISTS idx_tokens_token_type;

DROP INDEX IF EXISTS idx_tokens_deleted_at;

DROP TABLE tokens;