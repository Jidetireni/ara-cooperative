package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
)

// LoadIdentity resolves current access from storage so revocation affects existing tokens.
func (u *User) LoadIdentity(ctx context.Context, userID uuid.UUID) (*UserContextValue, error) {
	if userID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{ID: &userID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.UnauthenticatedError()
	}
	if err != nil {
		return nil, fmt.Errorf("load authenticated account: %w", err)
	}
	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{UserID: &userID})
	if err != nil {
		return nil, fmt.Errorf("load account roles: %w", err)
	}
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}
	permissions, err := u.PermissionRepo.List(ctx, &repository.PermissionRepositoryFilter{UserID: &userID})
	if err != nil {
		return nil, fmt.Errorf("load account permissions: %w", err)
	}
	owner := slices.Contains(roleNames, constants.RoleOwner)
	member, err := u.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{UserID: &userID}, nil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load membership access: %w", err)
	}
	activeMember := err == nil && member.ActivatedAt.Valid
	return &UserContextValue{
		ID: user.ID, Email: user.Email, Roles: roleNames,
		Permissions:             effectivePermissions(roleNames, permissions),
		IsAuthenticatedAsOwner:  owner,
		IsAuthenticatedAsAdmin:  owner || slices.Contains(roleNames, constants.RoleAdmin),
		IsAuthenticatedAsMember: slices.Contains(roleNames, constants.RoleMember),
		HasActiveMembership:     activeMember,
	}, nil
}

func effectivePermissions(roles []string, assigned []repository.Permission) []string {
	permissions := []string{}
	for _, permission := range assigned {
		permissions = append(permissions, permission.Slug)
	}
	for _, role := range roles {
		for _, permission := range constants.RolePermissions[role] {
			permissions = append(permissions, string(permission))
		}
	}
	slices.Sort(permissions)
	return slices.Compact(permissions)
}
