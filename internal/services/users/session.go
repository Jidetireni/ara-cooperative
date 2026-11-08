package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
)

// CurrentUser loads the signed-in account from storage rather than token snapshots.
func (u *User) CurrentUser(ctx context.Context) (*dto.AuthUser, error) {
	actor, ok := FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{ID: &actor.ID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.UnauthenticatedError()
	}
	if err != nil {
		return nil, fmt.Errorf("load current user: %w", err)
	}

	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{UserID: &user.ID})
	if err != nil {
		return nil, fmt.Errorf("load current user roles: %w", err)
	}

	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}

	return u.authUser(ctx, user, roleNames)
}

func (u *User) authUser(ctx context.Context, user *repository.User, roles []string) (*dto.AuthUser, error) {
	result := &dto.AuthUser{
		ID:    user.ID,
		Email: user.Email,
		Roles: roles,
	}
	if user.EmailConfirmedAt.Valid {
		result.ConfirmedAt = &user.EmailConfirmedAt.Time
	}

	member, err := u.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{UserID: &user.ID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load account membership: %w", err)
	}

	result.Member = u.MemberRepo.MapRepositoryToDTOModel(member)
	return result, nil
}
