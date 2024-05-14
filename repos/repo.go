package repos

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
)

type Repo struct {
	db *pgxpool.Pool
}

var ErrNotFound = pgx.ErrNoRows

func Connect(databaseURL string) (*Repo, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	config.ConnConfig.Tracer = &tracer{}

	db, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}

	return &Repo{db: db}, nil
}

func (r *Repo) Pool() *pgxpool.Pool {
	return r.db
}
