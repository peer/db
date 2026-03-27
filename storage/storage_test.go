package storage_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

func initDatabase(t *testing.T) (
	context.Context,
	*storage.Storage,
	*testutils.LockableSlice[store.CommittedChangesets[
		[]byte, *storage.FileMetadata, *internalStore.NoMetadata, *internalStore.NoMetadata, *internalStore.CommitMetadata, store.None,
	]],
) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"

	ctx = internalStore.WithFallbackDBContext(ctx, schema, "tests")

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

	listener := internalStore.NewListener(dbpool)

	riverClient, workers, errE := internalStore.NewRiver(ctx, logger, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &storage.Storage{
		Schema:             schema,
		Prefix:             prefix,
		PrimaryCoordinator: nil,
	}

	errE = s.Init(ctx, dbpool, listener, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := riverClient.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-riverClient.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	channelContents := new(testutils.LockableSlice[store.CommittedChangesets[
		[]byte, *storage.FileMetadata, *internalStore.NoMetadata, *internalStore.NoMetadata, *internalStore.CommitMetadata, store.None,
	]])

	go func() {
		for {
			ch, _ := s.Store().Committed.Get(ctx)
			select {
			case co, ok := <-ch:
				if ok {
					channelContents.Append(co)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ctx, s, channelContents
}

func TestHappyPath(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase(t)

	fileBase := []string{"test", "happypath"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 10, "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("bar"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("qrxzy"), 5)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 2)
	require.NoError(t, errE, "% -+#.1v", errE)

	chunks, errE := s.ListChunks(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{4, 3, 2, 1}, chunks)

	start, length, errE := s.GetChunk(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(3), length)

	start, length, errE = s.GetChunk(ctx, session, 2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(3), length)

	start, length, errE = s.GetChunk(ctx, session, 3)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(5), start)
	assert.Equal(t, int64(5), length)

	start, length, errE = s.GetChunk(ctx, session, 4)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), start)
	assert.Equal(t, int64(3), length)

	errE = s.EndUpload(ctx, session, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s.Store())
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			if assert.Len(t, committed.Changesets, 1) {
				changes, errE := committed.Changesets[0].Changes(ctx, nil)
				if assert.NoError(t, errE, "% -+#.1v", errE) {
					if assert.Len(t, changes, 1) {
						// The file ID is derived from base + "STORAGE" + session.
						expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
						expectedFileID := identifier.From(expectedBase...)
						assert.Equal(t, expectedFileID, changes[0].ID)
					}
				}
			}
		}
	}

	// The file ID is derived from base + "STORAGE" + session.
	expectedBase := append(append([]string{}, fileBase...), "STORAGE", session.String())
	expectedFileID := identifier.From(expectedBase...)

	data, metadata, _, _, errE := s.Store().GetLatest(ctx, expectedFileID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("bafooqrxzy"), data)

	assert.False(t, time.Time(metadata.At).IsZero())
	assert.Equal(t, int64(10), metadata.Size)
	assert.Equal(t, "text/plain", metadata.MediaType)
	assert.Equal(t, "test.txt", metadata.Filename)
	assert.Equal(t, `"pToAccwccTt9AbUHM5VQIeF7QsgW0Dv5Ka-eZS5O22Y"`, metadata.Etag)

	// Verify file metadata Base is recorded and file ID is derivable from it.
	assert.Equal(t, expectedBase, metadata.Base)
	assert.Equal(t, expectedFileID, identifier.From(metadata.Base...))

	// Verify changeset ID is derivable from its base.
	_, _, version, _, errE := s.Store().GetLatest(ctx, expectedFileID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	changesetBase := append(append([]string{}, expectedBase...), "SESSION", session.String())
	assert.Equal(t, identifier.From(changesetBase...), version.Changeset)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase(t)

	fileBase := []string{"test", "errors"}
	session, errE := s.BeginUploadNew(ctx, fileBase, 10, "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("bar"), 5)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session, nil)
	assert.ErrorContains(t, errE, "gap between chunks")

	errE = s.UploadChunk(ctx, session, []byte("zy"), 3)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session, nil)
	assert.ErrorContains(t, errE, "chunks smaller than file")

	errE = s.UploadChunk(ctx, session, []byte("large"), 8)
	assert.ErrorContains(t, errE, "chunk larger than file")

	errE = s.DiscardUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
}
