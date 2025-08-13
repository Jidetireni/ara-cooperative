package postgresql

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Jidetireni/ara-cooperative.git/internal/constants"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	DB         *sqlx.DB
	SqlBuilder sq.StatementBuilderType
}

func New(URL string) (*PostgresDB, func(), error) {
	db, cleanup, err := initDB(URL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pgDB := &PostgresDB{
		DB:         db,
		SqlBuilder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}

	pgDB.upsertRoles()

	return pgDB, cleanup, nil
}

func initDB(URL string) (*sqlx.DB, func(), error) {
	db, err := sqlx.Open("postgres", URL)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = db.Close()
	}
	db.Mapper = reflectx.NewMapper("json")

	return db, cleanup, nil
}

func (p *PostgresDB) upsertRoles() {
	fmt.Println("Upserting roles...")
	if len(constants.Roles) == 0 {
		log.Println("No roles to upsert. Check constants.Roles initialization.")
		return
	}

	for _, role := range constants.Roles {
		builder := p.SqlBuilder.Insert("roles").
			Columns("permission", "description").
			Values(role.Permission, role.Description).
			Suffix("ON CONFLICT (permission) DO UPDATE SET description = EXCLUDED.description")

		query, args, err := builder.ToSql()
		if err != nil {
			log.Fatalf("Failed to build upsert query: %v\n", err)
		}

		_, err = p.DB.ExecContext(context.Background(), query, args...)
		if err != nil {
			log.Fatalf("Failed to upsert roles: %v\n", err)
		}
		log.Printf("Role %s upserted successfully.\n", role.Permission)
	}
	fmt.Println("Roles upserted successfully.")
}
