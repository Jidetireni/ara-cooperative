package factory

import (
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/pkg/database"
	"github.com/go-chi/chi"
)

type Factory struct {
	DB     *database.PostgresDB
	Router *chi.Mux
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, cleanup, err := database.New(cfg.Database.URL, cfg.Database.Type)
	if err != nil {
		return nil, nil, err
	}

	return &Factory{
			DB:     db,
			Router: chi.NewRouter(),
		}, func() {
			cleanup()
		}, nil
}
