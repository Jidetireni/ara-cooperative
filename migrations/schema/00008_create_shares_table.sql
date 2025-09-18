-- +goose Up
CREATE TABLE shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    units DECIMAL(10, 2) NOT NULL,
    unit_price BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_shares_transaction_id ON shares(transactions_id);
CREATE INDEX idx_shares_created_at ON shares(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_shares_transaction_id;
DROP INDEX IF EXISTS idx_shares_created_at;

DROP TABLE IF EXISTS shares;
