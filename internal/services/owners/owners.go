package owners

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Repository interface {
	Bootstrap(context.Context, repository.OwnerBootstrapParams) (*repository.User, bool, error)
}

type Owner struct {
	repo Repository
}

type BootstrapResult struct {
	ID         uuid.UUID
	Email      string
	SetupToken string
	ExpiresAt  time.Time
}

func New(repo Repository) *Owner {
	return &Owner{repo: repo}
}

func (o *Owner) Bootstrap(ctx context.Context, email string) (*BootstrapResult, error) {
	return o.bootstrap(ctx, email, "")
}

// BootstrapWithPassword initializes a pending owner without resetting an existing password.
func (o *Owner) BootstrapWithPassword(ctx context.Context, email, password string) (*BootstrapResult, error) {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return nil, fmt.Errorf("OWNER_PASSWORD must contain at least 8 characters and at most 72 bytes")
	}
	return o.bootstrap(ctx, email, password)
}

func (o *Owner) bootstrap(ctx context.Context, email, password string) (*BootstrapResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return nil, fmt.Errorf("OWNER_EMAIL must be a valid email address without a display name")
	}
	raw, err := helpers.GenerateSetupToken()
	if err != nil {
		return nil, fmt.Errorf("generate owner setup token: %w", err)
	}
	expiresAt := time.Now().Add(30 * time.Minute)
	input := repository.OwnerBootstrapParams{
		Email: email, SetupHash: helpers.HashToken(raw), SetupExpiresAt: expiresAt,
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash initial owner password: %w", err)
		}
		input.PasswordHash = string(hash)
	}
	user, needsSetup, err := o.repo.Bootstrap(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bootstrap owner: %w", err)
	}
	result := &BootstrapResult{ID: user.ID, Email: user.Email}
	if needsSetup {
		result.SetupToken = raw
		result.ExpiresAt = expiresAt
	}
	return result, nil
}
