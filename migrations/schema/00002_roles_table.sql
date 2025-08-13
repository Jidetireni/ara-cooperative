-- +goose Up
CREATE TABLE
  roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    permission VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

-- Improve permission-based role lookup
CREATE INDEX idx_roles_permission ON roles (permission);

-- Improve time-based pagination
CREATE INDEX idx_roles_created_at_id ON roles (created_at, id);

CREATE TABLE
  user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

-- Improve user-based role queries
CREATE INDEX idx_user_role_user_id ON user_roles (user_id);

-- Improve role-based user queries
CREATE INDEX idx_user_role_role_id ON user_roles (role_id);

-- Improve time-based pagination for user roles
CREATE INDEX idx_user_role_created_at_id ON user_roles (created_at, id);

-- +goose Down
DROP INDEX IF EXISTS idx_user_role_user_id;

DROP INDEX IF EXISTS idx_user_role_role_id;

DROP INDEX IF EXISTS idx_user_role_created_at_id;

DROP INDEX IF EXISTS idx_roles_permission;

DROP INDEX IF EXISTS idx_roles_created_at_id;

DROP TABLE user_roles;

DROP TABLE roles;
