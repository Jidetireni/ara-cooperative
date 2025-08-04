package postgresql

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	DB *sqlx.DB
}

func New(URL string) *PostgresDB {
	db := &PostgresDB{}
	if err := db.initialize(URL); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	return db
}

func (p *PostgresDB) initialize(URL string) error {
	db, err := sqlx.Open("postgres", URL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	p.DB = db
	log.Println("PostgreSQL database connected successfully")

	return nil
}
