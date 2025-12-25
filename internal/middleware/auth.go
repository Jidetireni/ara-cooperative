package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/services/users"
)

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := ""

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
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

		userCtx := users.UserContextValue{
			ID:          claims.ID,
			Email:       claims.Email,
			Roles:       claims.Roles,
			Permissions: claims.Permissions,
		}

		if slices.Contains(claims.Roles, "member") {
			userCtx.IsAuthenticatedAsMember = true
		}
		if slices.Contains(claims.Roles, "admin") {
			userCtx.IsAuthenticatedAsAdmin = true
		}

		next.ServeHTTP(w, r.WithContext(users.NewContextWithUser(r.Context(), &userCtx)))
	})
}

func (m *Middleware) RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			claims, ok := users.FromContext(r.Context())
			if !ok {
				m.apiError(w, "Unauthorized: No user found", http.StatusUnauthorized)
				return
			}

			hasRole := false
			for _, role := range claims.Roles {
				if role == requiredRole {
					hasRole = true
					break
				}
			}

			if !hasRole {
				m.apiError(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
