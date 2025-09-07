package repository

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
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
	TransactionID *uuid.UUID
	MemberID      *uuid.UUID
	Confirmed     *bool
	Rejected      *bool
	Type          *TransactionType
}

type Saving struct {
	TransactionID uuid.UUID       `json:"id"`
	MemberID      uuid.UUID       `json:"member_id"`
	Description   string          `json:"description"`
	Reference     string          `json:"reference"`
	Amount        int64           `json:"amount"`
	Type          TransactionType `json:"type"`
	CreatedAt     sql.NullTime    `json:"created_at"`

	ConfirmedAt sql.NullTime `json:"confirmed_at"`
	RejectedAt  sql.NullTime `json:"rejected_at"`
}

func (s *SavingRepository) applyFilter(builder sq.SelectBuilder, filter SavingRepositoryFilter) sq.SelectBuilder {
	// Now apply filters from the input `filter`
	if filter.TransactionID != nil {
		builder = builder.Where(sq.Eq{"tr.id": *filter.TransactionID})
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

	if filter.Type != nil {
		builder = builder.Where(sq.Eq{"tr.type": *filter.Type})
	}

	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"tr.member_id": *filter.MemberID})
	}

	return builder
}

// Inside SavingRepository
func (s *SavingRepository) buildQuery(filter SavingRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	var queryType QueryType = QueryTypeSelect
	var err error
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = s.psql.Select(
			"tr.id",
			"tr.member_id",
			"tr.description",
			"tr.reference",
			"tr.amount",
			"tr.type",
			"tr.created_at",
			// savings status fields
			"ss.confirmed_at",
			"ss.rejected_at",
		)
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	// Join the two tables on the transaction ID
	builder = builder.
		From("transactions tr").
		Join("savings_status ss ON tr.id = ss.transaction_id")

	// Add filters specific to savings transactions
	builder = builder.Where(sq.Eq{"tr.ledger": LedgerTypeSAVINGS})
	builder = s.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("tr.created_at:desc")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (s *SavingRepository) GetStatus(ctx context.Context, filter SavingRepositoryFilter) (*SavingsStatus, error) {
	builder := s.psql.Select(
		"ss.id",
		"ss.transaction_id",
		"ss.confirmed_at",
		"ss.rejected_at",
	).From("savings_status ss").
		Join("transactions tr ON ss.transaction_id = tr.id").
		Where(sq.Eq{"tr.ledger": LedgerTypeSAVINGS})

	// Apply filters from the input `filter`
	builder = s.applyFilter(builder, filter)
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var savingsStatus SavingsStatus
	if err := s.db.GetContext(ctx, &savingsStatus, query, args...); err != nil {
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

// Inside SavingRepository
func (s *SavingRepository) List(ctx context.Context, filter SavingRepositoryFilter, opts QueryOptions) (*ListResult[Saving], error) {
	query, args, err := s.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var savings []*Saving
	err = s.db.SelectContext(ctx, &savings, query, args...)
	if err != nil {
		return nil, err
	}

	listResult := ListResult[Saving]{
		Items: lo.Slice(savings, 0, min(len(savings), int(opts.Limit))),
	}

	if len(savings) > int(opts.Limit) {
		lastItem := lo.LastOr(savings, nil)
		if lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt.Time, lastItem.TransactionID)
			listResult.NextCursor = &nextCursor
		}
	}

	return &listResult, nil
}

func (s *SavingRepository) GetBalance(ctx context.Context, filter SavingRepositoryFilter) (int64, error) {
	builder := s.psql.Select("COALESCE(SUM(tr.amount), 0) AS balance").
		From("transactions tr").
		Join("savings_status ss ON tr.id = ss.transaction_id").
		Where(sq.Eq{"tr.ledger": LedgerTypeSAVINGS})

	builder = s.applyFilter(builder, filter)

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, err
	}

	var balance int64
	if err := s.db.GetContext(ctx, &balance, query, args...); err != nil {
		return 0, err
	}

	return balance, nil
}

func (s *SavingRepository) UpdateStatus(ctx context.Context, savingsStatus SavingsStatus, tx *sqlx.Tx) (*SavingsStatus, error) {
	builder := s.psql.Update("savings_status").
		Set("confirmed_at", savingsStatus.ConfirmedAt).
		Set("rejected_at", savingsStatus.RejectedAt).
		Where(sq.Eq{"transaction_id": savingsStatus.TransactionID}).
		Where(sq.Expr("confirmed_at IS NULL AND rejected_at IS NULL")).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updatedSavingsStatus SavingsStatus
	if tx != nil {
		err = tx.GetContext(ctx, &updatedSavingsStatus, query, args...)
		return &updatedSavingsStatus, err
	}

	err = s.db.GetContext(ctx, &updatedSavingsStatus, query, args...)
	return &updatedSavingsStatus, err
}
