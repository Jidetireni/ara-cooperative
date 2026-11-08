-- +goose Up
-- Online (Paystack) payments. Each row keeps the amounts and fee rate agreed at
-- checkout, so later configuration changes never alter earlier payments.
CREATE TYPE payment_status AS ENUM ('PENDING', 'SUCCESS', 'FAILED');

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
    purpose ledger_type NOT NULL CHECK (purpose <> 'LOAN'),
    reference VARCHAR(100) NOT NULL UNIQUE,
    currency CHAR(3) NOT NULL DEFAULT 'NGN',
    -- Kobo. amount is what the member's ledger is credited with.
    amount BIGINT NOT NULL CHECK (amount > 0),
    maintenance_fee BIGINT NOT NULL CHECK (maintenance_fee >= 0),
    maintenance_fee_percent INT NOT NULL CHECK (maintenance_fee_percent BETWEEN 0 AND 100),
    processing_charge BIGINT NOT NULL CHECK (processing_charge >= 0),
    total BIGINT NOT NULL,
    description VARCHAR(255) NOT NULL,
    fine_id UUID REFERENCES fines(id) ON DELETE RESTRICT,
    -- SHARES: the quote agreed at checkout.
    share_unit_price BIGINT CHECK (share_unit_price > 0),
    share_units DECIMAL(18, 4) CHECK (share_units > 0),
    -- Paystack split snapshot: settlement_share is the gross allocation to
    -- settlement_subaccount. New checkouts allocate maintenance plus processing
    -- to the owner's subaccount, which bears the fee; the cooperative keeps the principal.
    settlement_subaccount VARCHAR(50),
    settlement_share BIGINT CHECK (settlement_share > 0),
    status payment_status NOT NULL DEFAULT 'PENDING',
    authorization_url TEXT,
    access_code VARCHAR(100),
    transaction_id UUID UNIQUE REFERENCES transactions(id) ON DELETE RESTRICT,
    provider_transaction_id BIGINT,
    provider_fees BIGINT,
    channel VARCHAR(50),
    failure_reason TEXT,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    CONSTRAINT payments_total_matches CHECK (total = amount + maintenance_fee + processing_charge),
    CONSTRAINT payments_fine_required CHECK ((purpose = 'FINES') = (fine_id IS NOT NULL)),
    CONSTRAINT payments_share_price_required CHECK ((purpose = 'SHARES') = (share_unit_price IS NOT NULL AND share_units IS NOT NULL)),
    CONSTRAINT payments_split_complete CHECK ((settlement_subaccount IS NULL) = (settlement_share IS NULL)),
    CONSTRAINT payments_success_applied CHECK (status <> 'SUCCESS' OR (transaction_id IS NOT NULL AND paid_at IS NOT NULL))
);

CREATE INDEX idx_payments_member_id ON payments(member_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_fine_id ON payments(fine_id);

-- +goose Down
DROP TABLE payments;
DROP TYPE payment_status;
