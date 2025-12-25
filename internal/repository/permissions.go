package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PermissionRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewPermissionRepository(db *sqlx.DB) *PermissionRepository {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	return &PermissionRepository{
		db:   db,
		psql: psql,
	}
}

type PermissionRepositoryFilter struct {
	ID     *uuid.UUID
	Slug   []string
	UserID *uuid.UUID
}

func (p *PermissionRepository) generateQuery(filter *PermissionRepositoryFilter, queryType QueryType) (string, []interface{}, error) {
	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = p.psql.Select("p.*").From("permissions p")
	case QueryTypeCount:
		builder = p.psql.Select("COUNT(p.*)").From("permissions p")
	}

	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"p.id": filter.ID})
	}

	if len(filter.Slug) > 0 {
		builder = builder.Where(sq.Eq{"p.slug": filter.Slug})
	}

	if filter.UserID != nil {
		builder = builder.Join("user_permissions up ON p.id = up.permission_id").
			Where(sq.Eq{"up.user_id": filter.UserID})
	}

	return builder.ToSql()
}

func (p *PermissionRepository) Get(ctx context.Context, filter *PermissionRepositoryFilter) (*Permission, error) {
	query, args, err := p.generateQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}

	var permission Permission
	err = p.db.GetContext(ctx, &permission, query, args...)
	if err != nil {
		return nil, err
	}

	return &permission, nil
}

func (p *PermissionRepository) AssignToUser(ctx context.Context, userID *uuid.UUID, permissionIDs []uuid.UUID, tx *sqlx.Tx) error {
	// Use an UPSERT to avoid duplicates without a separate check.
	builder := p.psql.Insert("user_permissions").
		Columns("user_id", "permission_id").
		Suffix("ON CONFLICT (user_id, permission_id) DO NOTHING")

	for _, permissionID := range permissionIDs {
		builder = builder.Values(userID, permissionID)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = p.db.ExecContext(ctx, query, args...)
	return err
}

func (p *PermissionRepository) List(ctx context.Context, filter *PermissionRepositoryFilter) ([]Permission, error) {
	query, args, err := p.generateQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}

	var permissions []Permission
	err = p.db.SelectContext(ctx, &permissions, query, args...)

	return permissions, err
}

func (p *PermissionRepository) RevokeFromUser(ctx context.Context, userID *uuid.UUID, permissionIDs []uuid.UUID, tx *sqlx.Tx) error {
	builder := p.psql.Delete("user_permissions").
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"permission_id": permissionIDs})

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = p.db.ExecContext(ctx, query, args...)
	return err
}
