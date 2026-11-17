package members

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
)

// SetExistingMember changes the fee exemption without revoking active membership.
// A pending member with a saved application activates immediately when enabled.
func (m *Member) SetExistingMember(ctx context.Context, slug string, enabled bool) (result *dto.Member, err error) {
	if err := requireAdminAccess(ctx); err != nil {
		return nil, err
	}
	tx, err := m.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin existing member update: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback existing member update: %w", rollbackErr))
		}
	}()
	slug = strings.TrimSpace(slug)
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{Slug: &slug, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load existing member: %w", err)
	}
	result, err = m.profile(ctx, member, tx)
	if err != nil {
		return nil, err
	}
	member.IsExistingMember = enabled
	if enabled && !member.ActivatedAt.Valid && applicationComplete(result) {
		member.ActivatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	updated, err := m.MemberRepository.Update(ctx, member, tx)
	if err != nil {
		return nil, fmt.Errorf("update existing member: %w", err)
	}
	result, err = m.profile(ctx, updated, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit existing member update: %w", err)
	}
	return result, nil
}
