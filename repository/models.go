package repository

import "github.com/google/uuid"

type QueryType string

const (
	QueryTypeSelect QueryType = "select"
	QueryTypeCount  QueryType = "count"
)

type User struct {
	ID           *uuid.UUID `json:"id"`
	Email        string     `json:"email"`
	HashPassword string     `json:"hash_password"`
	IsValid      bool       `json:"is_valid"`
	CreatedAt    string     `json:"created_at"`
}
