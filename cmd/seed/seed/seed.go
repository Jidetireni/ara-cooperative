package seed

import (
	"context"
	"database/sql"
	"fmt"

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

	fx, cleanup, err := factory.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize factory: %w", err)
	}

	return &Seed{
		Config:    cfg,
		DB:        fx.DB,
		UserRepo:  fx.Repositories.User,
		RolesRepo: fx.Repositories.Role,
	}, cleanup, nil
}

func (s *Seed) ResetDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Resetting database...")
	tx, err := s.DB.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
        TRUNCATE TABLE
            transaction_status,
            transactions,
            tokens,
            members,
            users
        RESTART IDENTITY CASCADE;
    `)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("reset database: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}

	fmt.Println("Database reset completed.")
	return nil
}

func (s *Seed) CreateRootUser() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Creating root user...")
	rootUser := RootUser{
		Email:    s.Config.Server.RootUserEmail,
		Password: s.Config.Server.RootUserPassword,
	}

	roles, err := s.RolesRepo.List(ctx, &repository.RoleRepositoryFilter{})
	if err != nil {
		return fmt.Errorf("fetch roles: %w", err)
	}
	if len(roles) == 0 {
		return fmt.Errorf("no roles found; ensure roles are seeded before creating root user")
	}

	exists, err := s.UserRepo.Exists(ctx, repository.UserRepositoryFilter{Email: &rootUser.Email})
	if err != nil {
		return fmt.Errorf("check root user existence: %w", err)
	}
	if exists {
		fmt.Println("Root user already exists. Skipping creation.")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(rootUser.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
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

	// Begin transaction for atomicity
	tx, err := s.DB.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	createdUser, err := s.UserRepo.Create(ctx, userModel, tx)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("create root user: %w", err)
	}

	roleIDs := lo.Map(roles, func(r repository.Role, _ int) uuid.UUID {
		return r.ID
	})

	err = s.RolesRepo.AssignToUser(ctx, &createdUser.ID, roleIDs, nil)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("assign roles to root user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit root user creation: %w", err)
	}

	fmt.Println("Root user created successfully.")
	return nil
}
