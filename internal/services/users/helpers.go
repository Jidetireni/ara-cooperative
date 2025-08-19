package users

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey struct {
	name string
}

var userCtxKey = &contextKey{"user"}

type UserContextValue struct {
	Writer http.ResponseWriter
	// User Identifiers
	ID       uuid.UUID
	MemberID *uuid.UUID

	SessionID uuid.UUID
	// Auth
	RefreshToken            string
	AccessToken             string
	IsAuthenticatedAsMember bool
	IsAuthenticatedAsAdmin  bool

	// Admin Details
	Roles []string
}

func NewContextWithUser(ctx context.Context, user UserContextValue) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// get user from context
func FromContext(ctx context.Context) UserContextValue {
	raw, _ := ctx.Value(userCtxKey).(UserContextValue)
	return raw
}
