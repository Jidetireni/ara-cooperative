package repository

import (
	"context"

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
	ID        *uuid.UUID
	MemberID  *uuid.UUID
	Type      *TransactionType
	Ledger    *LedgerType
	Reference *string
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
		builder = s.psql.Select("*")
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	builder = builder.From("transactions tr")

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"tr.id": *filter.ID})
	}

	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"tr.member_id": *filter.MemberID})
	}

	if filter.Type != nil {
		builder = builder.Where(sq.Eq{"tr.type": *filter.Type})
	}

	if filter.Ledger != nil {
		builder = builder.Where(sq.Eq{"tr.ledger": *filter.Ledger})
	}

	if filter.Reference != nil {
		builder = builder.Where(sq.Eq{"tr.reference": *filter.Reference})
	}

	if queryType != QueryTypeCount {
		opts.Sort = lo.ToPtr("tr.created_at:desc")
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

	err = t.db.GetContext(ctx, &transaction, query, args...)
	return &createdTransaction, err
}

func (t TransactionRepository) Get(ctx context.Context, filter TransactionRepositoryFilter) (*Transaction, error) {
	query, args, err := t.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var transaction Transaction
	err = t.db.GetContext(ctx, &transaction, query, args...)
	if err != nil {
		return nil, err
	}

	return &transaction, nil
}
