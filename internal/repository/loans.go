package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type LoanRepository struct {
	db               *sqlx.DB
	psql             sq.StatementBuilderType
	memberRepository *MemberRepository
}

func NewLoanRepository(db *sqlx.DB) *LoanRepository {
	return &LoanRepository{db: db, psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar), memberRepository: NewMemberRepository(db)}
}

type LoanRepositoryFilter struct {
	ID        *uuid.UUID
	MemberID  *uuid.UUID
	ExcludeID *uuid.UUID
	Status    *string
	Search    *string
	ForUpdate bool
}

type PopulatedLoan struct {
	Loan
	Member Member
}

func (l *LoanRepository) applyFilter(builder sq.SelectBuilder, filter LoanRepositoryFilter) sq.SelectBuilder {
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"l.id": *filter.ID})
	}
	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"l.member_id": *filter.MemberID})
	}
	if filter.ExcludeID != nil {
		builder = builder.Where(sq.NotEq{"l.id": *filter.ExcludeID})
	}
	if filter.Status != nil {
		builder = builder.Where(sq.Eq{"l.status": *filter.Status})
	}
	if filter.Search != nil {
		pattern := "%" + likeEscaper.Replace(strings.TrimSpace(*filter.Search)) + "%"
		builder = builder.Where(sq.Or{sq.ILike{"mb.first_name": pattern}, sq.ILike{"mb.last_name": pattern},
			sq.ILike{"mb.slug": pattern}, sq.ILike{"l.purpose": pattern}})
	}
	return builder
}

func (l *LoanRepository) buildQuery(filter LoanRepositoryFilter, opts QueryOptions) (string, []any, error) {
	return l.generateQuery(filter, opts, false)
}

func (l *LoanRepository) buildPopulatedQuery(filter LoanRepositoryFilter, opts QueryOptions) (string, []any, error) {
	return l.generateQuery(filter, opts, true)
}

func (l *LoanRepository) generateQuery(
	filter LoanRepositoryFilter, opts QueryOptions, populated bool,
) (string, []any, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	builder := l.psql.Select("l.*").From("loans l").Join("members mb ON mb.id = l.member_id")
	if queryType == QueryTypeCount {
		builder = builder.RemoveColumns().Column("COUNT(*)")
	} else if populated {
		builder = builder.RemoveColumns().Columns(l.populatedSelectColumns()...)
	}
	builder = l.applyFilter(builder, filter)
	if queryType != QueryTypeCount {
		var err error
		opts.Sort, err = SortFor(opts.Sort, "l")
		if err != nil {
			return "", nil, err
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
		if filter.ForUpdate {
			builder = builder.Suffix("FOR UPDATE OF l")
		}
	}
	return builder.ToSql()
}

func (l *LoanRepository) Get(ctx context.Context, filter LoanRepositoryFilter, tx *sqlx.Tx) (*Loan, error) {
	query, args, err := l.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}
	var loan Loan
	if tx != nil {
		err = tx.GetContext(ctx, &loan, query, args...)
	} else {
		if filter.ForUpdate {
			return nil, errors.New("row locking requires a transaction")
		}
		err = l.db.GetContext(ctx, &loan, query, args...)
	}
	return &loan, err
}

func (l *LoanRepository) GetPopulated(ctx context.Context, filter LoanRepositoryFilter, tx *sqlx.Tx) (*PopulatedLoan, error) {
	query, args, err := l.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}
	var flat populatedLoanFlat
	if tx != nil {
		err = tx.GetContext(ctx, &flat, query, args...)
	} else {
		if filter.ForUpdate {
			return nil, errors.New("row locking requires a transaction")
		}
		err = l.db.GetContext(ctx, &flat, query, args...)
	}
	if err != nil {
		return nil, err
	}
	return l.mapFlatToPopulated(&flat), nil
}

func (l *LoanRepository) ListPopulated(
	ctx context.Context, filter LoanRepositoryFilter, opts QueryOptions,
) (*ListResult[PopulatedLoan], error) {
	query, args, err := l.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}
	flatList := []populatedLoanFlat{}
	if err := l.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}
	items := lo.Map(flatList, func(flat populatedLoanFlat, _ int) *PopulatedLoan { return l.mapFlatToPopulated(&flat) })
	result := &ListResult[PopulatedLoan]{Items: lo.Slice(items, 0, min(len(items), int(opts.Limit)))}
	if len(items) > int(opts.Limit) {
		last := items[len(items)-1]
		result.NextCursor = lo.ToPtr(EncodeCursor(last.CreatedAt, last.ID))
	}
	return result, nil
}

func (l *LoanRepository) Count(ctx context.Context, filter LoanRepositoryFilter, tx *sqlx.Tx) (int64, error) {
	query, args, err := l.buildQuery(filter, QueryOptions{Type: lo.ToPtr(QueryTypeCount)})
	if err != nil {
		return 0, err
	}
	var count int64
	if tx != nil {
		err = tx.GetContext(ctx, &count, query, args...)
	} else {
		err = l.db.GetContext(ctx, &count, query, args...)
	}
	return count, err
}

func (l *LoanRepository) Create(ctx context.Context, loan *Loan, tx *sqlx.Tx) (*Loan, error) {
	query, args, err := l.psql.Insert("loans").
		Columns("member_id", "requested_amount", "purpose", "requested_duration_months").
		Values(loan.MemberID, loan.RequestedAmount, loan.Purpose, loan.RequestedDurationMonths).Suffix("RETURNING *").ToSql()
	if err != nil {
		return nil, err
	}
	var created Loan
	if tx != nil {
		err = tx.GetContext(ctx, &created, query, args...)
	} else {
		err = l.db.GetContext(ctx, &created, query, args...)
	}
	return &created, err
}

func (l *LoanRepository) Update(ctx context.Context, loan *Loan, tx *sqlx.Tx) (*Loan, error) {
	query, args, err := l.psql.Update("loans").Set("status", loan.Status).
		Set("approved_amount", loan.ApprovedAmount).Set("approved_duration_months", loan.ApprovedDurationMonths).
		Set("interest_method", loan.InterestMethod).Set("interest_rate_percent", loan.InterestRatePercent).
		Set("interest_period", loan.InterestPeriod).Set("review_reason", loan.ReviewReason).
		Set("override_reason", loan.OverrideReason).Set("eligibility_snapshot", loan.EligibilitySnapshot).
		Set("reviewed_by", loan.ReviewedBy).Set("reviewed_at", loan.ReviewedAt).
		Set("disbursed_amount", loan.DisbursedAmount).Set("disbursed_at", loan.DisbursedAt).
		Set("disbursement_reference", loan.DisbursementReference).Set("disbursed_by", loan.DisbursedBy).
		Set("transaction_id", loan.TransactionID).Set("updated_at", time.Now()).
		Where(sq.Eq{"id": loan.ID}).Suffix("RETURNING *").ToSql()
	if err != nil {
		return nil, err
	}
	var updated Loan
	if tx != nil {
		err = tx.GetContext(ctx, &updated, query, args...)
	} else {
		err = l.db.GetContext(ctx, &updated, query, args...)
	}
	return &updated, err
}

func (l *LoanRepository) GetSettings(ctx context.Context, tx *sqlx.Tx) (*LoanSetting, error) {
	query, args, err := l.psql.Select("*").From("loan_settings").Where(sq.Eq{"id": 1}).ToSql()
	if err != nil {
		return nil, err
	}
	var settings LoanSetting
	if tx != nil {
		err = tx.GetContext(ctx, &settings, query, args...)
	} else {
		err = l.db.GetContext(ctx, &settings, query, args...)
	}
	return &settings, err
}

func (l *LoanRepository) UpdateSettings(ctx context.Context, settings *LoanSetting, tx *sqlx.Tx) (*LoanSetting, error) {
	query, args, err := l.psql.Update("loan_settings").Set("minimum_savings", settings.MinimumSavings).
		Set("minimum_share_units", settings.MinimumShareUnits).Set("updated_by", settings.UpdatedBy).
		Set("updated_at", time.Now()).Where(sq.Eq{"id": 1}).Suffix("RETURNING *").ToSql()
	if err != nil {
		return nil, err
	}
	var updated LoanSetting
	if tx != nil {
		err = tx.GetContext(ctx, &updated, query, args...)
	} else {
		err = l.db.GetContext(ctx, &updated, query, args...)
	}
	return &updated, err
}

func (l *LoanRepository) MapRepositoryToDTOModel(loan *PopulatedLoan) *dto.Loan {
	if loan == nil {
		return nil
	}
	result := &dto.Loan{ID: loan.ID, MemberID: loan.MemberID, Member: l.memberRepository.MapRepositoryToDTOModel(&loan.Member),
		RequestedAmount: loan.RequestedAmount, Purpose: loan.Purpose, RequestedDurationMonths: loan.RequestedDurationMonths,
		Status: loan.Status, CreatedAt: loan.CreatedAt}
	if loan.ApprovedAmount.Valid {
		result.ApprovedAmount = &loan.ApprovedAmount.Int64
		result.ApprovedDurationMonths = &loan.ApprovedDurationMonths.Int32
		result.InterestMethod = &loan.InterestMethod.String
		result.InterestRatePercent = &loan.InterestRatePercent.String
		result.InterestPeriod = &loan.InterestPeriod.String
	}
	if loan.ReviewedAt.Valid {
		result.ReviewedAt, result.ReviewedBy = &loan.ReviewedAt.Time, &loan.ReviewedBy.UUID
	}
	if loan.ReviewReason.Valid {
		result.ReviewReason = &loan.ReviewReason.String
	}
	if loan.OverrideReason.Valid {
		result.OverrideReason = &loan.OverrideReason.String
	}
	if loan.EligibilitySnapshot.Valid {
		var snapshot dto.LoanEligibility
		if err := json.Unmarshal([]byte(loan.EligibilitySnapshot.String), &snapshot); err == nil {
			result.EligibilitySnapshot = &snapshot
		}
	}
	if loan.DisbursedAt.Valid {
		result.DisbursedAmount, result.DisbursedAt = &loan.DisbursedAmount.Int64, &loan.DisbursedAt.Time
		result.DisbursementReference, result.DisbursedBy = &loan.DisbursementReference.String, &loan.DisbursedBy.UUID
		result.TransactionID = &loan.TransactionID.UUID
	}
	if loan.UpdatedAt.Valid {
		result.UpdatedAt = &loan.UpdatedAt.Time
	}
	return result
}

type populatedLoanFlat struct {
	Loan
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

func (l *LoanRepository) populatedSelectColumns() []string {
	return []string{
		"l.*",
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

func (l *LoanRepository) mapFlatToPopulated(flat *populatedLoanFlat) *PopulatedLoan {
	return &PopulatedLoan{
		Loan: flat.Loan,
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
