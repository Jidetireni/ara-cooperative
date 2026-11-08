package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var ErrOwnerExists = errors.New("a different owner is already configured")

type OwnerRepository struct {
	db *sqlx.DB
}

type OwnerBootstrapParams struct {
	Email          string
	SetupHash      string
	PasswordHash   string
	SetupExpiresAt time.Time
}

func NewOwnerRepository(db *sqlx.DB) *OwnerRepository {
	return &OwnerRepository{db: db}
}

// Bootstrap serializes owner creation and preserves existing credentials and memberships.
func (r *OwnerRepository) Bootstrap(
	ctx context.Context,
	input OwnerBootstrapParams,
) (owner *User, needsSetup bool, err error) {
	email := input.Email
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin owner bootstrap: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback owner bootstrap: %w", rollbackErr))
		}
	}()
	if _, err := tx.ExecContext(ctx,
		"SELECT pg_advisory_xact_lock(hashtext(current_schema()), hashtext('owner-bootstrap'))",
	); err != nil {
		return nil, false, fmt.Errorf("lock owner bootstrap: %w", err)
	}

	owners := []User{}
	if err := tx.SelectContext(ctx, &owners, `SELECT u.* FROM users u
		JOIN user_roles ur ON ur.user_id = u.id JOIN roles r ON r.id = ur.role_id
		WHERE r.name = 'owner'`); err != nil {
		return nil, false, fmt.Errorf("load existing owner: %w", err)
	}
	if len(owners) > 1 {
		return nil, false, errors.New("multiple owners exist; repair owner assignments before bootstrapping")
	}
	if len(owners) == 1 && !strings.EqualFold(owners[0].Email, email) {
		return nil, false, ErrOwnerExists
	}
	if len(owners) == 1 && owners[0].DeletedAt.Valid {
		return nil, false, errors.New("the owner account is deleted; restore it before bootstrapping")
	}

	matches := []User{}
	if err := tx.SelectContext(ctx, &matches,
		"SELECT * FROM users WHERE lower(email) = lower($1)", email,
	); err != nil {
		return nil, false, fmt.Errorf("find owner account: %w", err)
	}
	if len(matches) > 1 {
		return nil, false, errors.New("multiple accounts match the owner email")
	}
	var user *User
	if len(matches) == 1 {
		user = &matches[0]
		if user.DeletedAt.Valid {
			return nil, false, errors.New("the requested owner account is deleted")
		}
	} else {
		user, err = NewUserRepository(r.db).Create(ctx, &User{Email: email}, tx)
		if err != nil {
			return nil, false, fmt.Errorf("create owner account: %w", err)
		}
	}

	var role Role
	if err := tx.GetContext(ctx, &role, `INSERT INTO roles (name) VALUES ('owner')
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING *`); err != nil {
		return nil, false, fmt.Errorf("ensure owner role: %w", err)
	}
	if err := NewRoleRepository(r.db).AssignToUser(ctx, &user.ID, []uuid.UUID{role.ID}, tx); err != nil {
		return nil, false, fmt.Errorf("assign owner role: %w", err)
	}
	needsSetup = !user.PasswordHash.Valid || user.PasswordHash.String == ""
	if needsSetup && input.PasswordHash != "" {
		if err := tx.GetContext(ctx, user, `UPDATE users SET password_hash = $1,
			email_confirmed_at = COALESCE(email_confirmed_at, NOW()), updated_at = NOW()
			WHERE id = $2 RETURNING *`, input.PasswordHash, user.ID); err != nil {
			return nil, false, fmt.Errorf("set initial owner password: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tokens SET is_valid = FALSE, updated_at = NOW()
			WHERE user_id = $1 AND token_type IN ('set_password_token', 'refresh_token')`, user.ID); err != nil {
			return nil, false, fmt.Errorf("invalidate owner setup credentials: %w", err)
		}
		needsSetup = false
	}
	if needsSetup {
		if _, err := NewTokenRepository(r.db).Create(ctx, &Token{
			UserID: user.ID, Token: input.SetupHash, TokenType: "set_password_token",
			IsValid: true, ExpiresAt: input.SetupExpiresAt,
		}, tx); err != nil {
			return nil, false, fmt.Errorf("create owner password setup: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit owner bootstrap: %w", err)
	}
	return user, needsSetup, nil
}
