package seed

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

func (s *Seed) createUserWithMember(ctx context.Context, seedUser SeedMember, roles []repository.Role) (*repository.Member, error) {
	// Check if user already exists
	exists, err := s.UserRepo.Exists(ctx, repository.UserRepositoryFilter{Email: &seedUser.Email})
	if err != nil {
		return nil, fmt.Errorf("check user existence: %w", err)
	}
	if exists {
		fmt.Printf("User %s already exists. Skipping creation.\n", seedUser.Email)
		return nil, nil
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(seedUser.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.DB.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Create user
	user := &repository.User{
		Email: seedUser.Email,
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
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Create member with unique slug
	memberSlug := fmt.Sprintf("ara%06d", helpers.GetNextMemberNumber())
	member := &repository.Member{
		UserID:    createdUser.ID,
		Slug:      memberSlug,
		FirstName: seedUser.FirstName,
		LastName:  seedUser.LastName,
		Phone:     seedUser.Phone,
		Address: sql.NullString{
			String: seedUser.Address,
			Valid:  seedUser.Address != "",
		},
		NextOfKinName: sql.NullString{
			String: fmt.Sprintf("%s %s Sr.", seedUser.FirstName, seedUser.LastName),
			Valid:  true,
		},
		NextOfKinPhone: sql.NullString{
			String: "+9876543210",
			Valid:  true,
		},
	}

	createdMember, err := s.MemberRepo.Create(ctx, member, tx)
	if err != nil {
		return nil, fmt.Errorf("create member: %w", err)
	}

	// Assign roles
	roleIDs := lo.Map(roles, func(r repository.Role, _ int) uuid.UUID {
		return r.ID
	})

	err = s.RolesRepo.AssignToUser(ctx, &createdUser.ID, roleIDs, tx)
	if err != nil {
		return nil, fmt.Errorf("assign roles: %w", err)
	}

	// Create a password reset token for demonstration (members can use these to set new passwords)
	tokenUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generate token UUID: %w", err)
	}
	tokenID := base64.URLEncoding.EncodeToString(tokenUUID[:])

	_, err = s.TokenRepo.Create(ctx, &repository.Token{
		UserID:    createdUser.ID,
		Token:     tokenID,
		TokenType: token.SetPasswordToken,
		IsValid:   true,
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hours for seed data
	}, tx)
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return createdMember, nil
}
