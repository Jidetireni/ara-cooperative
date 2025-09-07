package users

import (
	"context"
	"net/http"
	"slices"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/google/uuid"
)

type contextKey struct {
	name string
}

var userCtxKey = &contextKey{"user"}

type UserContextValue struct {
	Writer http.ResponseWriter
	// User Identifiers
	ID    uuid.UUID
	Email string
	Roles []string

	SessionID uuid.UUID
	// Auth
	RefreshToken            string
	AccessToken             string
	IsAuthenticatedAsMember bool
	IsAuthenticatedAsAdmin  bool
}

func NewContextWithUser(ctx context.Context, user UserContextValue) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// get user from context
func FromContext(ctx context.Context) UserContextValue {
	raw, _ := ctx.Value(userCtxKey).(UserContextValue)
	return raw
}

func HasAdminPermissions(ctx context.Context, permissions []constants.UserPermissions) bool {
	user := FromContext(ctx)
	if !user.IsAuthenticatedAsAdmin {
		return false
	}

	userPermissions := user.Roles
	for _, permission := range permissions {
		if !slices.Contains(userPermissions, string(permission)) {
			return false
		}

	}

	return true
}
