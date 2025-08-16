package handlers

import (
	"time"

	"github.com/google/uuid"
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
	ID               uuid.UUID `json:"id"`
	Email            string    `json:"email"`
	FirstName        string    `json:"first_name"`
	LastName         string    `json:"last_name"`
	Phone            string    `json:"phone"`
	Address          string    `json:"address"`
	EmailConfirmedAt time.Time `json:"email_confirmed_at"`
	PasswordHash     string    `json:"password_hash"`
	NextOfKinName    string    `json:"next_of_kin_name"`
	NextOfKinPhone   string    `json:"next_of_kin_phone"`
}
