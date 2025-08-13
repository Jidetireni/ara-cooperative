package user

import (
	"context"

	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type UserQuerier struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewUserQuerier(db *sqlx.DB) *UserQuerier {
	return &UserQuerier{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type UserQuerierFilter struct {
	ID    *uuid.UUID
	Email *string
}

func (uq *UserQuerier) buildQuery(filter UserQuerierFilter, queryType repository.QueryType) (string, []any, error) {
	var builder sq.SelectBuilder
	switch queryType {
	case repository.QueryTypeSelect:
		builder = uq.psql.Select("*").From("users")
	case repository.QueryTypeCount:
		builder = uq.psql.Select("COUNT(*)").From("users")
	}

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"id": *filter.ID})
	}
	if filter.Email != nil {
		builder = builder.Where(sq.Eq{"email": *filter.Email})
	}

	return builder.ToSql()
}

func (uq *UserQuerier) Get(ctx context.Context, filter UserQuerierFilter) (*repository.User, error) {
	query, args, err := uq.buildQuery(filter, repository.QueryTypeSelect)
	if err != nil {
		return nil, err
	}
	var user repository.User
	if err := uq.db.GetContext(ctx, &user, query, args...); err != nil {
		return nil, err
	}
	return &user, nil
}

func (uq *UserQuerier) Create(ctx context.Context, user *repository.User) (*repository.User, error) {
	builder := uq.psql.Insert("users").
		Columns("email", "hash_password").
		Values(user.Email, user.PasswordHash).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdUser repository.User
	if err := uq.db.GetContext(ctx, &createdUser, query, args...); err != nil {
		return nil, err
	}

	return &createdUser, nil
}
