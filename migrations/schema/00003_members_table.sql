-- +goose Up
CREATE TABLE members (  
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code VARCHAR(50) NOT NULL UNIQUE,
    slug VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    phone VARCHAR(20) NOT NULL UNIQUE,
    address TEXT,
    next_of_kin_name VARCHAR(255),
    next_of_kin_phone VARCHAR(20),
    membership_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- Member search indexes
CREATE UNIQUE INDEX idx_members_user_id ON members(user_id, (deleted_at IS NULL))
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX idx_members_code ON members(code, (deleted_at IS NULL))
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX idx_members_slug ON members(slug, (deleted_at IS NULL))
WHERE deleted_at IS NULL;

-- Phone index with soft-delete consideration
CREATE UNIQUE INDEX idx_members_phone ON members (phone, (deleted_at IS NULL))
WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_members_user_id;
DROP INDEX IF EXISTS idx_members_phone;
DROP INDEX IF EXISTS idx_members_code;
DROP INDEX IF EXISTS idx_members_slug;
DROP TABLE IF EXISTS members;