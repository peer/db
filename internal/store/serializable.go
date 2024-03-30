package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
)

const maxRetries = 10

var ErrMaxRetriesReached = errors.Base("max retries reached")

// TODO: For cases where only one query is made inside a transaction, we could make a query single-trip by making transaction and committing it ourselves.

func RetryTransaction(ctx context.Context, dbpool *pgxpool.Pool, accessMode pgx.TxAccessMode, fn func(ctx context.Context, tx pgx.Tx) errors.E) errors.E {
	for i := 0; i < maxRetries; i++ {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		errE := (func() (errE errors.E) { //nolint:nonamedreturns
			tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{
				IsoLevel:       pgx.Serializable,
				AccessMode:     accessMode,
				DeferrableMode: pgx.NotDeferrable,
				BeginQuery:     "",
			})
			if err != nil {
				return WithPgxError(err)
			}
			defer func() {
				err = tx.Rollback(ctx)
				if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					errE = errors.Join(errE, err)
				}
			}()

			errE = fn(ctx, tx)
			if errE != nil {
				return errE
			}

			err = tx.Commit(ctx)
			if err != nil && (errors.Is(err, pgx.ErrTxClosed) || errors.Is(err, pgx.ErrTxCommitRollback)) {
				// We allow for fn to commit or rollback already.
				return nil
			}
			return WithPgxError(err)
		})()

		if errE != nil {
			if errors.Is(errE, context.Canceled) || errors.Is(errE, context.DeadlineExceeded) {
				return errE
			}
			var safeToRetry interface{ SafeToRetry() bool }
			if errors.As(errE, &safeToRetry) && safeToRetry.SafeToRetry() {
				continue
			}
			var pgError *pgconn.PgError
			if errors.As(errE, &pgError) {
				// See: https://www.postgresql.org/docs/current/mvcc-serialization-failure-handling.html
				switch pgError.Code {
				// serialization_failure.
				case "40001":
					continue
				// deadlock_detected.
				case "40P01":
					continue
				}
			}
			// A non-retryable error.
			return errE
		}

		// No error.
		return nil
	}

	return errors.WithStack(ErrMaxRetriesReached)
}
