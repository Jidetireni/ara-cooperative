package handlers

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
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
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        time.Time `json:"deleted_at"`
	CreatedAt        time.Time `json:"created_at"`
}
