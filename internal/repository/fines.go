package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type FineRepository struct {
	db                    *sqlx.DB
	psql                  sq.StatementBuilderType
	transactionRepository *TransactionRepository
}

func NewFineRepository(db *sqlx.DB) *FineRepository {
	return &FineRepository{
		db:                    db,
		psql:                  sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		transactionRepository: NewTransactionRepository(db),
	}
}

// PopulatedFine contains fine with joined transaction, status, and member data
type PopulatedFine struct {
	Fine
	Transaction *PopulatedTransaction
}

type populatedFineFlat struct {
	FineID            uuid.UUID     `json:"f_id"`
	FineAdminID       uuid.UUID     `json:"f_admin_id"`
	FineMemberID      uuid.UUID     `json:"f_member_id"`
	FineTransactionID uuid.NullUUID `json:"f_transaction_id"`
	FineAmount        int64         `json:"f_amount"`
	FineReason        string        `json:"f_reason"`
	FineDeadline      time.Time     `json:"f_deadline"`
	FinePaidAt        sql.NullTime  `json:"f_paid_at"`
	FineCreatedAt     time.Time     `json:"f_created_at"`
	FineUpdatedAt     sql.NullTime  `json:"f_updated_at"`

	TrID          *uuid.UUID       `json:"tr_id"`
	TrMemberID    *uuid.UUID       `json:"tr_member_id"`
	TrDescription *string          `json:"tr_description"`
	TrReference   *string          `json:"tr_reference"`
	TrAmount      *int64           `json:"tr_amount"`
	TrType        *TransactionType `json:"tr_type"`
	TrLedger      *LedgerType      `json:"tr_ledger"`
	TrCreatedAt   *time.Time       `json:"tr_created_at"`

	TsID          *uuid.UUID `json:"ts_id"`
	TsConfirmedAt *time.Time `json:"ts_confirmed_at"`
	TsRejectedAt  *time.Time `json:"ts_rejected_at"`
	TsCreatedAt   *time.Time `json:"ts_created_at"`

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

type FineRepositoryFilter struct {
	ID            *uuid.UUID
	AdminID       *uuid.UUID
	MemberID      *uuid.UUID
	TransactionID *uuid.UUID
	Paid          *bool
}

func (f *FineRepository) populatedSelectColumns() []string {
	return []string{
		// Fine fields
		"f.id AS f_id",
		"f.admin_id AS f_admin_id",
		"f.member_id AS f_member_id",
		"f.transaction_id AS f_transaction_id",
		"f.amount AS f_amount",
		"f.reason AS f_reason",
		"f.deadline AS f_deadline",
		"f.paid_at AS f_paid_at",
		"f.created_at AS f_created_at",
		"f.updated_at AS f_updated_at",

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

func (f *FineRepository) applyFilter(builder sq.SelectBuilder, filter FineRepositoryFilter) sq.SelectBuilder {
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
	return builder
}

func (f *FineRepository) buildPopulatedQuery(filter FineRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	var err error

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = f.psql.Select(f.populatedSelectColumns()...)
	case QueryTypeCount:
		builder = f.psql.Select("COUNT(*)")
	}

	builder = builder.From("fines f").
		Join("members mb ON f.member_id = mb.id").
		LeftJoin("transactions tr ON f.transaction_id = tr.id").
		LeftJoin("transaction_status ts ON tr.id = ts.transaction_id")

	builder = f.applyFilter(builder, filter)

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

func (f *FineRepository) GetPopulated(ctx context.Context, filter FineRepositoryFilter, tx *sqlx.Tx) (*PopulatedFine, error) {
	query, args, err := f.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var flat populatedFineFlat
	if tx != nil {
		err = tx.GetContext(ctx, &flat, query, args...)
		return f.mapFlatToPopulated(&flat), err
	}

	err = f.db.GetContext(ctx, &flat, query, args...)

	return f.mapFlatToPopulated(&flat), err
}

func (f *FineRepository) ListPopulated(ctx context.Context, filter FineRepositoryFilter, opts QueryOptions) (*ListResult[PopulatedFine], error) {
	query, args, err := f.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var flatList []populatedFineFlat
	if err := f.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}

	populatedList := lo.Map(flatList, func(flat populatedFineFlat, _ int) *PopulatedFine {
		return f.mapFlatToPopulated(&flat)
	})

	listResult := ListResult[PopulatedFine]{
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

func (f *FineRepository) Create(ctx context.Context, fine *Fine, tx *sqlx.Tx) (*Fine, error) {
	builder := f.psql.Insert("fines").
		Columns("admin_id", "member_id", "amount", "reason", "deadline").
		Values(fine.AdminID, fine.MemberID, fine.Amount, fine.Reason, fine.Deadline).
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

func (f *FineRepository) mapFlatToPopulated(flat *populatedFineFlat) *PopulatedFine {
	fine := Fine{
		ID:            flat.FineID,
		AdminID:       flat.FineAdminID,
		MemberID:      flat.FineMemberID,
		TransactionID: flat.FineTransactionID,
		Amount:        flat.FineAmount,
		Reason:        flat.FineReason,
		Deadline:      flat.FineDeadline,
		PaidAt:        flat.FinePaidAt,
		CreatedAt:     flat.FineCreatedAt,
		UpdatedAt:     flat.FineUpdatedAt,
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

	populated := &PopulatedFine{
		Fine: fine,
	}

	var status TransactionStatus
	if flat.TsID != nil {
		status = TransactionStatus{
			ID:          lo.FromPtrOr(flat.TsID, uuid.Nil),
			ConfirmedAt: ToNullTime(flat.TsConfirmedAt),
			RejectedAt:  ToNullTime(flat.TsRejectedAt),
			CreatedAt:   ToNullTime(flat.TsCreatedAt),
		}
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
			Status: status,
			Member: member,
		}

	}

	return populated
}

func (f *FineRepository) MapRepositoryToDTOModel(populated *PopulatedFine) *dto.Fine {
	if populated == nil {
		return nil
	}

	result := &dto.Fine{
		ID:       populated.ID,
		Amount:   populated.Amount,
		Reason:   populated.Reason,
		Deadline: populated.Deadline,
		Paid:     populated.PaidAt.Valid,
	}

	if populated.Transaction != nil {
		result.Transaction = f.transactionRepository.MapRepositoryToDTOModel(populated.Transaction)
	}

	return result
}
