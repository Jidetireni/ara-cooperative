-- +goose Up
ALTER TABLE user_roles 
ADD CONSTRAINT unique_user_role 
UNIQUE (user_id, role_id);

-- +goose Down
ALTER TABLE user_roles 
DROP CONSTRAINT IF EXISTS unique_user_role;