package repository

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type LoanGuarantorRepositoryFilter struct {
	LoanIDs   []uuid.UUID
	LoanID    *uuid.UUID
	MemberID  *uuid.UUID
	ForUpdate bool
}

// PopulatedLoanGuarantor carries the guarantor's member record and the loan it
// guarantees, including the applicant, for guarantor-request views.
type PopulatedLoanGuarantor struct {
	LoanGuarantor
	Member    Member
	Loan      Loan
	Applicant Member
}

type populatedLoanGuarantorFlat struct {
	LoanGuarantor
	GmSlug       string     `json:"gm_slug"`
	GmUserID     uuid.UUID  `json:"gm_user_id"`
	GmFirstName  string     `json:"gm_first_name"`
	GmMiddleName *string    `json:"gm_middle_name"`
	GmLastName   string     `json:"gm_last_name"`
	GmAddress    *string    `json:"gm_address"`
	GmActivated  *time.Time `json:"gm_activated_at"`
	GmPhotoID    *string    `json:"gm_passport_photo_public_id"`
	GmPhotoVer   *int64     `json:"gm_passport_photo_version"`

	LnMemberID        uuid.UUID `json:"ln_member_id"`
	LnRequestedAmount int64     `json:"ln_requested_amount"`
	LnPurpose         string    `json:"ln_purpose"`
	LnDurationMonths  int32     `json:"ln_requested_duration_months"`
	LnStatus          string    `json:"ln_status"`
	LnCreatedAt       time.Time `json:"ln_created_at"`

	ApSlug       string     `json:"ap_slug"`
	ApFirstName  string     `json:"ap_first_name"`
	ApMiddleName *string    `json:"ap_middle_name"`
	ApLastName   string     `json:"ap_last_name"`
	ApActivated  *time.Time `json:"ap_activated_at"`
	ApPhotoID    *string    `json:"ap_passport_photo_public_id"`
	ApPhotoVer   *int64     `json:"ap_passport_photo_version"`
}

func (l *LoanRepository) guarantorQuery(filter LoanGuarantorRepositoryFilter, opts QueryOptions) (string, []any, error) {
	builder := l.psql.Select(
		"lg.*",
		"gm.slug AS gm_slug", "gm.user_id AS gm_user_id", "gm.first_name AS gm_first_name",
		"gm.middle_name AS gm_middle_name", "gm.last_name AS gm_last_name", "gm.address AS gm_address",
		"gm.activated_at AS gm_activated_at",
		"gm.passport_photo_public_id AS gm_passport_photo_public_id", "gm.passport_photo_version AS gm_passport_photo_version",
		"ln.member_id AS ln_member_id", "ln.requested_amount AS ln_requested_amount", "ln.purpose AS ln_purpose",
		"ln.requested_duration_months AS ln_requested_duration_months", "ln.status AS ln_status",
		"ln.created_at AS ln_created_at",
		"ap.slug AS ap_slug", "ap.first_name AS ap_first_name", "ap.middle_name AS ap_middle_name",
		"ap.last_name AS ap_last_name", "ap.activated_at AS ap_activated_at",
		"ap.passport_photo_public_id AS ap_passport_photo_public_id", "ap.passport_photo_version AS ap_passport_photo_version",
	).From("loan_guarantors lg").
		Join("members gm ON gm.id = lg.member_id").
		Join("loans ln ON ln.id = lg.loan_id").
		Join("members ap ON ap.id = ln.member_id")
	if len(filter.LoanIDs) > 0 {
		builder = builder.Where(sq.Eq{"lg.loan_id": filter.LoanIDs})
	}
	if filter.LoanID != nil {
		builder = builder.Where(sq.Eq{"lg.loan_id": *filter.LoanID})
	}
	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"lg.member_id": *filter.MemberID})
	}
	if opts.Limit > 0 {
		var err error
		opts.Sort, err = SortFor(opts.Sort, "lg")
		if err != nil {
			return "", nil, err
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	} else {
		builder = builder.OrderBy("lg.loan_id", "lg.position")
	}
	if filter.ForUpdate {
		builder = builder.Suffix("FOR UPDATE OF lg")
	}
	return builder.ToSql()
}

func (l *LoanRepository) mapGuarantorFlat(flat *populatedLoanGuarantorFlat) *PopulatedLoanGuarantor {
	return &PopulatedLoanGuarantor{
		LoanGuarantor: flat.LoanGuarantor,
		Member: Member{
			ID: flat.MemberID, UserID: flat.GmUserID, Slug: flat.GmSlug, FirstName: flat.GmFirstName,
			MiddleName: ToNullString(flat.GmMiddleName), LastName: flat.GmLastName,
			Address: ToNullString(flat.GmAddress), ActivatedAt: ToNullTime(flat.GmActivated),
			PassportPhotoPublicID: ToNullString(flat.GmPhotoID), PassportPhotoVersion: ToNullInt64(flat.GmPhotoVer),
		},
		Loan: Loan{
			ID: flat.LoanID, MemberID: flat.LnMemberID, RequestedAmount: flat.LnRequestedAmount, Purpose: flat.LnPurpose,
			RequestedDurationMonths: flat.LnDurationMonths, Status: flat.LnStatus, CreatedAt: flat.LnCreatedAt,
		},
		Applicant: Member{
			ID: flat.LnMemberID, Slug: flat.ApSlug, FirstName: flat.ApFirstName,
			MiddleName: ToNullString(flat.ApMiddleName), LastName: flat.ApLastName, ActivatedAt: ToNullTime(flat.ApActivated),
			PassportPhotoPublicID: ToNullString(flat.ApPhotoID), PassportPhotoVersion: ToNullInt64(flat.ApPhotoVer),
		},
	}
}

// ListGuarantors returns guarantors in loan and position order.
func (l *LoanRepository) ListGuarantors(
	ctx context.Context, filter LoanGuarantorRepositoryFilter, tx *sqlx.Tx,
) ([]*PopulatedLoanGuarantor, error) {
	if filter.ForUpdate && tx == nil {
		return nil, errors.New("row locking requires a transaction")
	}
	query, args, err := l.guarantorQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}
	flat := []populatedLoanGuarantorFlat{}
	if tx != nil {
		err = tx.SelectContext(ctx, &flat, query, args...)
	} else {
		err = l.db.SelectContext(ctx, &flat, query, args...)
	}
	if err != nil {
		return nil, err
	}
	return lo.Map(flat, func(item populatedLoanGuarantorFlat, _ int) *PopulatedLoanGuarantor {
		return l.mapGuarantorFlat(&item)
	}), nil
}

// ListGuarantorRequests pages through the loans a member has been asked to guarantee.
func (l *LoanRepository) ListGuarantorRequests(
	ctx context.Context, memberID uuid.UUID, opts QueryOptions,
) (*ListResult[PopulatedLoanGuarantor], error) {
	query, args, err := l.guarantorQuery(LoanGuarantorRepositoryFilter{MemberID: &memberID}, opts)
	if err != nil {
		return nil, err
	}
	flat := []populatedLoanGuarantorFlat{}
	if err := l.db.SelectContext(ctx, &flat, query, args...); err != nil {
		return nil, err
	}
	items := lo.Map(flat, func(item populatedLoanGuarantorFlat, _ int) *PopulatedLoanGuarantor {
		return l.mapGuarantorFlat(&item)
	})
	result := &ListResult[PopulatedLoanGuarantor]{Items: lo.Slice(items, 0, min(len(items), int(opts.Limit)))}
	if len(items) > int(opts.Limit) {
		last := items[len(items)-1]
		result.NextCursor = lo.ToPtr(EncodeCursor(last.CreatedAt, last.ID))
	}
	return result, nil
}

// CreateGuarantors records the applicant's guarantors in form order.
func (l *LoanRepository) CreateGuarantors(ctx context.Context, loanID uuid.UUID, memberIDs []uuid.UUID, tx *sqlx.Tx) error {
	if tx == nil {
		return errors.New("creating guarantors requires a transaction")
	}
	insert := l.psql.Insert("loan_guarantors").Columns("loan_id", "member_id", "position")
	for i, memberID := range memberIDs {
		insert = insert.Values(loanID, memberID, i+1)
	}
	query, args, err := insert.ToSql()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

// UpdateGuarantorStatus records a guarantor's response.
func (l *LoanRepository) UpdateGuarantorStatus(
	ctx context.Context, id uuid.UUID, status string, respondedAt time.Time, tx *sqlx.Tx,
) error {
	query, args, err := l.psql.Update("loan_guarantors").Set("status", status).Set("responded_at", respondedAt).
		Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return err
	}
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
	} else {
		_, err = l.db.ExecContext(ctx, query, args...)
	}
	return err
}
