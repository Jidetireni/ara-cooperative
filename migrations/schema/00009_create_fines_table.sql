-- +goose Up
CREATE TABLE fines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
    amount BIGINT NOT NULL,
    reason TEXT NOT NULL,
    deadline TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE INDEX idx_fines_member_id ON fines(member_id);
CREATE INDEX idx_fines_admin_id ON fines(admin_id);

-- +goose Down
DROP INDEX IF EXISTS idx_fines_member_id;
DROP INDEX IF EXISTS idx_fines_admin_id;
DROP TABLE IF EXISTS fines;