package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
)

type Store struct {
	dbpool *pgxpool.Pool
	schema string
}

func New(ctx context.Context, dbpool *pgxpool.Pool, schema string) (*Store, errors.E) {
	return &Store{
		dbpool: dbpool,
		schema: schema,
	}, nil
}
