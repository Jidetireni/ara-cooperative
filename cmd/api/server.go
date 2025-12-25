package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/api/handlers"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
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

	validate := validator.New()

	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")
	if err := en_translations.RegisterDefaultTranslations(validate, trans); err != nil {
		return nil, nil, err
	}

	handlers := handlers.NewHandlers(factory, cfg, validate, trans)
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
