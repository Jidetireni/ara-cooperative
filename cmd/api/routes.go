package main

import (
	"github.com/Jidetireni/ara-cooperative/pkg/token"
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
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeAdmin))
				r.Post("/", s.Handlers.CreateMember)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeMember))
				r.Route("/me", func(r chi.Router) {
					r.Get("/savings/balance", s.Handlers.SavingsBalance)
				})

				r.Get("/{slug}", s.Handlers.MemberBySlug)
			})
		})

		r.Route("/savings", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeMember))
				r.Post("/", s.Handlers.DepositSavings)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeAdmin))
				r.Get("/pending", s.Handlers.ListPendingDeposits)

			})
		})

		r.Route("/transactions", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeAdmin))
				r.Patch("/{id}/status", s.Handlers.UpdateStatus)
			})
		})

		r.Route("/shares", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeAdmin))
				r.Patch("/unit-price", s.Handlers.SetShareUnitPrice)
				r.Get("/pending", s.Handlers.ListPendingSharesTransactions)
				r.Get("/total", s.Handlers.GetTotalSharesPurchased)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth(token.JWTTypeMember))
				r.Get("/unit-price", s.Handlers.GetShareUnitPrice)
				r.Get("/quotes", s.Handlers.GetShareQuote)
				r.Post("/", s.Handlers.BuyShares)
				r.Get("/me/total", s.Handlers.GetMemberTotalSharesPurchased)
			})

		})
	})
}
