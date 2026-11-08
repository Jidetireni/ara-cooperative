package repository

import (
	"errors"
	"strings"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func TestSortFor(t *testing.T) {
	tests := []struct {
		name  string
		sort  *string
		want  string
		error bool
	}{
		{name: "default newest first", sort: nil, want: "f.created_at:desc"},
		{name: "ascending", sort: lo.ToPtr("created_at:asc"), want: "f.created_at:asc"},
		{name: "case and spaces", sort: lo.ToPtr(" Created_At:DESC "), want: "f.created_at:desc"},
		{name: "client alias rejected", sort: lo.ToPtr("f.created_at:asc"), error: true},
		{name: "other column", sort: lo.ToPtr("amount:asc"), error: true},
		{name: "bad direction", sort: lo.ToPtr("created_at:sideways"), error: true},
		{name: "no direction", sort: lo.ToPtr("created_at"), error: true},
		{name: "injection", sort: lo.ToPtr("created_at; DROP TABLE fines--:asc"), error: true},
		{name: "empty", sort: lo.ToPtr(""), error: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SortFor(tt.sort, "f")
			if tt.error {
				if !errors.Is(err, ErrInvalidSort) || got != nil {
					t.Fatalf("SortFor(%v) = %v, %v; want ErrInvalidSort", lo.FromPtr(tt.sort), got, err)
				}
				return
			}
			if err != nil || got == nil || *got != tt.want {
				t.Fatalf("SortFor(%v) = %v, %v; want %s", lo.FromPtr(tt.sort), got, err, tt.want)
			}
		})
	}
}

func TestApplyPaginationRejectsUnsafeInput(t *testing.T) {
	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("*").From("fines f")
	for _, sort := range []string{
		"id;drop table fines:asc",
		"created_at desc, (select 1):asc",
		"f.created_at::text:asc",
		"1:asc",
	} {
		if _, err := ApplyPagination(builder, QueryOptions{Limit: 10, Sort: lo.ToPtr(sort)}); !errors.Is(err, ErrInvalidSort) {
			t.Fatalf("sort %q accepted: %v", sort, err)
		}
	}

	if _, err := ApplyPagination(builder, QueryOptions{Limit: 10, Cursor: lo.ToPtr("not-a-cursor")}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("malformed cursor not reported as ErrInvalidCursor: %v", err)
	}

	cursor := EncodeCursor(time.Now(), uuid.New())
	query, args, err := lo.Must(ApplyPagination(builder, QueryOptions{
		Limit: 10, Sort: lo.ToPtr("f.created_at:asc"), Cursor: &cursor,
	})).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "ORDER BY f.created_at ASC, f.id ASC") || len(args) != 3 {
		t.Fatalf("unexpected paginated query %q with %d args", query, len(args))
	}
}
