package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	sqlx "github.com/jmoiron/sqlx"
)

type RoleRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewRoleRepository(db *sqlx.DB) *RoleRepository {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	return &RoleRepository{
		db:   db,
		psql: psql,
	}
}

type RoleRepositoryFilter struct {
	ID     *uuid.UUID
	Name   []string
	UserID *uuid.UUID
}

func (rr *RoleRepository) generateQuery(filter *RoleRepositoryFilter, queryType QueryType) (string, []interface{}, error) {
	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = rr.psql.Select("r.*").From("roles r")
	case QueryTypeCount:
		builder = rr.psql.Select("COUNT(r.*)").From("roles r")
	}

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"r.id": filter.ID})
	}

	if len(filter.Name) > 0 {
		builder = builder.Where(sq.Eq{"r.name": filter.Name})
	}

	if filter.UserID != nil {
		builder = builder.Join("user_roles ur ON r.id = ur.role_id").
			Where(sq.Eq{"ur.user_id": filter.UserID})
	}

	return builder.ToSql()
}

func (r *RoleRepository) Get(ctx context.Context, filter *RoleRepositoryFilter) (*Role, error) {
	query, args, err := r.generateQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}

	var role Role
	err = r.db.GetContext(ctx, &role, query, args...)
	if err != nil {
		return nil, err
	}

	return &role, nil
}

func (r *RoleRepository) AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error {
	// Use an UPSERT to avoid duplicates without a separate check.
	builder := r.psql.Insert("user_roles").
		Columns("user_id", "role_id").
		Suffix("ON CONFLICT (user_id, role_id) DO NOTHING")

	for _, roleID := range roleIDs {
		builder = builder.Values(userID, roleID)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *RoleRepository) List(ctx context.Context, filter *RoleRepositoryFilter) ([]Role, error) {
	query, args, err := r.generateQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}

	var permissions []Role
	err = r.db.SelectContext(ctx, &permissions, query, args...)

	return permissions, err
}

func (r *RoleRepository) RevokeFromUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error {
	builder := r.psql.Delete("user_roles").
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"role_id": roleIDs})

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}
