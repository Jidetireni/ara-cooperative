package dto

import (
	"time"

	"github.com/google/uuid"
)

type SavingsStatus string
type TransactionType string

const (
	SavingsStatusPending   SavingsStatus = "PENDING"
	SavingsStatusConfirmed SavingsStatus = "CONFIRMED"
	SavingsStatusRejected  SavingsStatus = "REJECTED"

	TransactionTypeDeposit    TransactionType = "DEPOSIT"
	TransactionTypeWithdrawal TransactionType = "WITHDRAWAL"
)

type CreateMemberInput struct {
	Email          string `json:"email" validate:"required,email"`
	FirstName      string `json:"first_name" validate:"required"`
	LastName       string `json:"last_name" validate:"required"`
	Phone          string `json:"phone" validate:"required"`
	Address        string `json:"address" validate:"required"`
	NextOfKinName  string `json:"next_of_kin_name" validate:"required"`
	NextOfKinPhone string `json:"next_of_kin_phone" validate:"required"`
}

type Member struct {
	ID             uuid.UUID `json:"id"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Slug           string    `json:"slug"`
	Phone          string    `json:"phone"`
	Address        string    `json:"address"`
	NextOfKinName  string    `json:"next_of_kin_name"`
	NextOfKinPhone string    `json:"next_of_kin_phone"`
}

type User struct {
	ID               uuid.UUID  `json:"id"`
	Email            string     `json:"email"`
	PasswordHash     string     `json:"password_hash,omitempty"`
	EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type SetPasswordInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Token    string `json:"token" validate:"required"`
}

type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
	User         *AuthUser `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
}

type AuthUser struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	Roles       []string   `json:"roles"`
	Member      *Member    `json:"member,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

type JWTClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Roles  []string  `json:"roles"`
	Type   string    `json:"type"` // "access" or "refresh"
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

type SavingsDepositInput struct {
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	Description string `json:"description" validate:"required"`
}

type Savings struct {
	TransactionID   uuid.UUID       `json:"transaction_id"`
	Amount          int64           `json:"amount"`
	Description     string          `json:"description"`
	TransactionType TransactionType `json:"transaction_type"`
	Reference       string          `json:"reference"`
	Status          SavingsStatus   `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
}

type QueryOptions struct {
	Limit  uint32  `json:"limit"`
	Cursor *string `json:"cursor,omitempty"`
	Sort   *string `json:"sort,omitempty"`
}
