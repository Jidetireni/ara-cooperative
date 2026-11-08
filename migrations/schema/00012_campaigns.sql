-- +goose Up
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL CHECK (btrim(name) <> ''),
    description TEXT NOT NULL CHECK (btrim(description) <> ''),
    participation VARCHAR(20) NOT NULL CHECK (participation IN ('VOLUNTARY', 'REQUIRED')),
    amount_type VARCHAR(20) NOT NULL CHECK (amount_type IN ('FIXED', 'FLEXIBLE')),
    amount BIGINT CHECK (amount >= 50000),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT campaigns_dates CHECK (ends_at > starts_at),
    CONSTRAINT campaigns_amount CHECK ((amount_type = 'FIXED') = (amount IS NOT NULL))
);

ALTER TABLE payments ADD COLUMN campaign_id UUID REFERENCES campaigns(id) ON DELETE RESTRICT;
-- Older special-deposit records have no campaign; preserve their history.
ALTER TABLE payments ADD CONSTRAINT payments_campaign_purpose
    CHECK (campaign_id IS NULL OR purpose = 'SPECIAL_DEPOSIT');
CREATE INDEX idx_payments_campaign_id ON payments(campaign_id, member_id, status);
CREATE INDEX idx_campaigns_created_at ON campaigns(created_at, id);

-- +goose Down
ALTER TABLE payments DROP CONSTRAINT payments_campaign_purpose;
ALTER TABLE payments DROP COLUMN campaign_id;
DROP TABLE campaigns;
