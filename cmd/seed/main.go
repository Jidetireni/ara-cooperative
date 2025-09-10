package main

import (
	"log"

	"github.com/Jidetireni/ara-cooperative/cmd/seed/seed"
	"github.com/Jidetireni/ara-cooperative/internal/config"
)

func main() {

	cfg := config.New()
	if !cfg.IsDev {
		log.Fatal("Seeding is only allowed in development environment")
	}

	seeder, cleanup, err := seed.NewSeeder(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize seeder: %v", err)
	}

	defer cleanup()
	seeder.ResetDB()
	seeder.CreateRootUser()
}
