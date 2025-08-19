package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type RoleRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewRoleRepository(db *sqlx.DB) *RoleRepository {
	return &RoleRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

type RoleRepositoryFilter struct {
	RoleID     *uuid.UUID
	UserID     *uuid.UUID
	Permission *string
}

// filterQuery applies the common WHERE clauses to a select builder.
// This is the core generic function that avoids repeating filter logic.
func (rr *RoleRepository) filterQuery(builder sq.SelectBuilder, filter RoleRepositoryFilter) sq.SelectBuilder {
	// Joining on roles table is only needed if permission filter is used.
	if filter.Permission != nil {
		builder = builder.Join("roles r ON ur.role_id = r.id")
	}

	if filter.UserID != nil {
		builder = builder.Where(sq.Eq{"ur.user_id": *filter.UserID})
	}

	if filter.RoleID != nil {
		builder = builder.Where(sq.Eq{"ur.role_id": *filter.RoleID})
	}

	if filter.Permission != nil {
		builder = builder.Where(sq.Eq{"r.permission": *filter.Permission})
	}

	return builder
}

// GetUserRoles lists all roles associated with a user ID.
func (rr *RoleRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	builder := rr.psql.Select("r.permission").From("user_roles ur").Join("roles r ON ur.role_id = r.id")

	// Apply the filter
	filteredBuilder := rr.filterQuery(builder, RoleRepositoryFilter{
		UserID: &userID,
	})

	query, args, err := filteredBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	var roles []string
	err = rr.db.SelectContext(ctx, &roles, query, args...)
	return roles, err
}

// CheckUserHasPermission checks if a user has a specific permission.
func (rr *RoleRepository) CheckUserHasPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	// Build a count query to check for the existence of the user-permission link.
	builder := rr.psql.Select("COUNT(*)").From("user_roles ur")

	// Apply filters for both UserID and Permission.
	filteredBuilder := rr.filterQuery(builder, RoleRepositoryFilter{
		UserID:     &userID,
		Permission: &permission,
	})

	query, args, err := filteredBuilder.ToSql()
	if err != nil {
		return false, err
	}

	var count int
	if err := rr.db.GetContext(ctx, &count, query, args...); err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetRoleByPermission retrieves a single role by its permission string.
func (rr *RoleRepository) GetRoleByPermission(ctx context.Context, permission string) (*Role, error) {
	builder := rr.psql.Select("*").From("roles").Where(sq.Eq{"permission": permission})
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var role Role
	err = rr.db.GetContext(ctx, &role, query, args...)
	if err != nil {
		return nil, err
	}

	return &role, nil
}

// AssignRolesToUser is a generic function to assign multiple roles to a user at once.
func (rr *RoleRepository) AssignRolesToUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error {
	// Use an UPSERT to avoid duplicates without a separate check.
	// This is often more efficient as it's a single database operation.
	builder := rr.psql.Insert("user_roles").
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

	_, err = rr.db.ExecContext(ctx, query, args...)
	return err
}

// RemoveRoleFromUser removes a single role from a user.
func (rr *RoleRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID uuid.UUID, tx *sqlx.Tx) error {
	builder := rr.psql.Delete("user_roles").
		Where(sq.Eq{
			"user_id": userID,
			"role_id": roleID,
		})

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = rr.db.ExecContext(ctx, query, args...)
	return err
}
