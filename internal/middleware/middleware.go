package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/google/uuid"
)

// TODO: add more midddleware, rate limiting,
// handle context very well too
type Middleware struct {
	TokenSvc *token.Jwt
	Logger   *logger.Logger
	Identity IdentityLoader
}

type IdentityLoader interface {
	LoadIdentity(context.Context, uuid.UUID) (*users.UserContextValue, error)
}

func New(tokenSvc *token.Jwt, logger *logger.Logger, identity IdentityLoader) *Middleware {
	return &Middleware{
		TokenSvc: tokenSvc,
		Logger:   logger,
		Identity: identity,
	}
}

func (m *Middleware) apiError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"message": message,
		"status":  code,
	})
}
