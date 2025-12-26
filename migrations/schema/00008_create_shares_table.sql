-- +goose Up
CREATE TABLE shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    units DECIMAL(18, 4) NOT NULL,
    unit_price BIGINT NOT NULL, 
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_shares_transaction_id ON shares(transaction_id);
CREATE INDEX idx_shares_created_at ON shares(created_at);

CREATE TABLE share_unit_prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), 
    price BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_share_unit_prices_price ON share_unit_prices(price);

-- +goose Down
DROP INDEX IF EXISTS idx_shares_transaction_id;
DROP INDEX IF EXISTS idx_shares_created_at;

DROP TABLE IF EXISTS shares;

DROP INDEX IF EXISTS idx_share_unit_prices_price;
DROP TABLE IF EXISTS share_unit_prices;