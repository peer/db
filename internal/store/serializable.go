package store

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
)

const maxRetries = 10

var ErrMaxRetriesReached = errors.Base("max retries reached")

// TODO: For cases where only one query is made inside a transaction, we could make a query single-trip by making transaction and committing it ourselves.

// See: https://github.com/jackc/pgx/issues/2001
type dbTx struct {
	Tx        pgx.Tx
	Callbacks []func()
}

func nestedTransaction(ctx context.Context, parentTx pgx.Tx, fn func(ctx context.Context, tx pgx.Tx) errors.E) (errE errors.E) { //nolint:nonamedreturns
	tx, err := parentTx.Begin(ctx)
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
}

// RetryTransaction executes a database transaction with automatic retry logic for serialization failures.
func RetryTransaction(
	ctx context.Context, dbpool *pgxpool.Pool, accessMode pgx.TxAccessMode,
	fn func(ctx context.Context, tx pgx.Tx) errors.E,
	afterCommitFn func(),
) errors.E {
	parentTx, ok := ctx.Value(transactionContextKey).(*dbTx)
	if ok {
		if afterCommitFn != nil {
			parentTx.Callbacks = append(parentTx.Callbacks, afterCommitFn)
		}
		return nestedTransaction(ctx, parentTx.Tx, fn)
	}

	metrics, _ := waf.GetMetrics(ctx)
	counter := metrics.Counter(MetricDatabaseRetries)

	// We make i match the counter. That means that when loop
	// reaches maxRetries, counter equals maxRetries, too.
	for i := 0; i < maxRetries; i, _ = i+1, counter.Inc() {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		var callbacks []func()

		errE := (func() (errE errors.E) { //nolint:nonamedreturns
			tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{
				IsoLevel:       pgx.Serializable,
				AccessMode:     accessMode,
				DeferrableMode: pgx.NotDeferrable,
				BeginQuery:     "",
				CommitQuery:    "",
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

			parentTx := &dbTx{
				Tx:        tx,
				Callbacks: nil,
			}

			errE = fn(context.WithValue(ctx, transactionContextKey, parentTx), tx)
			if errE != nil {
				return errE
			}

			callbacks = parentTx.Callbacks

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
				case ErrorCodeSerializationFailure:
					continue
				case ErrorCodeDeadlockDetected:
					continue
				}
			}
			// A non-retryable error.
			return errE
		}

		if afterCommitFn != nil {
			callbacks = append(callbacks, afterCommitFn)
		}
		slices.Reverse(callbacks)
		for _, fn := range callbacks {
			fn()
		}

		// No error.
		return nil
	}

	return errors.WithStack(ErrMaxRetriesReached)
}
