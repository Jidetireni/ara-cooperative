package factory

import (
	"github.com/Jidetireni/ara-cooperative.git/config"
	"github.com/Jidetireni/ara-cooperative.git/services/postgresql"
	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
)

type Factory struct {
	DB     *sqlx.DB
	Router *chi.Mux
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, cleanup, err := postgresql.New(cfg.Database.URL)
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
