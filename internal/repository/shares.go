package repository

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type ShareRepository struct {
	db          *sqlx.DB
	psql        sq.StatementBuilderType
	transaction *TransactionRepository
}

func NewShareRepository(db *sqlx.DB, transactionRepo *TransactionRepository) *ShareRepository {
	return &ShareRepository{
		db:          db,
		psql:        sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		transaction: transactionRepo,
	}
}

type PopShare struct {
	ID            uuid.UUID       `json:"id"`
	TransactionID uuid.UUID       `json:"transaction_id"` // fixed tag
	MemberID      uuid.UUID       `json:"member_id"`
	Description   string          `json:"description"`
	Reference     string          `json:"reference"`
	Amount        int64           `json:"amount"`
	Type          TransactionType `json:"type"`
	Units         string          `json:"units"`
	UnitPrice     int64           `json:"unit_price"`
	CreatedAt     time.Time       `json:"created_at"`

	ConfirmedAt sql.NullTime `json:"confirmed_at"`
	RejectedAt  sql.NullTime `json:"rejected_at"`
}

type ShareRepositoryFilter struct {
	ID            *uuid.UUID
	TransactionID *uuid.UUID
	// Optional convenience filter: filter shares by the member that owns the transaction.
	MemberID   *uuid.UUID
	Confirmed  *bool
	Rejected   *bool
	Type       *TransactionType
	LedgerType LedgerType
}

func (s *ShareRepository) applyFilter(builder sq.SelectBuilder, filter ShareRepositoryFilter) sq.SelectBuilder {
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"s.id": *filter.ID})
	}
	if filter.TransactionID != nil {
		builder = builder.Where(sq.Eq{"s.transaction_id": *filter.TransactionID})
	}
	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"tr.member_id": *filter.MemberID})
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

	if filter.Type != nil {
		builder = builder.Where(sq.Eq{"tr.type": *filter.Type})
	}

	return builder
}

func (s *ShareRepository) buildQuery(filter ShareRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	var queryType QueryType = QueryTypeSelect
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = s.psql.Select(
			"s.id",
			"s.transaction_id",
			"tr.member_id",
			"tr.description",
			"tr.reference",
			"tr.amount",
			"tr.type",
			"s.units",
			"s.unit_price",
			"s.created_at",
			"ts.confirmed_at",
			"ts.rejected_at",
		)
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	builder = builder.From("shares s").
		Join("transactions tr ON tr.id = s.transaction_id").
		Join("transaction_status ts ON tr.id = ts.transaction_id")

	builder = builder.Where(sq.Eq{"tr.ledger": filter.LedgerType})
	builder = s.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("s.created_at:desc")
		}
		var err error
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (s *ShareRepository) Get(ctx context.Context, filter ShareRepositoryFilter) (*PopShare, error) {
	query, args, err := s.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var share PopShare
	if err := s.db.GetContext(ctx, &share, query, args...); err != nil {
		return nil, err
	}

	return &share, nil
}

func (s *ShareRepository) Create(ctx context.Context, share Share, tx *sqlx.Tx) (*Share, error) {
	builder := s.psql.Insert("shares").
		Columns("transaction_id", "units", "unit_price").
		Values(share.TransactionID, share.Units, share.UnitPrice).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var created Share
	if tx != nil {
		err = tx.GetContext(ctx, &created, query, args...)
		return &created, err
	}

	err = s.db.GetContext(ctx, &created, query, args...)
	return &created, err
}

func (s *ShareRepository) List(ctx context.Context, filter ShareRepositoryFilter, opts QueryOptions) (*ListResult[PopShare], error) {
	query, args, err := s.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var shares []*PopShare
	if err := s.db.SelectContext(ctx, &shares, query, args...); err != nil {
		return nil, err
	}

	list := ListResult[PopShare]{
		Items: lo.Slice(shares, 0, min(len(shares), int(opts.Limit))),
	}
	if len(shares) > int(opts.Limit) {
		last := lo.LastOr(shares, nil)
		if last != nil {
			next := EncodeCursor(last.CreatedAt, last.ID)
			list.NextCursor = &next
		}
	}
	return &list, nil
}

type SharesTotalRows struct {
	Units  string `json:"units"`
	Amount int64  `json:"amount"`
}

func (s *ShareRepository) CountTotalSharesPurchased(ctx context.Context, filter ShareRepositoryFilter) (*SharesTotalRows, error) {
	builder := s.psql.Select(
		"COALESCE(SUM(s.units), '0') AS units",
		"COALESCE(SUM(tr.amount), 0) AS amount",
	).From("shares s").
		Join("transactions tr ON tr.id = s.transaction_id").
		Join("transaction_status ts ON tr.id = ts.transaction_id")

	builder = builder.Where(sq.Eq{"tr.ledger": filter.LedgerType})
	builder = s.applyFilter(builder, filter)

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var total SharesTotalRows
	if err := s.db.GetContext(ctx, &total, query, args...); err != nil {
		return nil, err
	}

	return &total, nil
}

func (s *ShareRepository) CreateUnitPrice(ctx context.Context, price int64, tx *sqlx.Tx) error {
	builder := s.psql.Insert("share_unit_prices").
		Columns("price").
		Values(price).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *ShareRepository) GetUnitPrice(ctx context.Context) (int64, error) {
	builder := s.psql.Select("price").
		From("share_unit_prices").
		OrderBy("created_at DESC").
		Limit(1)

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, err
	}

	var price int64
	if err := s.db.GetContext(ctx, &price, query, args...); err != nil {
		return 0, err
	}

	return price, nil
}

func (s *ShareRepository) MapRepositoryToDTO(share *Share, txn *Transaction, status *TransactionStatus) *dto.Shares {
	units, err := strconv.ParseFloat(share.Units, 64)
	if err != nil {
		return nil
	}

	return &dto.Shares{
		ID:          share.ID,
		Transaction: *s.transaction.MapRepositoryToDTO(txn, status),
		Units:       units,
		UnitPrice:   share.UnitPrice,
		CreatedAt:   share.CreatedAt,
	}
}
