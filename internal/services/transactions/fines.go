package transactions

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func (t *Transaction) ChargeFine(ctx context.Context, input *dto.FineInput) (*dto.Fine, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	_, err := t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		ID: &input.MemberID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, svc.ErrNotFound()
		}
		return nil, err
	}

	if input.Deadline.Before(time.Now()) {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "deadline must be a future date",
		}
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	fine, err := t.FineRepo.Create(ctx, &repository.Fine{
		AdminID:  actor.ID,
		MemberID: input.MemberID,
		Amount:   input.Amount,
		Reason:   input.Reason,
		Deadline: input.Deadline,
	}, tx)
	if err != nil {
		return nil, err
	}

	populatedFine, err := t.FineRepo.GetPopulated(ctx, repository.FineRepositoryFilter{
		ID: &fine.ID,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// TODO: Send notification to member about the fine charged

	return t.FineRepo.MapRepositoryToDTOModel(populatedFine), nil
}

func (t *Transaction) PayFine(ctx context.Context, fineID uuid.UUID, txInput *dto.TransactionsInput) (*dto.Fine, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	member, err := t.getMemberByUserID(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	fine, err := t.FineRepo.GetPopulated(ctx, repository.FineRepositoryFilter{
		ID:       &fineID,
		MemberID: &member.ID,
	}, tx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, svc.ErrNotFound()
		}
		return nil, err
	}

	if fine.PaidAt.Valid {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "fine already paid",
		}
	}

	if fine.Amount != txInput.Amount {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "payment amount does not match fine amount",
		}
	}

	description := "Fine payment"
	if txInput.Description != "" {
		description = txInput.Description
	}

	createdTxn, err := t.createTransactionWithStatus(ctx, member.ID, TransactionParams{
		Input: dto.TransactionsInput{
			Amount:      fine.Amount,
			Description: description,
		},
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: repository.LedgerTypeFINES,
	}, tx)
	if err != nil {
		return nil, err
	}

	updatedFine, err := t.FineRepo.Update(ctx, &repository.Fine{
		ID:            fine.ID,
		AdminID:       fine.AdminID,
		MemberID:      fine.MemberID,
		TransactionID: uuid.NullUUID{UUID: createdTxn.ID, Valid: true},
		Amount:        fine.Amount,
		Reason:        fine.Reason,
		Deadline:      fine.Deadline,
	}, tx)
	if err != nil {
		return nil, err
	}

	populatedFine, err := t.FineRepo.GetPopulated(ctx, repository.FineRepositoryFilter{
		ID: &updatedFine.ID,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return t.FineRepo.MapRepositoryToDTOModel(populatedFine), nil
}

func (t *Transaction) ListFines(ctx context.Context, filters *dto.FineFilter, options *dto.QueryOptions) (*dto.ListResponse[dto.Fine], error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	repoFilters := repository.FineRepositoryFilter{
		Paid: filters.Paid,
	}

	if actor.IsAuthenticatedAsAdmin {
		if filters.MemberID != nil {
			repoFilters.MemberID = filters.MemberID
		}
	} else {
		memberID, err := t.getMemberByUserID(ctx, actor.ID)
		if err != nil {
			return nil, err
		}

		if filters.MemberID != nil && *filters.MemberID != memberID.ID {
			return nil, &svc.APIError{
				Status:  http.StatusForbidden,
				Message: "cannot access fines of other members",
			}
		}

		repoFilters.MemberID = &memberID.ID
	}

	result, err := t.FineRepo.ListPopulated(ctx, repoFilters, repository.QueryOptions{
		Limit:  options.Limit,
		Cursor: options.Cursor,
		Sort:   options.Sort,
	})
	if err != nil {
		return nil, err
	}

	dtoItems := lo.Map(result.Items, func(item *repository.PopulatedFine, _ int) dto.Fine {
		return *t.FineRepo.MapRepositoryToDTOModel(item)
	})

	return &dto.ListResponse[dto.Fine]{
		Items:      dtoItems,
		NextCursor: result.NextCursor,
	}, nil
}
