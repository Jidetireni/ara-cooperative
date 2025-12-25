-- +goose Up
CREATE TABLE
  roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

CREATE INDEX idx_roles_name ON roles (name);
CREATE INDEX idx_roles_created_at_id ON roles (created_at, id);

CREATE TABLE
  user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, role_id)
  );

CREATE INDEX idx_user_role_user_id ON user_roles (user_id);
CREATE INDEX idx_user_role_role_id ON user_roles (role_id);
CREATE INDEX idx_user_role_created_at_id ON user_roles (created_at, id);

CREATE TABLE
  permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    slug VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

CREATE INDEX idx_permission_slug ON permissions (slug);
CREATE INDEX idx_permission_created_at_id ON permissions (created_at, id);

CREATE TABLE
  user_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, permission_id)
  );

CREATE INDEX idx_user_permission_user_id ON user_permissions (user_id);
CREATE INDEX idx_user_permission_permission_id ON user_permissions (permission_id);

-- +goose Down
DROP INDEX IF EXISTS idx_user_role_user_id;
DROP INDEX IF EXISTS idx_user_role_role_id;
DROP INDEX IF EXISTS idx_user_role_created_at_id;
DROP INDEX IF EXISTS idx_roles_name;
DROP INDEX IF EXISTS idx_roles_created_at_id;
DROP TABLE user_roles;
DROP TABLE roles;

DROP INDEX IF EXISTS idx_user_permission_user_id;
DROP INDEX IF EXISTS idx_user_permission_permission_id;
DROP INDEX IF EXISTS idx_permission_slug;
DROP INDEX IF EXISTS idx_permission_created_at_id;
DROP TABLE user_permissions;
DROP TABLE permissions;
