package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
)

func (u *User) GrantAdmin(ctx context.Context, email string) (*dto.AdminAccount, error) {
	if err := u.requireOwner(ctx); err != nil {
		return nil, err
	}
	email = strings.TrimSpace(email)
	target, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{Email: &email}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("find admin account: %w", err)
	}
	return u.setAdminAccess(ctx, target, true)
}

func (u *User) RevokeAdmin(ctx context.Context, userID uuid.UUID) (*dto.AdminAccount, error) {
	if err := u.requireOwner(ctx); err != nil {
		return nil, err
	}
	target, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{ID: &userID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("find admin account: %w", err)
	}
	return u.setAdminAccess(ctx, target, false)
}

func (u *User) requireOwner(ctx context.Context) error {
	actor, ok := FromContext(ctx)
	if !ok {
		return svc.UnauthenticatedError()
	}
	identity, err := u.LoadIdentity(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !identity.IsAuthenticatedAsOwner {
		return &svc.APIError{Status: http.StatusForbidden, Message: "owner access required"}
	}
	return nil
}

func (u *User) setAdminAccess(ctx context.Context, target *repository.User, grant bool) (result *dto.AdminAccount, err error) {
	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{UserID: &target.ID})
	if err != nil {
		return nil, fmt.Errorf("load target roles: %w", err)
	}
	for _, role := range roles {
		if role.Name == constants.RoleOwner {
			return nil, &svc.APIError{Status: http.StatusConflict, Message: "owner access cannot be changed through admin management"}
		}
	}
	adminRoles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{Name: []string{constants.RoleAdmin}})
	if err != nil {
		return nil, fmt.Errorf("load admin role: %w", err)
	}
	if len(adminRoles) != 1 {
		return nil, fmt.Errorf("admin role is missing; apply database setup first")
	}
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin admin access change: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback admin access change: %w", rollbackErr))
		}
	}()
	roleID := adminRoles[0].ID
	if grant {
		err = u.RoleRepo.AssignToUser(ctx, &target.ID, []uuid.UUID{roleID}, tx)
	} else {
		err = u.RoleRepo.RevokeFromUser(ctx, &target.ID, []uuid.UUID{roleID}, tx)
	}
	if err != nil {
		return nil, fmt.Errorf("update admin access: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit admin access change: %w", err)
	}
	roleNames := []string{}
	for _, role := range roles {
		if role.Name != constants.RoleAdmin {
			roleNames = append(roleNames, role.Name)
		}
	}
	if grant {
		roleNames = append(roleNames, constants.RoleAdmin)
	}
	slices.Sort(roleNames)
	return &dto.AdminAccount{ID: target.ID, Email: target.Email, Roles: roleNames}, nil
}
