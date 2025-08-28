-- +goose Up
CREATE TABLE savings_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID REFERENCES transactions(id) NOT NULL,
    confirmed_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_savings_status_transaction_id ON savings_status(transaction_id);
CREATE INDEX idx_savings_status_confirmed_at ON savings_status(confirmed_at);
CREATE INDEX idx_savings_status_rejected_at ON savings_status(rejected_at);

-- +goose Down
DROP INDEX IF EXISTS idx_savings_status_transaction_id;
DROP INDEX IF EXISTS idx_savings_status_confirmed_at;
DROP INDEX IF EXISTS idx_savings_status_rejected_at;

DROP TABLE IF EXISTS savings_status;
