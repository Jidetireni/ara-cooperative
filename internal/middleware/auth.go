package middleware

import (
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
)

// RequireAuth is a middleware factory. It takes a required JWT type
// and returns a middleware function that enforces it.
func (m *Middleware) RequireAuth(requiredType token.JWTType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// try to get the from the authorization header first
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// if not found in header, check for a cookie
			if tokenString == "" {
				cookie, err := r.Cookie(token.AccessTokenName)
				if err == nil {
					tokenString = cookie.Value
				}
			}

			if tokenString == "" {
				m.apiError(w, "Unauthorized: No token provided", http.StatusUnauthorized)
				return
			}

			claims, err := m.TokenSvc.ValidateToken(tokenString)
			if err != nil {
				m.apiError(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}

			if claims.JwtType != requiredType {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			userCtx := users.UserContextValue{
				ID:                      claims.ID,
				Email:                   claims.Email,
				Roles:                   claims.Roles,
				IsAuthenticatedAsMember: claims.JwtType == token.JWTTypeMember,
				IsAuthenticatedAsAdmin:  claims.JwtType == token.JWTTypeAdmin,
			}

			ctx := users.NewContextWithUser(r.Context(), userCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
