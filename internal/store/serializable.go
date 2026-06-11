package store

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"
)

// MaxRetries is the maximum number of retries for serializable transactions
// and other database retry loops.
const MaxRetries = 10

// retryBackoffBase and retryBackoffMax bound the exponential backoff between transaction retries.
const (
	retryBackoffBase = 5 * time.Millisecond
	retryBackoffMax  = 500 * time.Millisecond
)

var ErrMaxRetriesReached = errors.Base("max retries reached")

// TODO: For cases where only one query is made inside a transaction, we could make a query single-trip by making transaction and committing it ourselves.

func nestedTransaction(ctx context.Context, parentTx pgx.Tx, fn func(ctx context.Context, tx pgx.Tx) errors.E) (errE errors.E) { //nolint:nonamedreturns
	tx, err := parentTx.Begin(ctx)
	if err != nil {
		return WithPgxError(err)
	}
	defer func() {
		err = tx.Rollback(ctx)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			errE = errors.Join(errE, WithPgxError(err))
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

// retryBackoff sleeps before the next transaction attempt when i attempts have already failed,
// using exponential backoff with full jitter, respecting context cancellation. The first retry is
// immediate because a transient conflict often resolves right away, later retries back off so that
// under sustained contention attempts do not fail back-to-back and exhaust MaxRetries in a burst.
func retryBackoff(ctx context.Context, i int) errors.E {
	if i == 0 {
		return nil
	}
	// Jitter does not need cryptographic randomness.
	timer := time.NewTimer(rand.N(min(retryBackoffBase<<i, retryBackoffMax))) //nolint:gosec
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	case <-timer.C:
		return nil
	}
}

// RetryTransaction executes a database transaction at SERIALIZABLE isolation with automatic retry
// logic for serialization failures.
func RetryTransaction(
	ctx context.Context, dbpool *pgxpool.Pool, accessMode pgx.TxAccessMode,
	fn func(ctx context.Context, tx pgx.Tx) errors.E,
) errors.E {
	return RetryTransactionWithIsoLevel(ctx, dbpool, pgx.Serializable, accessMode, fn)
}

// RetryTransactionWithIsoLevel is like RetryTransaction but at an explicit isolation level. Most transactions
// should use RetryTransaction. A weaker level is appropriate only for read-only queries which tolerate
// observing a snapshot that is consistent but possibly not serializable with concurrent transactions, where
// SERIALIZABLE would make the query both fail spuriously and force concurrent writers to retry. The level is
// always set explicitly so that semantics do not depend on the server's default_transaction_isolation setting.
//
// If ctx already carries a transaction, fn runs inside that transaction, at its isolation level.
func RetryTransactionWithIsoLevel(
	ctx context.Context, dbpool *pgxpool.Pool, isoLevel pgx.TxIsoLevel, accessMode pgx.TxAccessMode,
	fn func(ctx context.Context, tx pgx.Tx) errors.E,
) errors.E {
	parentTx, ok := ctx.Value(transactionContextKey).(pgx.Tx)
	if ok {
		return nestedTransaction(ctx, parentTx, fn)
	}

	metrics, _ := waf.GetMetrics(ctx)
	counter := metrics.Counter(MetricDatabaseRetries)

	// We make i match the counter. That means that when loop
	// reaches MaxRetries, counter equals MaxRetries, too.
	for i := 0; i < MaxRetries; i, _ = i+1, counter.Inc() {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		errE := (func() (errE errors.E) { //nolint:nonamedreturns
			tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{
				IsoLevel:       isoLevel,
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
					errE = errors.Join(errE, WithPgxError(err))
				}
			}()

			errE = fn(context.WithValue(ctx, transactionContextKey, tx), tx)
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
			safeToRetry, ok := errors.AsType[interface {
				SafeToRetry() bool
				Error() string
			}](errE)
			if ok && safeToRetry.SafeToRetry() {
				errB := retryBackoff(ctx, i)
				if errB != nil {
					return errB
				}
				continue
			}
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				// See: https://www.postgresql.org/docs/current/mvcc-serialization-failure-handling.html
				switch pgError.Code {
				case pgerrcode.SerializationFailure, pgerrcode.DeadlockDetected:
					errB := retryBackoff(ctx, i)
					if errB != nil {
						return errB
					}
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
