package main

import (
	"log"

	"github.com/Jidetireni/ara-cooperative.git/cmd/seed/seed"
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
)

func main() {

	cfg := config.New()
	if !cfg.IsDev {
		log.Fatal("Seeding is only allowed in development environment")
	}

	seeder, cleanup := seed.NewSeeder(cfg)
	defer cleanup()

	seeder.ResetDB()
	seeder.CreateRootUser()
}
