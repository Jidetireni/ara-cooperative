package repository

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type TokenRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

// NewTokenRepository is the constructor for TokenRepository.
func NewTokenRepository(db *sqlx.DB) *TokenRepository {
	return &TokenRepository{
		db:   db,
		psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

// TokenRepositoryFilter is used to define common query filters for tokens.
type TokenRepositoryFilter struct {
	TokenID   *uuid.UUID
	UserID    *uuid.UUID
	Token     *string
	TokenType *string
	IsValid   *bool
	IsExpired *bool // Use true to filter for expired tokens
	IsDeleted *bool
}

// buildQuery builds a squirrel query based on the provided filter and query type.
// This generic function encapsulates all common filtering and query building logic for SELECT and COUNT queries.
func (tr *TokenRepository) buildQuery(filter TokenRepositoryFilter, queryType QueryType) (sq.SelectBuilder, error) {
	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = tr.psql.Select("*").From("tokens")
	case QueryTypeCount:
		builder = tr.psql.Select("COUNT(*)").From("tokens")
	default:
		return sq.SelectBuilder{}, errors.New("invalid query type provided")
	}

	if filter.TokenID != nil {
		builder = builder.Where(sq.Eq{"id": *filter.TokenID})
	}
	if filter.UserID != nil {
		builder = builder.Where(sq.Eq{"user_id": *filter.UserID})
	}
	if filter.Token != nil {
		builder = builder.Where(sq.Eq{"token": *filter.Token})
	}
	if filter.TokenType != nil {
		builder = builder.Where(sq.Eq{"token_type": *filter.TokenType})
	}
	if filter.IsValid != nil {
		builder = builder.Where(sq.Eq{"is_valid": *filter.IsValid})
	}
	if filter.IsExpired != nil {
		if *filter.IsExpired {
			builder = builder.Where(sq.Lt{"expires_at": time.Now()})
		} else {
			builder = builder.Where(sq.GtOrEq{"expires_at": time.Now()})
		}
	}
	if filter.IsDeleted != nil {
		if *filter.IsDeleted {
			builder = builder.Where(sq.NotEq{"deleted_at": nil})
		} else {
			builder = builder.Where(sq.Eq{"deleted_at": nil})
		}
	}

	return builder, nil
}

func (tr *TokenRepository) Update(ctx context.Context, token *Token, tx *sqlx.Tx) error {
	builder := tr.psql.Update("tokens").
		Set("token_type", token.TokenType).
		Set("is_valid", token.IsValid).
		Set("updated_at", time.Now())

	if token.DeletedAt.Valid {
		builder = builder.Set("deleted_at", token.DeletedAt)
	}

	if token.ID != uuid.Nil {
		builder = builder.Where(sq.Eq{"id": token.ID})
	}

	if token.Token != "" {
		builder = builder.Where(sq.Eq{"token": token.Token})
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return err
	}

	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
		return err
	}

	_, err = tr.db.ExecContext(ctx, query, args...)
	return err
}

// Create invalidates existing refresh tokens for a user and stores a new one.
func (tr *TokenRepository) Create(ctx context.Context, token Token, tx *sqlx.Tx) (*Token, error) {
	// Insert new refresh token.
	builder := tr.psql.Insert("tokens").
		Columns("id", "user_id", "token", "token_type", "is_valid", "expires_at", "created_at", "updated_at").
		Values(token.ID, token.UserID, token.Token, token.TokenType, token.IsValid, token.ExpiresAt,
			time.Now(), time.Now()).
		Suffix("ON CONFLICT (user_id, token_type) DO UPDATE SET token = EXCLUDED.token, is_valid = EXCLUDED.is_valid, expires_at = EXCLUDED.expires_at, updated_at = EXCLUDED.updated_at, deleted_at = NULL RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var newToken Token
	if tx != nil {
		err = tx.GetContext(ctx, &newToken, query, args...)
		return &newToken, err
	}

	err = tr.db.GetContext(ctx, &newToken, query, args...)
	return &newToken, err
}

// ValidateRefreshToken checks if a token is valid, not expired, and not deleted.
func (tr *TokenRepository) ValidateRefreshToken(ctx context.Context, filter TokenRepositoryFilter) (bool, error) {
	// Use buildQuery with QueryTypeCount.
	builder, err := tr.buildQuery(filter, QueryTypeCount)
	if err != nil {
		return false, err
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return false, err
	}
	var count int
	err = tr.db.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetTokenByID fetches a single token by its ID.
func (tr *TokenRepository) GetTokenByID(ctx context.Context, filter TokenRepositoryFilter) (*Token, error) {
	// Use buildQuery with QueryTypeSelect.
	builder, err := tr.buildQuery(filter, QueryTypeSelect)
	if err != nil {
		return nil, err
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var token Token
	err = tr.db.GetContext(ctx, &token, query, args...)
	if err != nil {
		return nil, err
	}

	return &token, nil
}
