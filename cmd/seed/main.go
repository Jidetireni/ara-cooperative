package main

import (
	"log"
	"os"

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
		log.Printf("Failed to reset database: %v", err)
		cleanup()
		os.Exit(1)
	}

	// Seed all data
	if err := seeder.SeedAll(); err != nil {
		log.Printf("Failed to seed database: %v", err)
		cleanup()
		os.Exit(1)
	}

	log.Println("Database seeding completed successfully!")
}
