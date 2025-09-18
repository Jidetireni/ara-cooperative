package repository

import (
	"context"

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

type MemberRepositoryFilter struct {
	ID     *uuid.UUID
	UserID *uuid.UUID
	Slug   *string
	Phone  *string
}

func (mq *MemberRepository) buildQuery(filter MemberRepositoryFilter, opts QueryOptions) (string, []any, error) {
	var queryType QueryType = QueryTypeSelect
	var err error
	if opts.Type != nil {
		queryType = *opts.Type
	}

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = mq.psql.Select("*").From("members")
	case QueryTypeCount:
		builder = mq.psql.Select("COUNT(*)").From("members")
	}

	// Only get non-deleted members
	builder = builder.Where("deleted_at IS NULL")

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"id": *filter.ID})
	}
	if filter.UserID != nil {
		builder = builder.Where(sq.Eq{"user_id": *filter.UserID})
	}

	if filter.Slug != nil {
		builder = builder.Where(sq.Eq{"slug": *filter.Slug})
	}

	if filter.Phone != nil {
		builder = builder.Where(sq.Eq{"phone": *filter.Phone})
	}

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("created_at:desc")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (mq *MemberRepository) Get(ctx context.Context, filter MemberRepositoryFilter) (*Member, error) {
	query, args, err := mq.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var member Member
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
