package middleware

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
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

		userCtx, err := m.Identity.LoadIdentity(r.Context(), claims.ID)
		if err != nil {
			var apiErr *svc.APIError
			if errors.As(err, &apiErr) {
				m.apiError(w, apiErr.Message, apiErr.Status)
				return
			}
			m.Logger.Sugar.Errorw("failed to load authenticated account", "error", err)
			m.apiError(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r.WithContext(users.NewContextWithUser(r.Context(), userCtx)))
	})
}

func (m *Middleware) RequireActiveMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := users.FromContext(r.Context())
		if !ok {
			m.apiError(w, "Unauthorized: No user found", http.StatusUnauthorized)
			return
		}
		if !actor.HasActiveMembership {
			m.apiError(w, "active membership required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
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

			hasRole := slices.Contains(claims.Roles, requiredRole)
			if requiredRole == constants.RoleAdmin && claims.IsAuthenticatedAsOwner {
				hasRole = true
			}

			if !hasRole {
				m.apiError(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
