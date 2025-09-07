package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/api/handlers"
	"github.com/Jidetireni/ara-cooperative/internal/config"
)

type Server struct {
	Config   *config.Config
	Factory  factory.Factory
	Handlers *handlers.Handlers
}

func NewServer() (*Server, func(), error) {
	cfg := config.New()

	factory, cleanup, err := factory.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	handlers := handlers.NewHandlers(factory, cfg)

	server := &Server{
		Config:   cfg,
		Factory:  *factory,
		Handlers: handlers,
	}

	server.router()
	return server, cleanup, nil
}

func (s *Server) Start() {
	fmt.Printf(" Server running on http://localhost:%s%s\n", s.Config.Server.Port, "/api/v1")

	srv := &http.Server{
		Addr:         ":" + s.Config.Server.Port,
		Handler:      s.Factory.Router,
		WriteTimeout: time.Second * 50,
		ReadTimeout:  time.Second * 30,
		IdleTimeout:  time.Minute,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
