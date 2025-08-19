package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type UserRepositoryFilter struct {
	ID    *uuid.UUID
	Email *string
}

func (uq *UserRepository) buildQuery(filter UserRepositoryFilter, queryType QueryType) (string, []any, error) {
	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = uq.psql.Select("*").From("users")
	case QueryTypeCount:
		builder = uq.psql.Select("COUNT(*)").From("users")
	}

	// Only get non-deleted users
	builder = builder.Where(sq.Eq{"deleted_at": nil})

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"id": *filter.ID})
	}
	if filter.Email != nil {
		builder = builder.Where(sq.Eq{"email": *filter.Email})
	}

	return builder.ToSql()
}

func (uq *UserRepository) Get(ctx context.Context, filter UserRepositoryFilter) (*User, error) {
	query, args, err := uq.buildQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}
	var user User
	if err := uq.db.GetContext(ctx, &user, query, args...); err != nil {
		return nil, err
	}
	return &user, nil
}

func (uq *UserRepository) Exists(ctx context.Context, filter UserRepositoryFilter) (bool, error) {
	query, args, err := uq.buildQuery(filter, QueryTypeCount)
	if err != nil {
		return false, err
	}

	var count int
	if err := uq.db.GetContext(ctx, &count, query, args...); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (uq *UserRepository) Create(ctx context.Context, user *User, tx *sqlx.Tx) (*User, error) {
	builder := uq.psql.Insert("users").
		Columns("email", "password_hash").
		Values(user.Email, user.PasswordHash).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdUser User
	if tx != nil {
		err = tx.GetContext(ctx, &createdUser, query, args...)
		return &createdUser, err
	}

	err = uq.db.GetContext(ctx, &createdUser, query, args...)
	return &createdUser, err
}

func (uq *UserRepository) Upsert(ctx context.Context, user *User, tx *sqlx.Tx) (*User, error) {
	builder := uq.psql.Insert("users").
		Columns("id", "email", "password_hash").
		Values(user.ID, user.Email, user.PasswordHash).
		Suffix("ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, password_hash = EXCLUDED.password_hash RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var upsertedUser User
	if tx != nil {
		err = tx.GetContext(ctx, &upsertedUser, query, args...)
		return &upsertedUser, err
	}

	err = uq.db.GetContext(ctx, &upsertedUser, query, args...)
	return &upsertedUser, err
}
