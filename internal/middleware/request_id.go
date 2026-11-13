package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

func (m *Middleware) RequestIDResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}
