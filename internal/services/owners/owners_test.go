package owners_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/services/owners"
)

func TestOwnerBootstrapRejectsInvalidEmail(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, email string }{
		{name: "missing", email: ""},
		{name: "malformed", email: "not-an-email"},
		{name: "display name", email: "Owner <owner@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := owners.New(nil).Bootstrap(context.Background(), tt.email); err == nil {
				t.Fatal("invalid email reached owner creation")
			}
		})
	}
}

func TestOwnerBootstrapRejectsInvalidPassword(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, password string }{
		{name: "missing", password: ""},
		{name: "too short", password: "short"},
		{name: "over bcrypt limit", password: strings.Repeat("a", 73)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := owners.New(nil).BootstrapWithPassword(context.Background(), "owner@example.com", tt.password); err == nil {
				t.Fatal("invalid password reached owner creation")
			}
		})
	}
}
