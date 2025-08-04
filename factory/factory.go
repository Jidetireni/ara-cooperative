package factory

import (
	"github.com/Jidetireni/asynchronous-API/config"
	"github.com/Jidetireni/asynchronous-API/services/postgresql"
	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
)

type Services struct {
	PostgresDB *postgresql.PostgresDB
}

type Postgres struct {
	DB *sqlx.DB
}

type Infra struct {
	Postgres *Postgres
}

type Core struct {
	Router *chi.Mux
}

type Factory struct {
	Services *Services
	Infra    *Infra
	Core     *Core
}

func New(cfg *config.Config) (*Factory, func()) {
	postgresDB := postgresql.New(cfg.Database.URL)

	services := &Services{
		PostgresDB: postgresDB,
	}

	infra := &Infra{
		Postgres: &Postgres{
			DB: postgresDB.DB,
		},
	}

	core := &Core{
		Router: chi.NewRouter(),
	}

	cleanUp := func() {
		infra.Postgres.DB.Close()
	}

	return &Factory{
		Services: services,
		Infra:    infra,
		Core:     core,
	}, cleanUp
}
