package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/owners"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Owner bootstrap failed: %v\n", err)
		os.Exit(1)
	}
}

func run() (err error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load environment: %w", err)
	}
	email := os.Getenv("OWNER_EMAIL")
	dsn := os.Getenv("DB_URL")
	if email == "" || dsn == "" {
		return fmt.Errorf("OWNER_EMAIL and DB_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close bootstrap database: %w", closeErr))
		}
	}()
	db.Mapper = reflectx.NewMapper("json")
	bootstrap := owners.New(repository.NewOwnerRepository(db))
	var result *owners.BootstrapResult
	if password := os.Getenv("OWNER_PASSWORD"); password != "" {
		result, err = bootstrap.BootstrapWithPassword(ctx, email, password)
	} else {
		result, err = bootstrap.Bootstrap(ctx, email)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Owner ready: %s (%s)\n", result.Email, result.ID)
	if result.SetupToken == "" {
		fmt.Println("Sign in using your account password.")
		return nil
	}
	fmt.Printf("Set your password with POST /api/v1/set-password using:\nemail: %s\ntoken: %s\n", result.Email, result.SetupToken)
	fmt.Printf("Supply your chosen password. This one-time setup token expires at %s.\n", result.ExpiresAt.UTC().Format(time.RFC3339))
	return nil
}
