-- +goose Up
CREATE TABLE loan_settings (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    minimum_savings BIGINT NOT NULL DEFAULT 0 CHECK (minimum_savings >= 0),
    minimum_share_units NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (minimum_share_units >= 0),
    updated_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ
);
INSERT INTO loan_settings (id) VALUES (1);

CREATE TABLE loans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
    requested_amount BIGINT NOT NULL CHECK (requested_amount > 0),
    purpose TEXT NOT NULL CHECK (btrim(purpose) <> ''),
    requested_duration_months INT NOT NULL CHECK (requested_duration_months > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'ACTIVE')),
    approved_amount BIGINT CHECK (approved_amount > 0),
    approved_duration_months INT CHECK (approved_duration_months > 0),
    interest_method VARCHAR(30) CHECK (interest_method IN ('FLAT', 'REDUCING_BALANCE')),
    interest_rate_percent NUMERIC(7,4) CHECK (interest_rate_percent BETWEEN 0 AND 100),
    interest_period VARCHAR(10) CHECK (interest_period IN ('MONTHLY', 'ANNUAL')),
    review_reason TEXT,
    override_reason TEXT,
    eligibility_snapshot JSONB,
    reviewed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    disbursed_amount BIGINT CHECK (disbursed_amount > 0),
    disbursed_at TIMESTAMPTZ,
    disbursement_reference VARCHAR(100) UNIQUE,
    disbursed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    transaction_id UUID UNIQUE REFERENCES transactions(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    CONSTRAINT loans_review_state CHECK (
        (status = 'PENDING' AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR (status <> 'PENDING' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    ),
    CONSTRAINT loans_terms CHECK (
        (status IN ('PENDING', 'REJECTED') AND approved_amount IS NULL AND approved_duration_months IS NULL
            AND interest_method IS NULL AND interest_rate_percent IS NULL AND interest_period IS NULL
            AND eligibility_snapshot IS NULL AND override_reason IS NULL)
        OR (status IN ('APPROVED', 'ACTIVE') AND approved_amount IS NOT NULL AND approved_duration_months IS NOT NULL
            AND interest_method IS NOT NULL AND interest_rate_percent IS NOT NULL AND interest_period IS NOT NULL
            AND eligibility_snapshot IS NOT NULL)
    ),
    CONSTRAINT loans_rejection_reason CHECK (status <> 'REJECTED' OR (review_reason IS NOT NULL AND btrim(review_reason) <> '')),
    CONSTRAINT loans_disbursement_state CHECK (
        (status <> 'ACTIVE' AND disbursed_amount IS NULL AND disbursed_at IS NULL AND disbursement_reference IS NULL
            AND disbursed_by IS NULL AND transaction_id IS NULL)
        OR (status = 'ACTIVE' AND disbursed_amount IS NOT NULL AND disbursed_amount = approved_amount
            AND disbursed_at IS NOT NULL AND disbursement_reference IS NOT NULL AND btrim(disbursement_reference) <> ''
            AND disbursed_by IS NOT NULL AND transaction_id IS NOT NULL)
    )
);
CREATE INDEX idx_loans_member ON loans(member_id);
CREATE INDEX idx_loans_status ON loans(status);
CREATE INDEX idx_loans_created_at ON loans(created_at, id);

-- +goose Down
DROP TABLE loans;
DROP TABLE loan_settings;
