package postgresql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	DB *sqlx.DB
}

func New(URL string) (*sqlx.DB, func(), error) {
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
