-- +goose Up
-- The applicant picks the bank from Paystack's list; the code lets the API
-- resolve the account holder's name with Paystack.
ALTER TABLE loans ADD COLUMN disbursement_bank_code VARCHAR(20);

-- +goose Down
ALTER TABLE loans DROP COLUMN disbursement_bank_code;
