package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/config"
	"github.com/Jidetireni/ara-cooperative.git/factory"
)

type Server struct {
	Config  *config.Config
	Factory factory.Factory
}

func NewServer() (*Server, func(), error) {
	cfg := config.New()

	factory, cleanup, err := factory.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	return &Server{
		Config:  cfg,
		Factory: *factory,
	}, cleanup, nil
}

func (s *Server) Start() {
	fmt.Printf(" Server running on http://localhost:%s%s\n", s.Config.Server.Port, "/api/v1")

	if err := http.ListenAndServe(":"+s.Config.Server.Port, s.Factory.Router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
