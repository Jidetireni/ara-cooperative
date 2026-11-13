package loans

import (
	"context"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// GuarantorRequests lists the loan applications that name the current member
// as a guarantor, newest first.
func (l *Loan) GuarantorRequests(
	ctx context.Context, opts *dto.QueryOptions,
) (*dto.ListResponse[dto.GuarantorRequest], error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.LoanRepo.ListGuarantorRequests(ctx, member.ID, repository.QueryOptions{
		Limit: opts.Limit, Cursor: opts.Cursor, Sort: opts.Sort,
	})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.GuarantorRequest]{
		Items:      lo.Map(result.Items, func(g *repository.PopulatedLoanGuarantor, _ int) dto.GuarantorRequest { return mapRequest(g) }),
		NextCursor: result.NextCursor,
	}, nil
}

// GuaranteeRequest returns the current member's guarantee request on one loan,
// for the page a guarantor opens from the request email.
func (l *Loan) GuaranteeRequest(ctx context.Context, loanID uuid.UUID) (*dto.GuarantorRequest, error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	guarantors, err := l.LoanRepo.ListGuarantors(ctx, repository.LoanGuarantorRepositoryFilter{
		LoanID: &loanID, MemberID: &member.ID,
	}, nil)
	if err != nil {
		return nil, err
	}
	if len(guarantors) == 0 {
		return nil, apiError(http.StatusNotFound, "you are not a guarantor on this loan application")
	}
	request := mapRequest(guarantors[0])
	return &request, nil
}

func mapRequest(g *repository.PopulatedLoanGuarantor) dto.GuarantorRequest {
	request := dto.GuarantorRequest{
		LoanID: g.LoanID, Applicant: memberCard(g.Applicant), RequestedAmount: g.Loan.RequestedAmount,
		Purpose: g.Loan.Purpose, RequestedDurationMonths: g.Loan.RequestedDurationMonths, LoanStatus: g.Loan.Status,
		Position: g.Position, Status: g.Status, CreatedAt: g.CreatedAt,
	}
	if g.RespondedAt.Valid {
		request.RespondedAt = &g.RespondedAt.Time
	}
	return request
}

// RespondAsGuarantor records the current member's acceptance or refusal to
// guarantee a pending loan application. It replaces the form's signature.
func (l *Loan) RespondAsGuarantor(
	ctx context.Context, loanID uuid.UUID, input dto.LoanGuaranteeInput,
) (*dto.GuarantorRequest, error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	status := map[string]string{"ACCEPT": "ACCEPTED", "DECLINE": "DECLINED"}[input.Decision]
	if status == "" {
		return nil, apiError(http.StatusBadRequest, "decision must be ACCEPT or DECLINE")
	}
	tx, err := l.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	guarantors, err := l.LoanRepo.ListGuarantors(ctx, repository.LoanGuarantorRepositoryFilter{
		LoanID: &loanID, MemberID: &member.ID, ForUpdate: true,
	}, tx)
	if err != nil {
		return nil, err
	}
	if len(guarantors) == 0 {
		return nil, apiError(http.StatusNotFound, "you are not a guarantor on this loan application")
	}
	guarantor := guarantors[0]
	if guarantor.Loan.Status != "PENDING" {
		return nil, apiError(http.StatusConflict, "guarantors can respond only while the application is pending")
	}
	now := time.Now()
	if err := l.LoanRepo.UpdateGuarantorStatus(ctx, guarantor.ID, status, now, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	guarantor.Status = status
	guarantor.RespondedAt.Time, guarantor.RespondedAt.Valid = now, true
	request := mapRequest(guarantor)
	return &request, nil
}
