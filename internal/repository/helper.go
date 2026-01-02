package repository

import (
	"database/sql"
	"encoding/base64"
	"fmt"
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

func parseSort(sort *string) (SortResult, error) {
	if sort == nil {
		return SortResult{
			Column: "created_at",
			Order:  SortOrderDesc,
		}, nil
	}

	//
	parts := strings.Split(*sort, ":")
	if len(parts) != 2 {
		return SortResult{}, fmt.Errorf("invalid sort format")
	}
	column := parts[0]
	Order := parts[1]

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

	return SortResult{}, fmt.Errorf("invalid sort order: %s", Order)
}

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
			return builder, err
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
