-- +goose Up
-- Fields from the paper loan application form (the source of truth).
-- Nullable so applications made before this change keep loading; the
-- application endpoint requires them for new applications.
ALTER TABLE loans
    ADD COLUMN repayment_start_date DATE,
    ADD COLUMN repayment_duration_months INT CHECK (repayment_duration_months > 0),
    ADD COLUMN monthly_repayment_amount BIGINT CHECK (monthly_repayment_amount > 0),
    ADD COLUMN disbursement_bank_name VARCHAR(100),
    ADD COLUMN disbursement_account_name VARCHAR(255),
    ADD COLUMN disbursement_account_number VARCHAR(10) CHECK (disbursement_account_number ~ '^[0-9]{10}$'),
    ADD COLUMN declaration_accepted_at TIMESTAMPTZ;

-- Guarantors (bye-law 68 sureties): two members who vouch for the applicant.
-- Each guarantor accepts or declines in the app, in place of the form's
-- signature and date.
CREATE TABLE loan_guarantors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    loan_id UUID NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
    position SMALLINT NOT NULL CHECK (position IN (1, 2)),
    status VARCHAR(10) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED')),
    responded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (loan_id, position),
    UNIQUE (loan_id, member_id),
    CONSTRAINT loan_guarantors_response CHECK ((status = 'PENDING') = (responded_at IS NULL))
);
CREATE INDEX idx_loan_guarantors_member ON loan_guarantors(member_id);

-- +goose Down
DROP TABLE loan_guarantors;
ALTER TABLE loans
    DROP COLUMN repayment_start_date,
    DROP COLUMN repayment_duration_months,
    DROP COLUMN monthly_repayment_amount,
    DROP COLUMN disbursement_bank_name,
    DROP COLUMN disbursement_account_name,
    DROP COLUMN disbursement_account_number,
    DROP COLUMN declaration_accepted_at;
