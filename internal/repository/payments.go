package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type PaymentRepository struct {
	db               *sqlx.DB
	psql             sq.StatementBuilderType
	memberRepository *MemberRepository
}

func NewPaymentRepository(db *sqlx.DB) *PaymentRepository {
	return &PaymentRepository{
		db:               db,
		psql:             sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		memberRepository: NewMemberRepository(db),
	}
}

// PopulatedPayment is a payment with the member who started it.
type PopulatedPayment struct {
	Payment
	Member Member
}

// populatedPaymentFlat scans p.* (through the embedded Payment) plus the
// aliased member columns.
type populatedPaymentFlat struct {
	Payment
	MbID             uuid.UUID  `json:"mb_id"`
	MbUserID         uuid.UUID  `json:"mb_user_id"`
	MbFirstName      string     `json:"mb_first_name"`
	MbLastName       string     `json:"mb_last_name"`
	MbSlug           string     `json:"mb_slug"`
	MbPhone          *string    `json:"mb_phone"`
	MbAddress        *string    `json:"mb_address"`
	MbNextOfKinName  *string    `json:"mb_next_of_kin_name"`
	MbNextOfKinPhone *string    `json:"mb_next_of_kin_phone"`
	MbActivatedAt    *time.Time `json:"mb_activated_at"`
	MbCreatedAt      *time.Time `json:"mb_created_at"`
	MbUpdatedAt      *time.Time `json:"mb_updated_at"`
	MbDeletedAt      *time.Time `json:"mb_deleted_at"`
}

type PaymentRepositoryFilter struct {
	ID         *uuid.UUID
	MemberID   *uuid.UUID
	Reference  *string
	FineID     *uuid.UUID
	CampaignID *uuid.UUID
	Purpose    *LedgerType
	Status     *PaymentStatus
	ForUpdate  bool
}

func (p *PaymentRepository) applyFilter(builder sq.SelectBuilder, filter PaymentRepositoryFilter) sq.SelectBuilder {
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"p.id": *filter.ID})
	}
	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"p.member_id": *filter.MemberID})
	}
	if filter.Reference != nil {
		builder = builder.Where(sq.Eq{"p.reference": *filter.Reference})
	}
	if filter.FineID != nil {
		builder = builder.Where(sq.Eq{"p.fine_id": *filter.FineID})
	}
	if filter.CampaignID != nil {
		builder = builder.Where(sq.Eq{"p.campaign_id": *filter.CampaignID})
	}
	if filter.Purpose != nil {
		builder = builder.Where(sq.Eq{"p.purpose": *filter.Purpose})
	}
	if filter.Status != nil {
		builder = builder.Where(sq.Eq{"p.status": *filter.Status})
	}
	return builder
}

func (p *PaymentRepository) populatedSelectColumns() []string {
	return []string{
		"p.*",
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

func (p *PaymentRepository) buildQuery(filter PaymentRepositoryFilter, opts QueryOptions) (string, []any, error) {
	return p.generateQuery(filter, opts, false)
}

func (p *PaymentRepository) buildPopulatedQuery(filter PaymentRepositoryFilter, opts QueryOptions) (string, []any, error) {
	return p.generateQuery(filter, opts, true)
}

func (p *PaymentRepository) generateQuery(filter PaymentRepositoryFilter, opts QueryOptions, populated bool) (string, []any, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	var err error

	var builder sq.SelectBuilder
	switch {
	case queryType == QueryTypeCount:
		builder = p.psql.Select("COUNT(*)").From("payments p")
	case populated:
		builder = p.psql.Select(p.populatedSelectColumns()...).From("payments p").
			Join("members mb ON p.member_id = mb.id")
	default:
		builder = p.psql.Select("p.*").From("payments p")
	}

	builder = p.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		opts.Sort, err = SortFor(opts.Sort, "p")
		if err != nil {
			return "", nil, err
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	if filter.ForUpdate && queryType == QueryTypeSelect {
		builder = builder.Suffix("FOR UPDATE")
	}
	return builder.ToSql()
}

func (p *PaymentRepository) Get(ctx context.Context, filter PaymentRepositoryFilter, tx *sqlx.Tx) (*Payment, error) {
	query, args, err := p.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var payment Payment
	if tx != nil {
		if err := tx.GetContext(ctx, &payment, query, args...); err != nil {
			return nil, err
		}
		return &payment, nil
	}
	if filter.ForUpdate {
		return nil, errors.New("row locking requires a transaction")
	}
	if err := p.db.GetContext(ctx, &payment, query, args...); err != nil {
		return nil, err
	}
	return &payment, nil
}

func (p *PaymentRepository) GetPopulated(ctx context.Context, filter PaymentRepositoryFilter, tx *sqlx.Tx) (*PopulatedPayment, error) {
	query, args, err := p.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var flat populatedPaymentFlat
	if tx != nil {
		err = tx.GetContext(ctx, &flat, query, args...)
	} else {
		err = p.db.GetContext(ctx, &flat, query, args...)
	}
	if err != nil {
		return nil, err
	}
	return p.mapFlatToPopulated(&flat), nil
}

// ListPopulated lists payments with their members, newest first.
func (p *PaymentRepository) ListPopulated(ctx context.Context, filter PaymentRepositoryFilter, opts QueryOptions) (*ListResult[PopulatedPayment], error) {
	query, args, err := p.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var flatList []populatedPaymentFlat
	if err := p.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}

	populatedList := lo.Map(flatList, func(flat populatedPaymentFlat, _ int) *PopulatedPayment {
		return p.mapFlatToPopulated(&flat)
	})

	listResult := ListResult[PopulatedPayment]{
		Items: lo.Slice(populatedList, 0, min(len(populatedList), int(opts.Limit))),
	}
	if len(populatedList) > int(opts.Limit) {
		if lastItem := lo.LastOr(populatedList, nil); lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt, lastItem.ID)
			listResult.NextCursor = &nextCursor
		}
	}
	return &listResult, nil
}

func (p *PaymentRepository) mapFlatToPopulated(flat *populatedPaymentFlat) *PopulatedPayment {
	return &PopulatedPayment{
		Payment: flat.Payment,
		Member: Member{
			ID:             flat.MbID,
			UserID:         flat.MbUserID,
			FirstName:      flat.MbFirstName,
			LastName:       flat.MbLastName,
			Slug:           flat.MbSlug,
			Phone:          ToNullString(flat.MbPhone),
			Address:        ToNullString(flat.MbAddress),
			NextOfKinName:  ToNullString(flat.MbNextOfKinName),
			NextOfKinPhone: ToNullString(flat.MbNextOfKinPhone),
			ActivatedAt:    ToNullTime(flat.MbActivatedAt),
			CreatedAt:      lo.FromPtrOr(flat.MbCreatedAt, time.Time{}),
			UpdatedAt:      ToNullTime(flat.MbUpdatedAt),
			DeletedAt:      ToNullTime(flat.MbDeletedAt),
		},
	}
}

func (p *PaymentRepository) Exists(ctx context.Context, filter PaymentRepositoryFilter, tx *sqlx.Tx) (bool, error) {
	query, args, err := p.buildQuery(filter, QueryOptions{Type: lo.ToPtr(QueryTypeCount)})
	if err != nil {
		return false, err
	}

	var count int
	if tx != nil {
		err = tx.GetContext(ctx, &count, query, args...)
	} else {
		err = p.db.GetContext(ctx, &count, query, args...)
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *PaymentRepository) GetTotal(ctx context.Context, filter PaymentRepositoryFilter, tx *sqlx.Tx) (int64, error) {
	builder := p.psql.Select("COALESCE(SUM(p.amount), 0)").From("payments p")
	query, args, err := p.applyFilter(builder, filter).ToSql()
	if err != nil {
		return 0, err
	}
	var total int64
	if tx != nil {
		err = tx.GetContext(ctx, &total, query, args...)
	} else {
		err = p.db.GetContext(ctx, &total, query, args...)
	}
	return total, err
}

func (p *PaymentRepository) Create(ctx context.Context, payment *Payment, tx *sqlx.Tx) (*Payment, error) {
	builder := p.psql.Insert("payments").
		Columns(
			"member_id", "purpose", "reference", "currency", "amount", "maintenance_fee",
			"maintenance_fee_percent", "processing_charge", "total", "description",
			"fine_id", "share_unit_price", "share_units", "settlement_subaccount", "settlement_share", "campaign_id",
		).
		Values(
			payment.MemberID, payment.Purpose, payment.Reference, payment.Currency, payment.Amount,
			payment.MaintenanceFee, payment.MaintenanceFeePercent, payment.ProcessingCharge,
			payment.Total, payment.Description, payment.FineID, payment.ShareUnitPrice, payment.ShareUnits,
			payment.SettlementSubaccount, payment.SettlementShare, payment.CampaignID,
		).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var created Payment
	if tx != nil {
		err = tx.GetContext(ctx, &created, query, args...)
		return &created, err
	}
	err = p.db.GetContext(ctx, &created, query, args...)
	return &created, err
}

// Update saves the mutable provider and outcome fields. Amounts, purpose and
// references are fixed at checkout and never change.
func (p *PaymentRepository) Update(ctx context.Context, payment *Payment, tx *sqlx.Tx) (*Payment, error) {
	builder := p.psql.Update("payments").
		Set("status", payment.Status).
		Set("authorization_url", payment.AuthorizationUrl).
		Set("access_code", payment.AccessCode).
		Set("transaction_id", payment.TransactionID).
		Set("provider_transaction_id", payment.ProviderTransactionID).
		Set("provider_fees", payment.ProviderFees).
		Set("channel", payment.Channel).
		Set("failure_reason", payment.FailureReason).
		Set("paid_at", payment.PaidAt).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"id": payment.ID}).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updated Payment
	if tx != nil {
		err = tx.GetContext(ctx, &updated, query, args...)
		return &updated, err
	}
	err = p.db.GetContext(ctx, &updated, query, args...)
	return &updated, err
}

func (p *PaymentRepository) MapRepositoryToDTOModel(payment *Payment) *dto.Payment {
	if payment == nil {
		return nil
	}

	result := &dto.Payment{
		ID:                    payment.ID,
		Reference:             payment.Reference,
		Purpose:               dto.LedgerType(payment.Purpose),
		Status:                dto.PaymentStatus(payment.Status),
		Currency:              payment.Currency,
		Amount:                payment.Amount,
		MaintenanceFee:        payment.MaintenanceFee,
		MaintenanceFeePercent: payment.MaintenanceFeePercent,
		ProcessingCharge:      payment.ProcessingCharge,
		Total:                 payment.Total,
		Description:           payment.Description,
		CreatedAt:             payment.CreatedAt,
	}
	if payment.FineID.Valid {
		result.FineID = &payment.FineID.UUID
	}
	if payment.CampaignID.Valid {
		result.CampaignID = &payment.CampaignID.UUID
	}
	if payment.Status == PaymentStatusPENDING && payment.AuthorizationUrl.Valid {
		result.AuthorizationURL = &payment.AuthorizationUrl.String
	}
	if payment.PaidAt.Valid {
		result.PaidAt = &payment.PaidAt.Time
	}
	if payment.FailureReason.Valid {
		result.FailureReason = &payment.FailureReason.String
	}
	return result
}

// MapPopulatedToDTOModel is MapRepositoryToDTOModel plus the member.
func (p *PaymentRepository) MapPopulatedToDTOModel(populated *PopulatedPayment) *dto.Payment {
	if populated == nil {
		return nil
	}
	result := p.MapRepositoryToDTOModel(&populated.Payment)
	result.Member = p.memberRepository.MapRepositoryToDTOModel(&populated.Member)
	return result
}
