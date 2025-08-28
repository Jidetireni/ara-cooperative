package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type SavingRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewSavingRepository(db *sqlx.DB) *SavingRepository {
	return &SavingRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type SavingRepositoryFilter struct {
	ID        *uuid.UUID
	Confirmed *bool
	Rejected  *bool
}

func (s *SavingRepository) buildQuery(filter SavingRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	var queryType QueryType = QueryTypeSelect
	var err error
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = s.psql.Select("*")
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}
	builder = builder.From("savings_status ss")

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"ss.id": *filter.ID})
	}

	if filter.Confirmed != nil {
		if *filter.Confirmed {
			builder = builder.Where(sq.NotEq{"ss.confirmed_at": nil})
		} else {
			builder = builder.Where(sq.Eq{"ss.confirmed_at": nil})
		}
	}

	if filter.Rejected != nil {
		if *filter.Rejected {
			builder = builder.Where(sq.NotEq{"ss.rejected_at": nil})
		} else {
			builder = builder.Where(sq.Eq{"ss.rejected_at": nil})
		}
	}

	if queryType != QueryTypeCount {
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (s *SavingRepository) GetStatus(ctx context.Context, filter SavingRepositoryFilter) (*SavingsStatus, error) {
	query, args, err := s.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var savingsStatus SavingsStatus
	err = s.db.GetContext(ctx, &savingsStatus, query, args...)
	if err != nil {
		return nil, err
	}

	return &savingsStatus, nil
}

func (s *SavingRepository) CreateStatus(ctx context.Context, savingStatus SavingsStatus, tx *sqlx.Tx) (*SavingsStatus, error) {
	builder := s.psql.Insert("savings_status").
		Columns("transaction_id", "confirmed_at", "rejected_at").
		Values(savingStatus.TransactionID, savingStatus.ConfirmedAt, savingStatus.RejectedAt).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdSavingsStatus SavingsStatus
	if tx != nil {
		err = tx.GetContext(ctx, &createdSavingsStatus, query, args...)
		return &createdSavingsStatus, err
	}

	err = s.db.GetContext(ctx, &createdSavingsStatus, query, args...)
	return &createdSavingsStatus, err
}
