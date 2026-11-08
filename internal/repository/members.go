package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type MemberRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewMemberRepository(db *sqlx.DB) *MemberRepository {
	return &MemberRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// NextNumber preserves the ara000001 slug format across concurrent requests and restarts.
func (mq *MemberRepository) NextNumber(ctx context.Context, tx *sqlx.Tx) (int64, error) {
	query, args, err := mq.psql.Select("nextval('members_slug_seq')").ToSql()
	if err != nil {
		return 0, err
	}
	var number int64
	if err := tx.GetContext(ctx, &number, query, args...); err != nil {
		return 0, err
	}
	return number, nil
}

type MemberRepositoryFilter struct {
	ID       *uuid.UUID
	UserID   *uuid.UUID
	Slug     *string
	Phone    *string
	IsActive *bool
	// Search matches name, member number, or account email (case-insensitive).
	Search    *string
	ForUpdate bool
}

// PopulatedMember carries the member's account so admins can see the email
// and whether the invitation has been accepted.
type PopulatedMember struct {
	Member
	User *User
}

type populatedMemberFlat struct {
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
	MbCreatedAt      time.Time  `json:"mb_created_at"`
	MbUpdatedAt      *time.Time `json:"mb_updated_at"`
	MbDeletedAt      *time.Time `json:"mb_deleted_at"`

	UsID               uuid.UUID  `json:"us_id"`
	UsEmail            string     `json:"us_email"`
	UsEmailConfirmedAt *time.Time `json:"us_email_confirmed_at"`
	UsCreatedAt        time.Time  `json:"us_created_at"`
}

func (mq *MemberRepository) applyFilter(builder sq.SelectBuilder, filter MemberRepositoryFilter) sq.SelectBuilder {
	// Only get non-deleted members
	builder = builder.Where("m.deleted_at IS NULL")

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"m.id": *filter.ID})
	}
	if filter.UserID != nil {
		builder = builder.Where(sq.Eq{"m.user_id": *filter.UserID})
	}
	if filter.Slug != nil {
		builder = builder.Where(sq.Eq{"m.slug": *filter.Slug})
	}
	if filter.Phone != nil {
		builder = builder.Where(sq.Eq{"m.phone": *filter.Phone})
	}
	if filter.IsActive != nil {
		if *filter.IsActive {
			builder = builder.Where("m.activated_at IS NOT NULL")
		} else {
			builder = builder.Where("m.activated_at IS NULL")
		}
	}
	if filter.Search != nil {
		if term := strings.TrimSpace(*filter.Search); term != "" {
			pattern := "%" + likeEscaper.Replace(term) + "%"
			// The email lives on users. A correlated EXISTS keeps this filter usable
			// from the plain members query as well as the populated (joined) one.
			builder = builder.Where(sq.Or{
				sq.ILike{"m.first_name": pattern},
				sq.ILike{"m.last_name": pattern},
				sq.ILike{"m.slug": pattern},
				sq.Expr("(m.first_name || ' ' || m.last_name) ILIKE ?", pattern),
				sq.Expr("EXISTS (SELECT 1 FROM users su WHERE su.id = m.user_id AND su.email ILIKE ?)", pattern),
			})
		}
	}
	return builder
}

// likeEscaper makes user input match literally inside an ILIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func (mq *MemberRepository) buildQuery(filter MemberRepositoryFilter, opts QueryOptions) (string, []any, error) {
	var queryType QueryType = QueryTypeSelect
	var err error
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = mq.psql.Select("m.*").From("members m")
	case QueryTypeCount:
		builder = mq.psql.Select("COUNT(*)").From("members m")
	}

	builder = mq.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		opts.Sort, err = SortFor(opts.Sort, "m")
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

func (mq *MemberRepository) populatedSelectColumns() []string {
	return []string{
		// Member fields
		"m.id AS mb_id",
		"m.user_id AS mb_user_id",
		"m.first_name AS mb_first_name",
		"m.last_name AS mb_last_name",
		"m.slug AS mb_slug",
		"m.phone AS mb_phone",
		"m.address AS mb_address",
		"m.next_of_kin_name AS mb_next_of_kin_name",
		"m.next_of_kin_phone AS mb_next_of_kin_phone",
		"m.activated_at AS mb_activated_at",
		"m.created_at AS mb_created_at",
		"m.updated_at AS mb_updated_at",
		"m.deleted_at AS mb_deleted_at",

		// User fields (no password hash)
		"u.id AS us_id",
		"u.email AS us_email",
		"u.email_confirmed_at AS us_email_confirmed_at",
		"u.created_at AS us_created_at",
	}
}

func (mq *MemberRepository) buildPopulatedQuery(filter MemberRepositoryFilter, opts QueryOptions) (string, []any, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	var err error

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = mq.psql.Select(mq.populatedSelectColumns()...)
	case QueryTypeCount:
		builder = mq.psql.Select("COUNT(*)")
	}

	builder = builder.From("members m").
		Join("users u ON u.id = m.user_id AND u.deleted_at IS NULL")

	builder = mq.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		opts.Sort, err = SortFor(opts.Sort, "m")
		if err != nil {
			return "", nil, err
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (mq *MemberRepository) GetPopulated(ctx context.Context, filter MemberRepositoryFilter, tx *sqlx.Tx) (*PopulatedMember, error) {
	query, args, err := mq.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var flat populatedMemberFlat
	if tx != nil {
		if err := tx.GetContext(ctx, &flat, query, args...); err != nil {
			return nil, err
		}
		return mq.mapFlatToPopulated(&flat), nil
	}

	if err := mq.db.GetContext(ctx, &flat, query, args...); err != nil {
		return nil, err
	}
	return mq.mapFlatToPopulated(&flat), nil
}

func (mq *MemberRepository) ListPopulated(ctx context.Context, filter MemberRepositoryFilter, opts QueryOptions) (*ListResult[PopulatedMember], error) {
	query, args, err := mq.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var flatList []populatedMemberFlat
	if err := mq.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}

	populatedList := lo.Map(flatList, func(flat populatedMemberFlat, _ int) *PopulatedMember {
		return mq.mapFlatToPopulated(&flat)
	})

	listResult := ListResult[PopulatedMember]{
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

func (mq *MemberRepository) mapFlatToPopulated(flat *populatedMemberFlat) *PopulatedMember {
	return &PopulatedMember{
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
			CreatedAt:      flat.MbCreatedAt,
			UpdatedAt:      ToNullTime(flat.MbUpdatedAt),
			DeletedAt:      ToNullTime(flat.MbDeletedAt),
		},
		User: &User{
			ID:               flat.UsID,
			Email:            flat.UsEmail,
			EmailConfirmedAt: ToNullTime(flat.UsEmailConfirmedAt),
			CreatedAt:        flat.UsCreatedAt,
		},
	}
}

func (mq *MemberRepository) Get(ctx context.Context, filter MemberRepositoryFilter, tx *sqlx.Tx) (*Member, error) {
	query, args, err := mq.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var member Member
	if tx != nil {
		err = tx.GetContext(ctx, &member, query, args...)
		return &member, err
	}
	if filter.ForUpdate {
		return nil, errors.New("row locking requires a transaction")
	}
	if err := mq.db.GetContext(ctx, &member, query, args...); err != nil {
		return nil, err
	}
	return &member, nil
}

func (mq *MemberRepository) Exists(ctx context.Context, filter MemberRepositoryFilter) (bool, error) {
	query, args, err := mq.buildQuery(filter, QueryOptions{
		Type: lo.ToPtr(QueryTypeCount),
	})
	if err != nil {
		return false, err
	}

	var count int
	if err := mq.db.GetContext(ctx, &count, query, args...); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (mq *MemberRepository) Create(ctx context.Context, member *Member, tx *sqlx.Tx) (*Member, error) {
	builder := mq.psql.Insert("members").
		Columns("user_id", "slug", "first_name", "last_name", "phone", "address", "next_of_kin_name", "next_of_kin_phone").
		Values(member.UserID, member.Slug, member.FirstName, member.LastName, member.Phone, member.Address, member.NextOfKinName, member.NextOfKinPhone).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdMember Member
	if tx != nil {
		err = tx.GetContext(ctx, &createdMember, query, args...)
		return &createdMember, err
	}

	err = mq.db.GetContext(ctx, &createdMember, query, args...)
	return &createdMember, err
}

func (mq *MemberRepository) Update(ctx context.Context, member *Member, tx *sqlx.Tx) (*Member, error) {
	builder := mq.psql.Update("members").
		Set("first_name", member.FirstName).
		Set("last_name", member.LastName).
		Set("phone", member.Phone).
		Set("address", member.Address).
		Set("next_of_kin_name", member.NextOfKinName).
		Set("next_of_kin_phone", member.NextOfKinPhone).
		Set("activated_at", member.ActivatedAt).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"id": member.ID}).
		Where(sq.Eq{"deleted_at": nil}).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updatedMember Member
	if tx != nil {
		err = tx.GetContext(ctx, &updatedMember, query, args...)
		return &updatedMember, err
	}

	err = mq.db.GetContext(ctx, &updatedMember, query, args...)
	return &updatedMember, err
}

func (mq *MemberRepository) List(ctx context.Context, filter MemberRepositoryFilter, opts QueryOptions) (*ListResult[Member], error) {
	query, args, err := mq.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var members []*Member
	err = mq.db.SelectContext(ctx, &members, query, args...)
	if err != nil {
		return nil, err
	}

	listResult := ListResult[Member]{
		Items: lo.Slice(members, 0, min(len(members), int(opts.Limit))),
	}

	if len(members) > int(opts.Limit) {
		lastItem := lo.LastOr(members, nil)
		if lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt, lastItem.ID)
			listResult.NextCursor = &nextCursor
		}
	}

	return &listResult, nil
}

func (mq *MemberRepository) MapRepositoryToDTOModel(member *Member) *dto.Member {
	if member == nil {
		return nil
	}

	isActive := false
	if member.ActivatedAt.Valid {
		isActive = true
	}
	return &dto.Member{
		ID:             member.ID,
		FirstName:      member.FirstName,
		LastName:       member.LastName,
		Phone:          member.Phone.String,
		Slug:           member.Slug,
		Address:        member.Address.String,
		NextOfKinName:  member.NextOfKinName.String,
		NextOfKinPhone: member.NextOfKinPhone.String,
		IsActive:       isActive,
	}
}

func (mq *MemberRepository) MapPopulatedToDTOModel(populated *PopulatedMember) *dto.MemberSummary {
	if populated == nil {
		return nil
	}

	summary := &dto.MemberSummary{
		Member:    *mq.MapRepositoryToDTOModel(&populated.Member),
		CreatedAt: populated.CreatedAt,
	}
	if populated.User != nil {
		summary.Email = populated.User.Email
		summary.EmailConfirmed = populated.User.EmailConfirmedAt.Valid
	}
	return summary
}
