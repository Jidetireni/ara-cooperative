package users

import (
	"context"
	"slices"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/google/uuid"
)

type contextKey string

const userKey contextKey = "user"

type UserContextValue struct {
	ID          uuid.UUID
	Email       string
	Roles       []string
	Permissions []string

	IsAuthenticatedAsMember bool
	IsAuthenticatedAsAdmin  bool
}

func NewContextWithUser(ctx context.Context, user *UserContextValue) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func FromContext(ctx context.Context) (*UserContextValue, bool) {
	u, ok := ctx.Value(userKey).(*UserContextValue)
	return u, ok
}

func HasAdminPermissions(ctx context.Context, requiredPermissions []constants.UserPermissions) bool {
	user, ok := FromContext(ctx)
	if !ok {
		return false
	}

	if !user.IsAuthenticatedAsAdmin {
		return false
	}

	for _, req := range requiredPermissions {
		if !slices.Contains(user.Permissions, string(req)) {
			return false
		}
	}

	return true
}
