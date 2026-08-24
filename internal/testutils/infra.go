package testutils

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// Infra is the live PostgreSQL and ElasticSearch a test runs against, set up for that test alone and
// torn down when it ends.
type Infra struct {
	// Ctx is the test's context, carrying its logger and the values the application name of its
	// PostgreSQL connections is derived from. It lives exactly as long as the test does, which is what
	// the infra is, so it is kept here instead of being returned alongside.
	//
	//nolint:containedctx
	Ctx context.Context

	// Logger writes to the test's output.
	Logger zerolog.Logger

	// Name is unique to the test and names everything the test gets: its PostgreSQL schema and, where one
	// is used, its ElasticSearch index.
	Name string

	// DBPool is nil for an infra set up without PostgreSQL.
	DBPool *pgxpool.Pool

	// ESClient is nil for an infra set up without ElasticSearch.
	ESClient *elasticsearch.TypedClient
}

// NewPostgres skips the test when POSTGRES is not configured, and otherwise returns an infra with a
// PostgreSQL schema of the test's own and a connection pool for it.
func NewPostgres(t *testing.T) Infra {
	t.Helper()

	return newInfra(t, true, false)
}

// NewElastic skips the test when ELASTIC is not configured, and otherwise returns an infra with an
// ElasticSearch client. It sets up no PostgreSQL.
func NewElastic(t *testing.T) Infra {
	t.Helper()

	return newInfra(t, false, true)
}

// NewPostgresAndElastic skips the test when either POSTGRES or ELASTIC is not configured, and otherwise
// returns an infra with both.
func NewPostgresAndElastic(t *testing.T) Infra {
	t.Helper()

	return newInfra(t, true, true)
}

func newInfra(t *testing.T, withPostgres, withElastic bool) Infra {
	t.Helper()

	if withPostgres && os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}
	if withElastic && os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	name := "s" + strings.ToLower(identifier.New().String())

	ctx := logger.WithContext(t.Context())
	ctx = internalStore.WithFallbackDBContext(ctx, name, "tests")

	infra := Infra{Ctx: ctx, Logger: logger, Name: name, DBPool: nil, ESClient: nil}

	if withPostgres {
		// We use context.WithoutCancel here because we want to cancel the pool ourselves and not when context
		// is cancelled (so that cleanup code which needs PostgreSQL access can continue to use connections).
		dbCtx := internalStore.WithMaxDBPoolConnections(context.WithoutCancel(ctx), internalStore.TestMaxDBPoolConnections)
		dbpool, dbpoolCleanup, errE := internalStore.InitPostgres(dbCtx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
			return name, "tests"
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		t.Cleanup(dbpoolCleanup)

		errE = internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			return internalStore.EnsureSchema(ctx, tx, name)
		})
		require.NoError(t, errE, "% -+#.1v", errE)

		infra.DBPool = dbpool
	}

	if withElastic {
		esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
		require.NoError(t, errE, "% -+#.1v", errE)

		infra.ESClient = esClient
	}

	return infra
}

// DeleteIndexOnCleanup removes the given ElasticSearch index when the test ends.
func DeleteIndexOnCleanup(t *testing.T, esClient *elasticsearch.TypedClient, index string) {
	t.Helper()

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		errE := internalSearch.DeleteIndex(context.Background(), esClient, index)
		require.NoError(t, errE, "% -+#.1v", errE)
	})
}
