package main

import (
	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) router() {
	s.Factory.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Logger)

		r.Get("/healthz", s.Handlers.HealthCheckHandler)

		r.Post("/set-password", s.Handlers.SetPassword)
		r.Post("/login", s.Handlers.Login)
		r.Post("/refresh", s.Handlers.RefreshToken)

		r.Route("/members", func(r chi.Router) {
			r.Post("/", s.Handlers.CreateMember)

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeMember))
				r.Get("/{slug}", s.Handlers.MemberBySlug)
			})
		})
	})
}
