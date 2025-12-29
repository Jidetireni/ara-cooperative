package repository

import (
	"context"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type FineRepository struct {
	db          *sqlx.DB
	psql        sq.StatementBuilderType
	transaction *TransactionRepository
}

func NewFineRepository(db *sqlx.DB, transactionRepo *TransactionRepository) *FineRepository {
	return &FineRepository{
		db:          db,
		psql:        sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		transaction: transactionRepo,
	}
}

type FineRepositoryFilter struct {
	ID            *uuid.UUID
	AdminID       *uuid.UUID
	MemberID      *uuid.UUID
	TransactionID *uuid.UUID
	Paid          *bool
}

func (f *FineRepository) buildQuery(filter FineRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	var queryType QueryType = QueryTypeSelect
	var err error
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = f.psql.Select(
			"f.id",
			"f.admin_id",
			"f.member_id",
			"f.transaction_id",
			"f.amount",
			"f.reason",
			"f.deadline",
			"f.paid_at",
			"f.created_at",
			"f.updated_at",
		)
	case QueryTypeCount:
		builder = f.psql.Select("COUNT(*)")
	}

	builder = builder.From("fines f")

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"f.id": *filter.ID})
	}

	if filter.AdminID != nil {
		builder = builder.Where(sq.Eq{"f.admin_id": *filter.AdminID})
	}

	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"f.member_id": *filter.MemberID})
	}

	if filter.TransactionID != nil {
		builder = builder.Where(sq.Eq{"f.transaction_id": *filter.TransactionID})
	}

	if filter.Paid != nil {
		if *filter.Paid {
			builder = builder.Where(sq.NotEq{"f.paid_at": nil})
		} else {
			builder = builder.Where(sq.Eq{"f.paid_at": nil})
		}
	}

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("f.created_at:desc")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (f *FineRepository) Get(ctx context.Context, filter FineRepositoryFilter, tx *sqlx.Tx) (*Fine, error) {
	query, args, err := f.buildQuery(filter, QueryOptions{Type: lo.ToPtr(QueryTypeSelect)})
	if err != nil {
		return nil, err
	}

	var fine Fine
	if tx != nil {
		if err := tx.GetContext(ctx, &fine, query, args...); err != nil {
			return nil, err
		}
		return &fine, nil
	}

	err = f.db.GetContext(ctx, &fine, query, args...)
	return &fine, err
}

// Create inserts a new fine and returns it.
func (f *FineRepository) Create(ctx context.Context, fine *Fine, tx *sqlx.Tx) (*Fine, error) {
	builder := f.psql.Insert("fines").
		Columns("admin_id", "member_id", "transaction_id", "amount", "reason", "deadline", "paid_at").
		Values(fine.AdminID, fine.MemberID, fine.TransactionID, fine.Amount, fine.Reason, fine.Deadline, fine.PaidAt).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var created Fine
	if tx != nil {
		err = tx.GetContext(ctx, &created, query, args...)
		return &created, err
	}

	err = f.db.GetContext(ctx, &created, query, args...)
	return &created, err
}

// List returns a paginated list of fines.
func (f *FineRepository) List(ctx context.Context, filter FineRepositoryFilter, opts QueryOptions) (*ListResult[Fine], error) {
	query, args, err := f.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var fines []*Fine
	if err := f.db.SelectContext(ctx, &fines, query, args...); err != nil {
		return nil, err
	}

	list := ListResult[Fine]{
		Items: lo.Slice(fines, 0, min(len(fines), int(opts.Limit))),
	}
	if len(fines) > int(opts.Limit) {
		last := lo.LastOr(fines, nil)
		if last != nil {
			next := EncodeCursor(last.CreatedAt, last.ID)
			list.NextCursor = &next
		}
	}

	return &list, nil
}

// Update modifies an existing fine (full update of mutable fields).
func (f *FineRepository) Update(ctx context.Context, fine *Fine, tx *sqlx.Tx) (*Fine, error) {
	builder := f.psql.Update("fines").
		Set("admin_id", fine.AdminID).
		Set("member_id", fine.MemberID).
		Set("transaction_id", fine.TransactionID).
		Set("amount", fine.Amount).
		Set("reason", fine.Reason).
		Set("deadline", fine.Deadline).
		Set("paid_at", fine.PaidAt).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"id": fine.ID}).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updatedFine Fine
	if tx != nil {
		err = tx.GetContext(ctx, &updatedFine, query, args...)
		return &updatedFine, err
	}

	err = f.db.GetContext(ctx, &updatedFine, query, args...)
	return &updatedFine, err
}

// Delete removes a fine by ID.
func (f *FineRepository) Delete(ctx context.Context, id uuid.UUID, tx *sqlx.Tx) error {
	builder := f.psql.Delete("fines").
		Where(sq.Eq{"id": id})

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = f.db.ExecContext(ctx, query, args...)
	return err
}

func (f *FineRepository) MapRepositoryToDTO(fine *Fine, txn *Transaction, status *TransactionStatus) *dto.Fine {
	if fine != nil {
		return &dto.Fine{
			ID:          fine.ID,
			MemberID:    fine.MemberID,
			Transaction: f.transaction.MapRepositoryToDTO(txn, status),
			Amount:      fine.Amount,
			Reason:      fine.Reason,
			Deadline:    fine.Deadline,
			Paid:        fine.PaidAt.Valid,
		}
	}

	return nil
}
