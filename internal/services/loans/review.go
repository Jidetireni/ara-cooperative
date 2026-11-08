package loans

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func (l *Loan) pending(ctx context.Context, id uuid.UUID, tx *sqlx.Tx) (*repository.Loan, error) {
	loan, err := l.LoanRepo.Get(ctx, repository.LoanRepositoryFilter{ID: &id, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	if loan.Status != "PENDING" {
		return nil, apiError(http.StatusConflict, "only pending loan applications can be reviewed")
	}
	return loan, nil
}

func (l *Loan) Approve(ctx context.Context, id uuid.UUID, input dto.LoanApprovalInput) (*dto.Loan, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	rate, number, err := decimal(input.InterestRatePercent)
	if err != nil {
		return nil, err
	}
	if number.Cmp(big.NewRat(100, 1)) > 0 {
		return nil, apiError(http.StatusBadRequest, "interest_rate_percent must be between 0 and 100")
	}
	validMethod := input.InterestMethod == "FLAT" || input.InterestMethod == "REDUCING_BALANCE"
	validPeriod := input.InterestPeriod == "MONTHLY" || input.InterestPeriod == "ANNUAL"
	validAmounts := input.Amount > 0 && input.DurationMonths > 0
	if !validMethod || !validPeriod || !validAmounts {
		return nil, apiError(http.StatusBadRequest, "valid amount, duration and interest terms are required")
	}
	tx, err := l.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	loan, err := l.pending(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	member, err := l.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{ID: &loan.MemberID, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	if member.UserID == actor.ID {
		return nil, apiError(http.StatusForbidden, "you cannot approve your own loan")
	}
	if !member.ActivatedAt.Valid {
		return nil, apiError(http.StatusConflict, "active membership required before loan approval")
	}
	checks, err := l.eligibility(ctx, loan, tx)
	if err != nil {
		return nil, err
	}
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)
	if len(checks.Flags) > 0 && input.OverrideReason == "" {
		return nil, apiError(http.StatusConflict, "advisory flags require an override_reason before approval")
	}
	snapshot, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}
	loan.Status = "APPROVED"
	loan.ApprovedAmount = sql.NullInt64{Int64: input.Amount, Valid: true}
	loan.ApprovedDurationMonths = sql.NullInt32{Int32: input.DurationMonths, Valid: true}
	loan.InterestMethod = sql.NullString{String: input.InterestMethod, Valid: true}
	loan.InterestRatePercent = sql.NullString{String: rate, Valid: true}
	loan.InterestPeriod = sql.NullString{String: input.InterestPeriod, Valid: true}
	loan.ReviewReason = optionalText(input.Reason)
	loan.OverrideReason = optionalText(input.OverrideReason)
	loan.EligibilitySnapshot = sql.NullString{String: string(snapshot), Valid: true}
	loan.ReviewedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	loan.ReviewedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := l.LoanRepo.Update(ctx, loan, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l.Get(ctx, id)
}

func (l *Loan) Reject(ctx context.Context, id uuid.UUID, input dto.LoanRejectionInput) (*dto.Loan, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return nil, apiError(http.StatusBadRequest, "a rejection reason is required")
	}
	tx, err := l.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	loan, err := l.pending(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	loan.Status = "REJECTED"
	loan.ReviewReason = optionalText(input.Reason)
	loan.ReviewedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	loan.ReviewedAt = sql.NullTime{Time: time.Now(), Valid: true}
	if _, err := l.LoanRepo.Update(ctx, loan, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l.Get(ctx, id)
}

func (l *Loan) Disburse(ctx context.Context, id uuid.UUID, input dto.LoanDisbursementInput) (*dto.Loan, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input.Reference = strings.TrimSpace(input.Reference)
	if input.Reference == "" || input.PaidAt.IsZero() || input.PaidAt.After(time.Now()) {
		return nil, apiError(http.StatusBadRequest, "reference and an actual paid_at date are required")
	}
	tx, err := l.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	loan, err := l.LoanRepo.Get(ctx, repository.LoanRepositoryFilter{ID: &id, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	if loan.Status != "APPROVED" {
		return nil, apiError(http.StatusConflict, "only approved loans can be disbursed")
	}
	if input.Amount != loan.ApprovedAmount.Int64 || input.PaidAt.Before(loan.ReviewedAt.Time) {
		return nil, apiError(http.StatusBadRequest, "disbursement must match the approved amount and follow approval")
	}
	transaction, err := l.TransactionRepo.Create(ctx, repository.Transaction{
		MemberID: loan.MemberID, Description: "Loan disbursement", Amount: input.Amount,
		Reference: "ara_loan_" + loan.ID.String(), Type: repository.TransactionTypeLOANDISBURSEMENT, Ledger: repository.LedgerTypeLOAN,
	}, tx)
	if err != nil {
		return nil, err
	}
	if _, err := l.TransactionRepo.CreateStatus(ctx, repository.TransactionStatus{
		TransactionID: transaction.ID, ConfirmedAt: sql.NullTime{Time: input.PaidAt, Valid: true},
	}, tx); err != nil {
		return nil, err
	}
	loan.Status = "ACTIVE"
	loan.DisbursedAmount = sql.NullInt64{Int64: input.Amount, Valid: true}
	loan.DisbursedAt = sql.NullTime{Time: input.PaidAt, Valid: true}
	loan.DisbursementReference = sql.NullString{String: input.Reference, Valid: true}
	loan.DisbursedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	loan.TransactionID = uuid.NullUUID{UUID: transaction.ID, Valid: true}
	if _, err := l.LoanRepo.Update(ctx, loan, tx); err != nil {
		var duplicate *pq.Error
		if errors.As(err, &duplicate) && duplicate.Code == "23505" {
			return nil, apiError(http.StatusConflict, "transfer reference has already been recorded")
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l.Get(ctx, id)
}

func optionalText(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
