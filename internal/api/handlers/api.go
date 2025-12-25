package handlers

import (
	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

type Handlers struct {
	factory *factory.Factory
	config  *config.Config

	validate *validator.Validate
	trans    ut.Translator
}

func NewHandlers(factory *factory.Factory, config *config.Config, validate *validator.Validate, trans ut.Translator) *Handlers {
	return &Handlers{
		factory:  factory,
		config:   config,
		validate: validate,
		trans:    trans,
	}

}
