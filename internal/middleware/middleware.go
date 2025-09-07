package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/pkg/token"
)

// TODO: add more midddleware, rate limiting,
// handle context very well too
type Middleware struct {
	TokenSvc *token.Jwt
}

func New(tokenSvc *token.Jwt) *Middleware {
	return &Middleware{TokenSvc: tokenSvc}
}

func (m *Middleware) apiError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"message": message,
		"status":  code,
	})
}
