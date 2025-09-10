-- +goose Up
CREATE TYPE transaction_type AS ENUM  (
    'DEPOSIT',
    'WITHDRAWAL',
    'LOAN_DISBURSEMENT',
    'LOAN_REPAYMENT'
);

CREATE TYPE ledger_type AS ENUM (
    'SAVINGS',
    'SHARES',
    'LOAN',
    'FINES',
    'REGISTRATION_FEE',
    'SPECIAL_DEPOSIT'
);

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    description VARCHAR(255) NOT NULL,
    reference VARCHAR(255) NOT NULL UNIQUE,
    amount BIGINT NOT NULL,
    type transaction_type NOT NULL,
    ledger ledger_type NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE INDEX idx_transactions_member_id ON transactions(member_id);
CREATE INDEX idx_transactions_type ON transactions(type);
CREATE INDEX idx_transactions_ledger ON transactions(ledger);


-- +goose Down
DROP INDEX IF EXISTS idx_transactions_member_id;
DROP INDEX IF EXISTS idx_transactions_type;
DROP INDEX IF EXISTS idx_transactions_ledger;

DROP TABLE IF EXISTS transactions;
DROP TYPE IF EXISTS transaction_type;
DROP TYPE IF EXISTS ledger_type;
