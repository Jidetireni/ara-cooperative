-- +goose Up
-- An invited member completes their phone number after setting a password.
ALTER TABLE members ALTER COLUMN phone DROP NOT NULL;

-- Login and invitation identity are case-insensitive, including deleted accounts.
CREATE UNIQUE INDEX idx_users_email_case_insensitive ON users (lower(email));

CREATE SEQUENCE members_slug_seq;
SELECT setval('members_slug_seq',
    COALESCE(MAX(SUBSTRING(slug FROM 4)::bigint), 0) + 1, FALSE)
FROM members WHERE slug ~ '^ara[0-9]+$';

-- +goose Down
-- Refuse rollback until incomplete profiles have phone numbers again.
ALTER TABLE members ALTER COLUMN phone SET NOT NULL;
DROP INDEX idx_users_email_case_insensitive;
DROP SEQUENCE members_slug_seq;
