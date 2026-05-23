package store_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func initTestPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())

	// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
	// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
	dbCtx := internalStore.WithMaxDBPoolConnections(context.WithoutCancel(ctx), internalStore.TestMaxDBPoolConnections)
	dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(dbCtx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	t.Cleanup(dbpoolCleanup)

	errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, dbpool
}

func TestInitPostgres(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	// Verify pool is functional by running a simple query.
	var result int
	err := dbpool.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestInitPostgresInvalidURI(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(zerolog.NewTestWriter(t))
	_, _, errE := internalStore.InitPostgres(t.Context(), "not a valid uri %%%", logger, func(context.Context) (string, string) {
		return "test", "test"
	})
	assert.EqualError(t, errE, "cannot parse `not a valid uri %%%`: failed to parse as keyword/value (invalid keyword/value)")
}

func TestEnsureSchemaCreatesSchema(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	schema := "s" + strings.ToLower(identifier.New().String())

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify schema exists.
	var exists bool
	err := dbpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`, schema).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEnsureSchemaIdempotent(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	schema := "s" + strings.ToLower(identifier.New().String())

	// Create schema twice: second call should not error.
	for range 2 {
		errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			return internalStore.EnsureSchema(ctx, tx, schema)
		})
		require.NoError(t, errE, "% -+#.1v", errE)
	}
}

func TestRetryTransactionSuccess(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	called := 0
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		called++
		var result int
		err := tx.QueryRow(ctx, "SELECT 1").Scan(&result)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 1, called)
}

func TestRetryTransactionNonRetryableError(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	expectedErr := errors.New("test error")
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		return expectedErr
	})
	assert.ErrorIs(t, errE, expectedErr)
}

func TestRetryTransactionCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	errE := internalStore.RetryTransaction(cancelledCtx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		return nil
	})
	assert.ErrorIs(t, errE, context.Canceled)
}

func TestRetryTransactionNestedTransaction(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	// Outer transaction.
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, "SELECT 1")
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		// Nested transaction via context.
		return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			var result int
			err := tx.QueryRow(ctx, "SELECT 2").Scan(&result)
			return internalStore.WithPgxError(err)
		})
	})
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestRetryTransactionNestedTransactionError(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	expectedErr := errors.New("nested error")

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, _ pgx.Tx) errors.E {
		// Nested transaction that fails.
		return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(_ context.Context, _ pgx.Tx) errors.E {
			return expectedErr
		})
	})
	assert.ErrorIs(t, errE, expectedErr)
}

func TestRetryTransactionFnCommits(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	// Function that commits the transaction itself.
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.Commit(ctx)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestRetryTransactionFnRollbacks(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	// Function that rolls back the transaction itself.
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.Rollback(ctx)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestRetryTransactionNestedFnCommits(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, _ pgx.Tx) errors.E {
		// Nested transaction where fn commits itself.
		return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			err := tx.Commit(ctx)
			return internalStore.WithPgxError(err)
		})
	})
	require.NoError(t, errE, "% -+#.1v", errE)
}

// createKVTable creates a small key/value table inside its own transaction.
// Used by the nested-transaction tests below to observe row-level effects.
func createKVTable(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, table string) { //nolint:revive
	t.Helper()
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `CREATE TABLE "`+table+`" ("k" text PRIMARY KEY, "v" text NOT NULL)`)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)
}

func countRows(t *testing.T, ctx context.Context, dbpool *pgxpool.Pool, table, key string) int { //nolint:revive
	t.Helper()
	var c int
	err := dbpool.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE "k"=$1`, key).Scan(&c)
	require.NoError(t, err)
	return c
}

// TestNestedTransactionSavepointIsolation: when the nested fn errors, only its
// writes are rolled back (via SAVEPOINT/ROLLBACK TO); the outer transaction
// continues and its writes (both before and after the failed nested block)
// persist.
func TestNestedTransactionSavepointIsolation(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	table := "kv_" + strings.ToLower(identifier.New().String())
	createKVTable(t, ctx, dbpool, table)

	intentional := errors.Base("intentional")

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Outer write A. Pre-nested.
		_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('A','outer-a')`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		// Nested fn writes B and then errors. The savepoint rolls back B.
		nestedErr := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('B','nested-b')`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			return errors.WithStack(intentional)
		})
		require.ErrorIs(t, nestedErr, intentional)

		// Within the same outer transaction, A is still visible and B is gone.
		var aSeen, bSeen int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE "k"='A'`).Scan(&aSeen)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		err = tx.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE "k"='B'`).Scan(&bSeen)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		assert.Equal(t, 1, aSeen, "outer still sees its pre-nested write")
		assert.Equal(t, 0, bSeen, "nested write rolled back by savepoint")

		// Outer continues with another write. Savepoint failure must not poison
		// the outer transaction.
		_, err = tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('C','outer-c')`)
		return internalStore.WithPgxError(err)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	// After commit: A and C persist, B does not.
	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "A"))
	assert.Equal(t, 0, countRows(t, ctx, dbpool, table, "B"))
	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "C"))
}

// TestNestedTransactionCommitVisibleToOuter: when the nested fn returns nil,
// the savepoint is released and the nested writes become part of the outer
// transaction, visible to subsequent outer queries and durable after outer
// commit.
func TestNestedTransactionCommitVisibleToOuter(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	table := "kv_" + strings.ToLower(identifier.New().String())
	createKVTable(t, ctx, dbpool, table)

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('A','outer-a')`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		nestedErr := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('B','nested-b')`)
			return internalStore.WithPgxError(err)
		})
		require.NoError(t, nestedErr, "% -+#.1v", nestedErr)

		// Both visible to outer after savepoint release.
		var aSeen, bSeen int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE "k"='A'`).Scan(&aSeen)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		err = tx.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE "k"='B'`).Scan(&bSeen)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		assert.Equal(t, 1, aSeen)
		assert.Equal(t, 1, bSeen)
		return nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "A"))
	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "B"))
}

// TestNestedTransactionOuterRollbackDiscardsNested: a successful nested
// savepoint release is not durable on its own; if the outer transaction is
// then aborted (by the outer fn returning an error), all writes (both outer
// and nested) are rolled back together.
func TestNestedTransactionOuterRollbackDiscardsNested(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	table := "kv_" + strings.ToLower(identifier.New().String())
	createKVTable(t, ctx, dbpool, table)

	outerErr := errors.Base("outer aborts")

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('A','outer-a')`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		// Nested fn succeeds and releases its savepoint.
		nestedErr := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('B','nested-b')`)
			return internalStore.WithPgxError(err)
		})
		require.NoError(t, nestedErr, "% -+#.1v", nestedErr)

		// Outer aborts.
		return errors.WithStack(outerErr)
	})
	assert.ErrorIs(t, errE, outerErr)

	// Neither row should be present: outer rollback discards the entire tx.
	assert.Equal(t, 0, countRows(t, ctx, dbpool, table, "A"))
	assert.Equal(t, 0, countRows(t, ctx, dbpool, table, "B"))
}

// TestNestedTransactionThreeLevelsSuccess: three levels of nesting succeed and
// all writes persist after the outermost commit.
func TestNestedTransactionThreeLevelsSuccess(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	table := "kv_" + strings.ToLower(identifier.New().String())
	createKVTable(t, ctx, dbpool, table)

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('L0','top')`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('L1','mid')`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
				_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('L2','deep')`)
				return internalStore.WithPgxError(err)
			})
		})
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	for _, k := range []string{"L0", "L1", "L2"} {
		assert.Equal(t, 1, countRows(t, ctx, dbpool, table, k), "row %q persisted", k)
	}
}

// TestNestedTransactionInnermostRollbackPreservesMiddleAndOuter: 3-level
// nesting where the innermost errors. The middle catches and continues with
// another write. The outer also writes. Innermost's write is rolled back via
// its savepoint; middle and outer writes persist.
func TestNestedTransactionInnermostRollbackPreservesMiddleAndOuter(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	table := "kv_" + strings.ToLower(identifier.New().String())
	createKVTable(t, ctx, dbpool, table)

	intentional := errors.Base("innermost fails")

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('outer','o')`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		return internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('middle','m')`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			// Innermost fails after writing.
			innerErr := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
				_, err := tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('inner','i')`)
				if err != nil {
					return internalStore.WithPgxError(err)
				}
				return errors.WithStack(intentional)
			})
			require.ErrorIs(t, innerErr, intentional)

			// Middle writes again after catching the failure.
			_, err = tx.Exec(ctx, `INSERT INTO "`+table+`" VALUES ('middle2','m2')`)
			return internalStore.WithPgxError(err)
		})
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "outer"))
	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "middle"))
	assert.Equal(t, 1, countRows(t, ctx, dbpool, table, "middle2"))
	assert.Equal(t, 0, countRows(t, ctx, dbpool, table, "inner"), "innermost write rolled back by its savepoint")
}

type mockHandler struct {
	ready chan struct{}
}

func (m *mockHandler) HandleNotification(_ context.Context, _ *pgconn.Notification, _ *pgx.Conn) error {
	return nil
}

func (m *mockHandler) HandleBacklog(_ context.Context, _ string, _ *pgx.Conn) error {
	return nil
}

func (m *mockHandler) HandlingReady(_ context.Context, _ string) errors.E {
	<-m.ready
	return nil
}

func TestListenerHandle(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)
	_ = ctx

	listener := internalStore.NewListener(dbpool)
	require.NotNil(t, listener)

	handler := &mockHandler{ready: make(chan struct{})}
	listener.Handle("test_channel", handler)
}

func TestListenerStartWithHandler(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	listener := internalStore.NewListener(dbpool)

	handler := &mockHandler{ready: make(chan struct{})}
	close(handler.ready)
	listener.Handle("test_channel", handler)

	errE := listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestListenerStartAlreadyStarted(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	listener := internalStore.NewListener(dbpool)
	errE := listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Starting again should error.
	errE = listener.Start(ctx)
	assert.EqualError(t, errE, "already started")
}

type safeToRetryError struct {
	msg string
}

func (e *safeToRetryError) Error() string {
	return e.msg
}

func (e *safeToRetryError) SafeToRetry() bool {
	return true
}

func TestRetryTransactionSafeToRetry(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	attempts := 0
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		attempts++
		if attempts < 3 {
			return errors.WithStack(&safeToRetryError{msg: "retry me"})
		}
		return nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 3, attempts)
}

func TestRetryTransactionSerializationFailure(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	attempts := 0
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		attempts++
		if attempts < 2 {
			return errors.WithStack(&pgconn.PgError{ //nolint:exhaustruct
				Code: pgerrcode.SerializationFailure,
			})
		}
		return nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 2, attempts)
}

func TestRetryTransactionDeadlockDetected(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	attempts := 0
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		attempts++
		if attempts < 2 {
			return errors.WithStack(&pgconn.PgError{ //nolint:exhaustruct
				Code: pgerrcode.DeadlockDetected,
			})
		}
		return nil
	})
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, 2, attempts)
}

func TestRetryTransactionMaxRetries(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	attempts := 0
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		attempts++
		return errors.WithStack(&safeToRetryError{msg: "always retry"})
	})
	assert.ErrorIs(t, errE, internalStore.ErrMaxRetriesReached)
	assert.Equal(t, internalStore.MaxRetries, attempts)
}

func TestRetryTransactionDeadlineExceeded(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	deadlineCtx, cancel := context.WithTimeout(ctx, 0)
	defer cancel()

	// Context is already expired.
	errE := internalStore.RetryTransaction(deadlineCtx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		return nil
	})
	assert.ErrorIs(t, errE, context.DeadlineExceeded)
}

func TestRetryTransactionContextCancelledDuringFn(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	cancelCtx, cancel := context.WithCancel(ctx)

	errE := internalStore.RetryTransaction(cancelCtx, dbpool, pgx.ReadOnly, func(_ context.Context, _ pgx.Tx) errors.E {
		cancel()
		return errors.WithStack(context.Canceled)
	})
	assert.ErrorIs(t, errE, context.Canceled)
}

func TestNewRiver(t *testing.T) {
	t.Parallel()

	ctx, dbpool := initTestPool(t)

	schema := "s" + strings.ToLower(identifier.New().String())

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	riverClient, workers, errE := internalStore.NewRiver(ctx, logger, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotNil(t, riverClient)
	require.NotNil(t, workers)
}
