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

	// Reset database first
	if err := seeder.ResetDB(); err != nil {
		log.Fatalf("Failed to reset database: %v", err)
	}

	// Seed all data
	if err := seeder.SeedAll(); err != nil {
		log.Fatalf("Failed to seed database: %v", err)
	}

	log.Println("Database seeding completed successfully!")
}
