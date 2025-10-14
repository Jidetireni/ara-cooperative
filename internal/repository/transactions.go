package repository

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type TransactionRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewTransactionRepository(db *sqlx.DB) *TransactionRepository {
	return &TransactionRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type TransactionRepositoryFilter struct {
	ID         *uuid.UUID
	MemberID   *uuid.UUID
	StatusID   *uuid.UUID
	Confirmed  *bool
	Rejected   *bool
	Type       *TransactionType
	LedgerType *LedgerType
}

type PopTransaction struct {
	ID          uuid.UUID       `json:"id"`
	MemberID    uuid.UUID       `json:"member_id"`
	Description string          `json:"description"`
	Reference   string          `json:"reference"`
	Amount      int64           `json:"amount"`
	Type        TransactionType `json:"type"`
	LedgerType  LedgerType      `json:"ledger_type"`
	CreatedAt   sql.NullTime    `json:"created_at"`
	StatusID    uuid.UUID       `json:"status_id"`
	ConfirmedAt sql.NullTime    `json:"confirmed_at"`
	RejectedAt  sql.NullTime    `json:"rejected_at"`
}

func (s *TransactionRepository) applyFilter(builder sq.SelectBuilder, filter TransactionRepositoryFilter) sq.SelectBuilder {
	// Now apply filters from the input `filter`
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"tr.id": *filter.ID})
	}

	if filter.Confirmed != nil {
		if *filter.Confirmed {
			builder = builder.Where("ts.confirmed_at IS NOT NULL")
		} else {
			builder = builder.Where("ts.confirmed_at IS NULL")
		}
	}

	if filter.Rejected != nil {
		if *filter.Rejected {
			builder = builder.Where("ts.rejected_at IS NOT NULL")
		} else {
			builder = builder.Where("ts.rejected_at IS NULL")
		}
	}

	if filter.StatusID != nil {
		builder = builder.Where(sq.Eq{"ts.id": *filter.StatusID})
	}

	if filter.Type != nil {
		builder = builder.Where(sq.Eq{"tr.type": *filter.Type})
	}

	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"tr.member_id": *filter.MemberID})
	}

	if filter.LedgerType != nil {
		builder = builder.Where(sq.Eq{"tr.ledger": *filter.LedgerType})
	}

	return builder
}

func (s *TransactionRepository) buildQuery(filter TransactionRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
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
			"tr.ledger AS ledger_type",
			"tr.created_at",
			// transaction status fields
			"ts.id AS status_id",
			"ts.confirmed_at",
			"ts.rejected_at",
		)
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	// Join the two tables on the transaction ID
	builder = builder.
		From("transactions tr").
		Join("transaction_status ts ON tr.id = ts.transaction_id")

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

func (t TransactionRepository) Create(ctx context.Context, transaction Transaction, tx *sqlx.Tx) (*Transaction, error) {
	builder := t.psql.Insert("transactions").
		Columns("member_id", "description", "reference", "amount", "type", "ledger").
		Values(transaction.MemberID, transaction.Description, transaction.Reference, transaction.Amount, transaction.Type, transaction.Ledger).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdTransaction Transaction
	if tx != nil {
		err = tx.GetContext(ctx, &createdTransaction, query, args...)
		return &createdTransaction, err
	}

	err = t.db.GetContext(ctx, &createdTransaction, query, args...)
	return &createdTransaction, err
}

func (t TransactionRepository) Get(ctx context.Context, filter TransactionRepositoryFilter) (*PopTransaction, error) {
	query, args, err := t.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var popTxn PopTransaction
	err = t.db.GetContext(ctx, &popTxn, query, args...)
	if err != nil {
		return nil, err
	}

	return &popTxn, nil
}

func (s *TransactionRepository) List(ctx context.Context, filter TransactionRepositoryFilter, opts QueryOptions) (*ListResult[PopTransaction], error) {
	query, args, err := s.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var transactions []*PopTransaction
	err = s.db.SelectContext(ctx, &transactions, query, args...)
	if err != nil {
		return nil, err
	}

	listResult := ListResult[PopTransaction]{
		Items: lo.Slice(transactions, 0, min(len(transactions), int(opts.Limit))),
	}

	if len(transactions) > int(opts.Limit) {
		lastItem := lo.LastOr(transactions, nil)
		if lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt.Time, lastItem.ID)
			listResult.NextCursor = &nextCursor
		}
	}

	return &listResult, nil
}

func (s *TransactionRepository) GetStatus(ctx context.Context, filter TransactionRepositoryFilter) (*TransactionStatus, error) {
	builder := s.psql.Select(
		"ts.id",
		"ts.transaction_id",
		"ts.confirmed_at",
		"ts.rejected_at",
	).From("transaction_status ts").
		Join("transactions tr ON ts.transaction_id = tr.id")

	// Apply filters from the input `filter`
	builder = s.applyFilter(builder, filter)
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var TransactionStatus TransactionStatus
	if err := s.db.GetContext(ctx, &TransactionStatus, query, args...); err != nil {
		return nil, err
	}

	return &TransactionStatus, nil
}

func (s *TransactionRepository) CreateStatus(ctx context.Context, transactionStatus TransactionStatus, tx *sqlx.Tx) (*TransactionStatus, error) {
	builder := s.psql.Insert("transaction_status").
		Columns("transaction_id", "confirmed_at", "rejected_at").
		Values(transactionStatus.TransactionID, transactionStatus.ConfirmedAt, transactionStatus.RejectedAt).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdStatus TransactionStatus
	if tx != nil {
		err = tx.GetContext(ctx, &createdStatus, query, args...)
		return &createdStatus, err
	}

	err = s.db.GetContext(ctx, &createdStatus, query, args...)
	return &createdStatus, err
}

func (s *TransactionRepository) GetBalance(ctx context.Context, filter TransactionRepositoryFilter) (int64, error) {
	builder := s.psql.Select("COALESCE(SUM(tr.amount), 0) AS balance").
		From("transactions tr").
		Join("transaction_status ts ON tr.id = ts.transaction_id").
		Where(sq.Eq{"tr.ledger": filter.LedgerType})

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

func (s *TransactionRepository) UpdateStatus(ctx context.Context, transactionStatus TransactionStatus, tx *sqlx.Tx) (*TransactionStatus, error) {
	builder := s.psql.Update("transaction_status").
		Set("confirmed_at", transactionStatus.ConfirmedAt).
		Set("rejected_at", transactionStatus.RejectedAt).
		Where(sq.Eq{"id": transactionStatus.ID}).
		Where(sq.Expr("confirmed_at IS NULL AND rejected_at IS NULL")).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updatedTransactionStatus TransactionStatus
	if tx != nil {
		err = tx.GetContext(ctx, &updatedTransactionStatus, query, args...)
		return &updatedTransactionStatus, err
	}

	err = s.db.GetContext(ctx, &updatedTransactionStatus, query, args...)
	return &updatedTransactionStatus, err
}
