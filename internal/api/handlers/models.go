package handlers

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID      `json:"id"`
	Email            string         `json:"email"`
	FirstName        string         `json:"first_name"`
	LastName         string         `json:"last_name"`
	Phone            string         `json:"phone"`
	Address          sql.NullString `json:"address"`
	EmailConfirmedAt sql.NullTime   `json:"email_confirmed_at"`
	PasswordHash     sql.NullString `json:"password_hash"`
	NextOfKinName    sql.NullString `json:"next_of_kin_name"`
	NextOfKinPhone   sql.NullString `json:"next_of_kin_phone"`
	UpdatedAt        sql.NullTime   `json:"updated_at"`
	DeletedAt        sql.NullTime   `json:"deleted_at"`
	CreatedAt        time.Time      `json:"created_at"`
}
