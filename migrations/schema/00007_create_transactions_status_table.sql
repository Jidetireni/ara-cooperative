-- +goose Up
CREATE TABLE transaction_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    confirmed_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (transaction_id)
);

CREATE INDEX idx_transaction_status_transaction_id ON transaction_status(transaction_id);
CREATE INDEX idx_transaction_status_confirmed_at ON transaction_status(confirmed_at);
CREATE INDEX idx_transaction_status_rejected_at ON transaction_status(rejected_at);

-- +goose Down
DROP INDEX IF EXISTS idx_transaction_status_transaction_id;
DROP INDEX IF EXISTS idx_transaction_status_confirmed_at;
DROP INDEX IF EXISTS idx_transaction_status_rejected_at;

DROP TABLE IF EXISTS transaction_status;
