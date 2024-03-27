package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

type Store struct {
	dbpool *pgxpool.Pool
	schema string
}

func New(ctx context.Context, dbpool *pgxpool.Pool, schema string) (*Store, errors.E) {
	// We create a direct connection ourselves and do not use the pool
	// because current ctx does not have Site or request ID set.
	conn, err := pgx.ConnectConfig(ctx, dbpool.Config().ConnConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schema))
	if err != nil {
		return nil, internal.WithPgxError(err)
	}

	return &Store{
		dbpool: dbpool,
		schema: schema,
	}, nil
}
