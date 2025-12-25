package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	DB         *sqlx.DB
	SqlBuilder sq.StatementBuilderType
}

func New(URL string, dbType string) (*PostgresDB, func(), error) {
	db, cleanup, err := initDB(URL, dbType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pgDB := &PostgresDB{
		DB:         db,
		SqlBuilder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}

	pgDB.upsertRoles()
	pgDB.upsertPermissions()

	return pgDB, cleanup, nil
}

func initDB(URL string, dbType string) (*sqlx.DB, func(), error) {
	db, err := sqlx.Open(dbType, URL)
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

func (p *PostgresDB) upsertPermissions() {
	fmt.Println("Upserting permissions...")
	if len(constants.Permissions) == 0 {
		log.Println("No permissions to upsert. Check constants.Permissions initialization.")
		return
	}

	for _, role := range constants.Permissions {
		builder := p.SqlBuilder.Insert("permissions").
			Columns("slug", "description").
			Values(role.Slug, role.Description).
			Suffix("ON CONFLICT (slug) DO UPDATE SET description = EXCLUDED.description")

		query, args, err := builder.ToSql()
		if err != nil {
			log.Fatalf("Failed to build upsert query: %v\n", err)
		}

		_, err = p.DB.ExecContext(context.Background(), query, args...)
		if err != nil {
			log.Fatalf("Failed to upsert permissions: %v\n", err)
		}
		log.Printf("Permission %s upserted successfully.\n", role.Slug)
	}
	fmt.Println("Permissions upserted successfully.")
}

func (p *PostgresDB) upsertRoles() {
	fmt.Println("Upserting roles...")
	roles := []struct {
		Name string
	}{
		{Name: string(constants.RoleAdmin)},
		{Name: string(constants.RoleMember)},
	}

	for _, role := range roles {
		builder := p.SqlBuilder.Insert("roles").
			Columns("name").
			Values(role.Name).
			Suffix("ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name")

		query, args, err := builder.ToSql()
		if err != nil {
			log.Fatalf("Failed to build upsert query: %v\n", err)
		}

		_, err = p.DB.ExecContext(context.Background(), query, args...)
		if err != nil {
			log.Fatalf("Failed to upsert roles: %v\n", err)
		}
		log.Printf("Role %s upserted successfully.\n", role.Name)
	}
	fmt.Println("Roles upserted successfully.")
}
