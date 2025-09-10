package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"time"

	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/pkg/database"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

type Seed struct {
	Config    *config.Config
	DB        *database.PostgresDB
	UserRepo  *repository.UserRepository
	RolesRepo *repository.RoleRepository
}

type RootUser struct {
	Email    string
	Password string
}

func NewSeeder(cfg *config.Config) (*Seed, func(), error) {

	if !cfg.IsDev {
		return nil, nil, fmt.Errorf("seeding is only allowed in development environment")
	}

	factory, cleanup, err := factory.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize factory: %w", err)
	}

	return &Seed{
		Config:    cfg,
		DB:        factory.DB,
		UserRepo:  factory.Repositories.User,
		RolesRepo: factory.Repositories.Role,
	}, cleanup, nil
}

func (s *Seed) ResetDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Resetting database...")
	_, err := s.DB.DB.ExecContext(ctx, `
		TRUNCATE TABLE
			transaction_status,
			transactions,
			tokens,
			members,
			users
		RESTART IDENTITY CASCADE;
	`)
	if err != nil {
		log.Fatalf("Failed to reset database: %v", err)
	}

	fmt.Println("Database reset completed.")
}

func (s *Seed) CreateRootUser() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Creating root user...")
	rootUser := RootUser{
		Email:    s.Config.Server.RootUserEmail,
		Password: s.Config.Server.RootUserPassword,
	}

	roles, err := s.RolesRepo.List(ctx, &repository.RoleRepositoryFilter{})
	if err != nil {
		log.Fatalf("Failed to fetch roles: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(rootUser.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	userModel := &repository.User{
		Email: rootUser.Email,
		PasswordHash: sql.NullString{
			String: string(hash),
			Valid:  true,
		},
		EmailConfirmedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	createdUser, err := s.UserRepo.Create(ctx, userModel, nil)
	if err != nil {
		log.Fatalf("Failed to create root user: %v", err)
	}

	rolesID := lo.Map(roles, func(r repository.Role, _ int) uuid.UUID {
		return r.ID
	})

	err = s.RolesRepo.AssignToUser(ctx, &createdUser.ID, rolesID, nil)
	if err != nil {
		log.Fatalf("Failed to assign roles to root user: %v", err)
	}

	fmt.Println("Root user created successfully.")
}
