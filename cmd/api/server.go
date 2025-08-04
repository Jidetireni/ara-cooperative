package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Jidetireni/asynchronous-API/config"
	"github.com/Jidetireni/asynchronous-API/factory"
)

type Server struct {
	Config  *config.Config
	Factory *factory.Factory
}

func NewServer() (*Server, func()) {
	cfg := config.New()
	factory, cleanup := factory.New(cfg)

	return &Server{
		Config:  cfg,
		Factory: factory,
	}, cleanup
}

func (s *Server) Start() {
	fmt.Printf(" Server running on http://localhost:%s%s\n", s.Config.Server.Port, "/api/v1")

	if err := http.ListenAndServe(":"+s.Config.Server.Port, s.Factory.Core.Router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
