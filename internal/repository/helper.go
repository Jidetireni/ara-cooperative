package repository

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type QueryType string

type SortOrder string

const (
	QueryTypeSelect QueryType = "select"
	QueryTypeCount  QueryType = "count"

	SortOrderAsc  SortOrder = "ASC"
	SortOrderDesc SortOrder = "DESC"
)

type QueryOptions struct {
	Limit  uint32
	Cursor *string
	Sort   *string
	Type   *QueryType
}

type SortResult struct {
	Column string
	Order  SortOrder
}

// ErrInvalidSort marks a client sort value that lists cannot use.
var ErrInvalidSort = errors.New("invalid sort")

// sortableColumn is the only column lists can be ordered by: the pagination
// cursor encodes it, so any other order would break paging.
const sortableColumn = "created_at"

// sortColumnPattern admits only plain or table-qualified identifiers, so a sort
// value can never inject SQL into ORDER BY or the cursor condition.
var sortColumnPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*(\.[a-z_][a-z0-9_]*)?$`)

// SortFor turns a client sort ("created_at:asc" or "created_at:desc"; nil means
// newest first) into the qualified column for the query's main table alias.
// The column name always comes from the repository, never from the client.
func SortFor(sort *string, alias string) (*string, error) {
	order := "desc"
	if sort != nil {
		column, direction, ok := strings.Cut(strings.ToLower(strings.TrimSpace(*sort)), ":")
		if !ok || column != sortableColumn || (direction != "asc" && direction != "desc") {
			return nil, fmt.Errorf("%w: use created_at:asc or created_at:desc", ErrInvalidSort)
		}
		order = direction
	}
	qualified := fmt.Sprintf("%s.%s:%s", alias, sortableColumn, order)
	return &qualified, nil
}

func parseSort(sort *string) (SortResult, error) {
	if sort == nil {
		return SortResult{
			Column: "created_at",
			Order:  SortOrderDesc,
		}, nil
	}

	parts := strings.Split(*sort, ":")
	if len(parts) != 2 {
		return SortResult{}, fmt.Errorf("%w: invalid sort format", ErrInvalidSort)
	}
	column := parts[0]
	Order := parts[1]
	if !sortColumnPattern.MatchString(column) {
		return SortResult{}, fmt.Errorf("%w: invalid sort column", ErrInvalidSort)
	}

	switch Order {
	case "asc":
		return SortResult{
			Column: column,
			Order:  SortOrderAsc,
		}, nil
	case "desc":
		return SortResult{
			Column: column,
			Order:  SortOrderDesc,
		}, nil
	}

	return SortResult{}, fmt.Errorf("%w: invalid sort order: %s", ErrInvalidSort, Order)
}

// ErrInvalidCursor marks a pagination cursor the client sent that cannot be
// decoded, so services can answer 400 instead of 500.
var ErrInvalidCursor = errors.New("invalid cursor")

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}

	timeV, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid created_at in cursor: %w", err)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid id in cursor: %w", err)
	}

	return timeV, id, nil
}

func EncodeCursor(timev time.Time, id uuid.UUID) string {
	cursorStr := fmt.Sprintf("%s|%s", timev.Format(time.RFC3339Nano), id.String())
	return base64.StdEncoding.EncodeToString([]byte(cursorStr))
}

func ApplyPagination(builder sq.SelectBuilder, opts QueryOptions) (sq.SelectBuilder, error) {
	sortResult, err := parseSort(opts.Sort)
	if err != nil {
		return builder, err
	}

	idColumn := "id"
	if strings.Contains(sortResult.Column, ".") {
		table := strings.Split(sortResult.Column, ".")[0]
		idColumn = fmt.Sprintf("%s.id", table)
	}

	if opts.Cursor != nil {
		cursorTime, cursorID, err := decodeCursor(*opts.Cursor)
		if err != nil {
			return builder, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
		}

		switch sortResult.Order {
		case SortOrderAsc:
			builder = builder.Where(sq.Or{
				sq.Gt{sortResult.Column: cursorTime},
				sq.And{
					sq.Eq{sortResult.Column: cursorTime},
					sq.GtOrEq{idColumn: cursorID},
				},
			})
		case SortOrderDesc:
			builder = builder.Where(sq.Or{
				sq.Lt{sortResult.Column: cursorTime},
				sq.And{
					sq.Eq{sortResult.Column: cursorTime},
					sq.LtOrEq{idColumn: cursorID},
				},
			})
		}

	}

	builder = builder.OrderBy(fmt.Sprintf("%s %s, %s %s", sortResult.Column, string(sortResult.Order), idColumn, string(sortResult.Order)))
	builder = builder.Limit(uint64(min(opts.Limit, 100) + 1))
	return builder, nil
}

type ListResult[T any] struct {
	Items      []*T
	NextCursor *string
}

func ToNullUUID(id uuid.UUID) uuid.NullUUID {
	if id == uuid.Nil {
		return uuid.NullUUID{UUID: uuid.Nil, Valid: false}
	}

	return uuid.NullUUID{UUID: id, Valid: true}
}

func ToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}

	return sql.NullTime{Time: *t, Valid: true}
}

func ToNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{Valid: false}
	}

	return sql.NullString{String: *s, Valid: true}
}

func ToNullInt64(n *int64) sql.NullInt64 {
	if n == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *n, Valid: true}
}
