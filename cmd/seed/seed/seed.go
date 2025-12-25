package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/pkg/database"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

type Seed struct {
	Config          *config.Config
	DB              *database.PostgresDB
	UserRepo        *repository.UserRepository
	RolesRepo       *repository.RoleRepository
	PermissionRepo  *repository.PermissionRepository
	MemberRepo      *repository.MemberRepository
	TokenRepo       *repository.TokenRepository
	TransactionRepo *repository.TransactionRepository
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
		Config:          cfg,
		DB:              fx.Pkgs.DB,
		UserRepo:        fx.Repositories.User,
		RolesRepo:       fx.Repositories.Role,
		PermissionRepo:  fx.Repositories.Permission,
		MemberRepo:      fx.Repositories.Member,
		TokenRepo:       fx.Repositories.Token,
		TransactionRepo: fx.Repositories.Transaction,
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
	defer tx.Rollback()

	// Order matters due to foreign key constraints
	tables := []string{
		"transaction_status",
		"transactions",
		"tokens",
		"user_roles",
		"members",
		"users",
	}

	for _, table := range tables {
		_, err = tx.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table))
		if err != nil {
			return fmt.Errorf("truncate table %s: %w", table, err)
		}
		fmt.Printf("Truncated table: %s\n", table)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}

	fmt.Println("Database reset completed.")
	return nil
}

func (s *Seed) SeedAll() error {
	fmt.Println("Starting database seeding...")

	if err := s.CreateRootUser(); err != nil {
		return fmt.Errorf("failed to create root user: %w", err)
	}

	if err := s.CreateDefaultMembers(); err != nil {
		return fmt.Errorf("failed to create default members: %w", err)
	}

	if err := s.CreateSampleTransactions(); err != nil {
		return fmt.Errorf("failed to create sample transactions: %w", err)
	}

	fmt.Println("Database seeding completed successfully!")
	return nil
}

func (s *Seed) CreateRootUser() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Creating root user...")

	// Check if root user already exists
	exists, err := s.UserRepo.Exists(ctx, repository.UserRepositoryFilter{
		Email: &s.Config.Server.RootUserEmail,
	})
	if err != nil {
		return fmt.Errorf("check root user existence: %w", err)
	}
	if exists {
		fmt.Println("Root user already exists. Skipping creation.")
		return nil
	}

	// Get all roles for root user
	roles, err := s.RolesRepo.List(ctx, &repository.RoleRepositoryFilter{})
	if err != nil {
		return fmt.Errorf("fetch roles: %w", err)
	}
	if len(roles) == 0 {
		return fmt.Errorf("no roles found; ensure roles are seeded before creating root user")
	}

	permissions, err := s.PermissionRepo.List(ctx, &repository.PermissionRepositoryFilter{})
	if err != nil {
		return fmt.Errorf("fetch permissions: %w", err)
	}
	if len(permissions) == 0 {
		return fmt.Errorf("no permissions found; ensure permissions are seeded before creating root user")
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(s.Config.Server.RootUserPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Begin transaction
	tx, err := s.DB.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Create user
	user := &repository.User{
		Email: s.Config.Server.RootUserEmail,
		PasswordHash: sql.NullString{
			String: string(hash),
			Valid:  true,
		},
		EmailConfirmedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	createdUser, err := s.UserRepo.Create(ctx, user, tx)
	if err != nil {
		return fmt.Errorf("create root user: %w", err)
	}

	// Assign all roles to root user
	roleIDs := lo.Map(roles, func(r repository.Role, _ int) uuid.UUID {
		return r.ID
	})

	err = s.RolesRepo.AssignToUser(ctx, &createdUser.ID, roleIDs, tx)
	if err != nil {
		return fmt.Errorf("assign roles to root user: %w", err)
	}

	permissionsIDs := lo.Map(permissions, func(p repository.Permission, _ int) uuid.UUID {
		return p.ID
	})

	err = s.PermissionRepo.AssignToUser(ctx, &createdUser.ID, permissionsIDs, tx)
	if err != nil {
		return fmt.Errorf("assign permissions to root user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit root user creation: %w", err)
	}

	fmt.Printf("Root user created successfully: %s\n", createdUser.Email)
	return nil
}

func (s *Seed) CreateDefaultMembers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Creating default members...")

	// Get member roles (non-admin permissions)
	memberRole := []string{
		string(constants.RoleMember),
	}

	memberRoles, err := s.RolesRepo.List(ctx, &repository.RoleRepositoryFilter{
		Name: memberRole,
	})
	if err != nil {
		return fmt.Errorf("fetch member roles: %w", err)
	}

	createdCount := 0
	for _, userData := range Users {
		member, err := s.createUserWithMember(ctx, userData, memberRoles)
		if err != nil {
			fmt.Printf("Warning: Failed to create member %s: %v\n", userData.Email, err)
			continue
		}
		if member != nil {
			fmt.Printf("Created member: %s %s (%s) - Slug: %s\n",
				userData.FirstName, userData.LastName, userData.Email, member.Slug)
			createdCount++
		}
	}

	fmt.Printf("Successfully created %d out of %d default members.\n", createdCount, len(Users))
	return nil
}

func (s *Seed) CreateSampleTransactions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Creating sample transactions...")

	// Get all members
	result, err := s.MemberRepo.List(ctx, repository.MemberRepositoryFilter{}, repository.QueryOptions{
		Limit: 100, // Ensure we get all members
	})
	if err != nil {
		return fmt.Errorf("fetch members: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No members found, skipping transaction creation")
		return nil
	}

	totalTransactions := 0
	for _, member := range result.Items {
		err := s.createSavingsTransactionsForMember(ctx, member.ID, SavingsTransactions)
		if err != nil {
			fmt.Printf("Warning: Failed to create transactions for member %s: %v\n", member.Slug, err)
			continue
		}
		totalTransactions += len(SavingsTransactions)
		fmt.Printf("Created %d transactions for member: %s\n", len(SavingsTransactions), member.Slug)
	}

	fmt.Printf("Successfully created %d sample transactions across %d members.\n",
		totalTransactions, len(result.Items))
	return nil
}
