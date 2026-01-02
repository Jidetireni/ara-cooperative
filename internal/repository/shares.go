package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type ShareRepository struct {
	db              *sqlx.DB
	psql            sq.StatementBuilderType
	transactionRepo *TransactionRepository
}

func NewShareRepository(db *sqlx.DB, transactionRepo *TransactionRepository) *ShareRepository {
	return &ShareRepository{
		db:              db,
		psql:            sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		transactionRepo: transactionRepo,
	}
}

// PopulatedShare contains share with joined transaction, status, and member data
type PopulatedShare struct {
	Share
	Transaction *PopulatedTransaction
}

type populatedShareFlat struct {
	// Share fields
	ShareID            uuid.UUID `json:"s_id"`
	ShareTransactionID uuid.UUID `json:"s_transaction_id"`
	ShareUnits         string    `json:"s_units"`
	ShareUnitPrice     int64     `json:"s_unit_price"`
	ShareCreatedAt     time.Time `json:"s_created_at"`

	// Transaction fields
	TrID          *uuid.UUID       `json:"tr_id"`
	TrMemberID    *uuid.UUID       `json:"tr_member_id"`
	TrDescription *string          `json:"tr_description"`
	TrReference   *string          `json:"tr_reference"`
	TrAmount      *int64           `json:"tr_amount"`
	TrType        *TransactionType `json:"tr_type"`
	TrLedger      *LedgerType      `json:"tr_ledger"`
	TrCreatedAt   *time.Time       `json:"tr_created_at"`

	// Transaction Status fields
	TsID          *uuid.UUID `json:"ts_id"`
	TsConfirmedAt *time.Time `json:"ts_confirmed_at"`
	TsRejectedAt  *time.Time `json:"ts_rejected_at"`
	TsCreatedAt   *time.Time `json:"ts_created_at"`

	// Member fields
	MbID             uuid.UUID  `json:"mb_id"`
	MbUserID         uuid.UUID  `json:"mb_user_id"`
	MbFirstName      string     `json:"mb_first_name"`
	MbLastName       string     `json:"mb_last_name"`
	MbSlug           string     `json:"mb_slug"`
	MbPhone          string     `json:"mb_phone"`
	MbAddress        *string    `json:"mb_address"`
	MbNextOfKinName  *string    `json:"mb_next_of_kin_name"`
	MbNextOfKinPhone *string    `json:"mb_next_of_kin_phone"`
	MbActivatedAt    *time.Time `json:"mb_activated_at"`
	MbCreatedAt      *time.Time `json:"mb_created_at"`
	MbUpdatedAt      *time.Time `json:"mb_updated_at"`
	MbDeletedAt      *time.Time `json:"mb_deleted_at"`
}

type ShareRepositoryFilter struct {
	ID            *uuid.UUID
	TransactionID *uuid.UUID
	MemberID      *uuid.UUID
	Confirmed     *bool
	Rejected      *bool
	Type          *TransactionType
	LedgerType    *LedgerType
}

func (s *ShareRepository) populatedSelectColumns() []string {
	return []string{
		// Share fields
		"s.id AS s_id",
		"s.transaction_id AS s_transaction_id",
		"s.units AS s_units",
		"s.unit_price AS s_unit_price",
		"s.created_at AS s_created_at",

		// Transaction fields
		"tr.id AS tr_id",
		"tr.member_id AS tr_member_id",
		"tr.description AS tr_description",
		"tr.reference AS tr_reference",
		"tr.amount AS tr_amount",
		"tr.type AS tr_type",
		"tr.ledger AS tr_ledger",
		"tr.created_at AS tr_created_at",

		// Transaction Status fields
		"ts.id AS ts_id",
		"ts.confirmed_at AS ts_confirmed_at",
		"ts.rejected_at AS ts_rejected_at",
		"ts.created_at AS ts_created_at",

		// Member fields
		"mb.id AS mb_id",
		"mb.user_id AS mb_user_id",
		"mb.first_name AS mb_first_name",
		"mb.last_name AS mb_last_name",
		"mb.slug AS mb_slug",
		"mb.phone AS mb_phone",
		"mb.address AS mb_address",
		"mb.next_of_kin_name AS mb_next_of_kin_name",
		"mb.next_of_kin_phone AS mb_next_of_kin_phone",
		"mb.activated_at AS mb_activated_at",
		"mb.created_at AS mb_created_at",
		"mb.updated_at AS mb_updated_at",
		"mb.deleted_at AS mb_deleted_at",
	}
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

func (s *ShareRepository) buildPopulatedQuery(filter ShareRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	var err error

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = s.psql.Select(s.populatedSelectColumns()...)
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	builder = builder.From("shares s").
		Join("transactions tr ON s.transaction_id = tr.id").
		Join("transaction_status ts ON tr.id = ts.transaction_id").
		Join("members mb ON tr.member_id = mb.id")

	if filter.LedgerType != nil {
		builder = builder.Where(sq.Eq{"tr.ledger": *filter.LedgerType})
	}
	builder = s.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("s.created_at:desc")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (s *ShareRepository) GetPopulated(ctx context.Context, filter ShareRepositoryFilter, tx *sqlx.Tx) (*PopulatedShare, error) {
	query, args, err := s.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var flat populatedShareFlat
	if tx != nil {
		err = tx.GetContext(ctx, &flat, query, args...)
		return s.mapFlatToPopulated(&flat), err
	}

	err = s.db.GetContext(ctx, &flat, query, args...)
	return s.mapFlatToPopulated(&flat), err
}

func (s *ShareRepository) ListPopulated(ctx context.Context, filter ShareRepositoryFilter, opts QueryOptions) (*ListResult[PopulatedShare], error) {
	query, args, err := s.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var flatList []populatedShareFlat
	if err := s.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}

	populatedList := lo.Map(flatList, func(flat populatedShareFlat, _ int) *PopulatedShare {
		return s.mapFlatToPopulated(&flat)
	})

	listResult := ListResult[PopulatedShare]{
		Items: lo.Slice(populatedList, 0, min(len(populatedList), int(opts.Limit))),
	}

	if len(populatedList) > int(opts.Limit) {
		lastItem := lo.LastOr(populatedList, nil)
		if lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt, lastItem.ID)
			listResult.NextCursor = &nextCursor
		}
	}

	return &listResult, nil
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

type SharesTotalRows struct {
	Units  string `json:"units"`
	Amount int64  `json:"amount"`
}

func (s *ShareRepository) CountTotalSharesPurchased(ctx context.Context, filter ShareRepositoryFilter) (*SharesTotalRows, error) {
	builder := s.psql.Select(
		"COALESCE(SUM(CAST(s.units AS DECIMAL)), 0) AS units",
		"COALESCE(SUM(tr.amount), 0) AS amount",
	).From("shares s").
		Join("transactions tr ON tr.id = s.transaction_id").
		Join("transaction_status ts ON tr.id = ts.transaction_id")

	if filter.LedgerType != nil {
		builder = builder.Where(sq.Eq{"tr.ledger": *filter.LedgerType})
	}
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

func (s *ShareRepository) mapFlatToPopulated(flat *populatedShareFlat) *PopulatedShare {
	share := Share{
		ID:            flat.ShareID,
		TransactionID: flat.ShareTransactionID,
		Units:         flat.ShareUnits,
		UnitPrice:     flat.ShareUnitPrice,
		CreatedAt:     flat.ShareCreatedAt,
	}

	member := Member{
		ID:             flat.MbID,
		UserID:         flat.MbUserID,
		FirstName:      flat.MbFirstName,
		LastName:       flat.MbLastName,
		Slug:           flat.MbSlug,
		Phone:          flat.MbPhone,
		Address:        ToNullString(flat.MbAddress),
		NextOfKinName:  ToNullString(flat.MbNextOfKinName),
		NextOfKinPhone: ToNullString(flat.MbNextOfKinPhone),
		ActivatedAt:    ToNullTime(flat.MbActivatedAt),
		CreatedAt:      lo.FromPtrOr(flat.MbCreatedAt, time.Time{}),
		UpdatedAt:      ToNullTime(flat.MbUpdatedAt),
		DeletedAt:      ToNullTime(flat.MbDeletedAt),
	}

	populated := &PopulatedShare{
		Share: share,
	}

	if flat.TrID != nil {
		populated.Transaction = &PopulatedTransaction{
			Transaction: Transaction{
				ID:          *flat.TrID,
				MemberID:    lo.FromPtrOr(flat.TrMemberID, uuid.Nil),
				Description: lo.FromPtrOr(flat.TrDescription, ""),
				Reference:   lo.FromPtrOr(flat.TrReference, ""),
				Amount:      lo.FromPtrOr(flat.TrAmount, 0),
				Type:        lo.FromPtrOr(flat.TrType, ""),
				Ledger:      lo.FromPtrOr(flat.TrLedger, ""),
				CreatedAt:   ToNullTime(flat.TrCreatedAt),
			},
			Status: TransactionStatus{
				ID:          lo.FromPtrOr(flat.TsID, uuid.Nil),
				ConfirmedAt: ToNullTime(flat.TsConfirmedAt),
				RejectedAt:  ToNullTime(flat.TsRejectedAt),
				CreatedAt:   ToNullTime(flat.TsCreatedAt),
			},
			Member: member,
		}
	}

	return populated
}

func (s *ShareRepository) MapRepositoryToDTOModel(populated *PopulatedShare) *dto.Shares {
	if populated == nil {
		return nil
	}

	units, err := strconv.ParseFloat(populated.Units, 64)
	if err != nil {
		return nil
	}

	result := &dto.Shares{
		ID:        populated.ID,
		Units:     units,
		UnitPrice: populated.UnitPrice,
		CreatedAt: populated.CreatedAt,
	}

	if populated.Transaction != nil {
		result.Transaction = *s.transactionRepo.MapRepositoryToDTOModel(populated.Transaction)
	}

	return result
}
