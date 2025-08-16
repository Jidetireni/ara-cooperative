package handlers

import (
	"github.com/Jidetireni/ara-cooperative.git/factory"
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
)

type Handlers struct {
	factory *factory.Factory
	config  *config.Config
}

func NewHandlers(factory *factory.Factory, config *config.Config) *Handlers {
	return &Handlers{
		factory: factory,
		config:  config,
	}
}
