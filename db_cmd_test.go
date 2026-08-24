package peerdb_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
)

func TestClearDirContents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Populate the directory with a top-level file and a nested subdirectory with a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assemble-abc.tmp"), []byte("x"), 0o644)) //nolint:gosec
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))                         //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "hash"), []byte("y"), 0o644))   //nolint:gosec

	errE := peerdb.TestingClearDirContents(dir)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The directory itself remains in place, but is now empty.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// A missing directory is treated as already empty.
	errE = peerdb.TestingClearDirContents(filepath.Join(dir, "does-not-exist"))
	assert.NoError(t, errE, "% -+#.1v", errE)
}

// TestVacuumSchema tests that vacuumSchema vacuums and analyzes every table of the schema it is given
// and no table outside it, and that the statement timeout it lifts for the vacuum does not stay lifted
// on the pooled connection afterwards.
//
// It does not prove the lift is what carries a vacuum past the timeout: that would need a table whose
// vacuum takes longer than the pool's whole timeout, which is too large to build here. The first step
// below instead records why the lift exists at all, by showing that the timeout does apply to a VACUUM.
func TestVacuumSchema(t *testing.T) {
	t.Parallel()

	infra := testutils.NewPostgres(t)
	ctx, dbpool := infra.Ctx, infra.DBPool

	// The schema's tables, each with as many dead tuples as live ones so a vacuum has work to do.
	tables := []string{"vacuumone", "vacuumtwo"}
	for _, table := range tables {
		_, err := dbpool.Exec(ctx, fmt.Sprintf(`CREATE TABLE "%s"."%s" AS
			SELECT g AS i, repeat('x', 100)::text AS payload FROM generate_series(1, 200000) g`, infra.Name, table))
		require.NoError(t, err)
		_, err = dbpool.Exec(ctx, fmt.Sprintf(`UPDATE "%s"."%s" SET payload = payload || 'y'`, infra.Name, table))
		require.NoError(t, err)
	}

	// A table in another schema, which enumerating the schema's tables (rather than running a bare
	// VACUUM) is what keeps untouched.
	otherSchema := infra.Name + "other"
	_, err := dbpool.Exec(ctx, `CREATE SCHEMA "`+otherSchema+`"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		// We do not use the test context because we want an active context, not a canceled one.
		_, err := dbpool.Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+otherSchema+`" CASCADE`)
		require.NoError(t, err)
	})
	_, err = dbpool.Exec(ctx, fmt.Sprintf(`CREATE TABLE "%s"."other" AS
		SELECT g AS i FROM generate_series(1, 1000) g`, otherSchema))
	require.NoError(t, err)

	var timeoutBefore string
	require.NoError(t, dbpool.QueryRow(ctx, `SHOW statement_timeout`).Scan(&timeoutBefore))
	require.NotEqual(t, "0", timeoutBefore, "the pool should set a statement timeout for the vacuum to have to lift")

	// The statement timeout the pool sets applies to a VACUUM like to any other statement, which is what
	// vacuumSchema has to lift. The timeout is set on the session because SET LOCAL has no effect outside
	// a transaction block and a VACUUM cannot run inside one, and it is reset before the connection is
	// released so that this step does not leave behind the very leak the last step checks for.
	func() {
		conn, err := dbpool.Acquire(ctx)
		require.NoError(t, err)
		defer conn.Release()

		_, err = conn.Exec(ctx, `SET statement_timeout = 1`)
		require.NoError(t, err)
		defer func() {
			_, err := conn.Exec(ctx, `RESET statement_timeout`)
			require.NoError(t, err)
		}()

		_, err = conn.Exec(ctx, fmt.Sprintf(`VACUUM (ANALYZE) "%s"."%s"`, infra.Name, tables[0]))
		var pgError *pgconn.PgError
		require.ErrorAs(t, err, &pgError)
		assert.Equal(t, pgerrcode.QueryCanceled, pgError.Code)
	}()

	errE := peerdb.TestingVacuumSchema(ctx, dbpool, infra.Name)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The statistics are reported by the backend which did the vacuum, so they can lag the call.
	for _, table := range tables {
		require.Eventually(t, func() bool {
			var vacuumed, analyzed bool
			err := dbpool.QueryRow(ctx, `
				SELECT "last_vacuum" IS NOT NULL, "last_analyze" IS NOT NULL
					FROM pg_stat_all_tables WHERE "schemaname"=$1 AND "relname"=$2
			`, infra.Name, table).Scan(&vacuumed, &analyzed)
			return err == nil && vacuumed && analyzed
		}, 10*time.Second, 50*time.Millisecond, "table %s should have been vacuumed and analyzed", table)
	}

	var otherVacuumed, otherAnalyzed bool
	err = dbpool.QueryRow(ctx, `
		SELECT "last_vacuum" IS NOT NULL, "last_analyze" IS NOT NULL
			FROM pg_stat_all_tables WHERE "schemaname"=$1 AND "relname"='other'
	`, otherSchema).Scan(&otherVacuumed, &otherAnalyzed)
	require.NoError(t, err)
	assert.False(t, otherVacuumed, "a table outside the schema should not have been vacuumed")
	assert.False(t, otherAnalyzed, "a table outside the schema should not have been analyzed")

	// Releasing a connection resets only the application name and the search path, so a lifted timeout
	// left on one would be inherited by whoever acquires it next. We check every connection of the pool,
	// because the vacuum used whichever one it happened to get.
	conns := make([]*pgxpool.Conn, 0, internalStore.TestMaxDBPoolConnections)
	for range internalStore.TestMaxDBPoolConnections {
		conn, err := dbpool.Acquire(ctx)
		require.NoError(t, err)
		conns = append(conns, conn)
		var timeoutAfter string
		require.NoError(t, conn.QueryRow(ctx, `SHOW statement_timeout`).Scan(&timeoutAfter))
		assert.Equal(t, timeoutBefore, timeoutAfter, "the lifted statement timeout should not outlive the vacuum")
	}
	for _, conn := range conns {
		conn.Release()
	}
}
