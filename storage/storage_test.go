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

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
)

func initDatabase(t *testing.T) (
	context.Context,
	*storage.Storage,
	*internal.LockableSlice[store.CommittedChangesets[[]byte, *storage.FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None]],
) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internal.NewListener(dbpool)

	riverClient, workers, errE := internal.NewRiver(ctx, logger, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &storage.Storage{
		Schema: schema,
		Prefix: prefix,
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

	channelContents := new(internal.LockableSlice[store.CommittedChangesets[
		[]byte, *storage.FileMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, store.None,
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

	session, errE := s.BeginUpload(ctx, 10, "text/plain", "test.txt")
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

	errE = s.EndUpload(ctx, session)
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
						assert.Equal(t, session, changes[0].ID)
					}
				}
			}
		}
	}

	data, metadata, _, errE := s.Store().GetLatest(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("bafooqrxzy"), data)

	assert.False(t, time.Time(metadata.At).IsZero())
	assert.Equal(t, int64(10), metadata.Size)
	assert.Equal(t, "text/plain", metadata.MediaType)
	assert.Equal(t, "test.txt", metadata.Filename)
	assert.Equal(t, `"pToAccwccTt9AbUHM5VQIeF7QsgW0Dv5Ka-eZS5O22Y"`, metadata.Etag)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase(t)

	session, errE := s.BeginUpload(ctx, 10, "text/plain", "test.txt")
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("foo"), 0)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.UploadChunk(ctx, session, []byte("bar"), 5)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session)
	assert.ErrorContains(t, errE, "gap between chunks")

	errE = s.UploadChunk(ctx, session, []byte("zy"), 3)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = s.EndUpload(ctx, session)
	assert.ErrorContains(t, errE, "chunks smaller than file")

	errE = s.UploadChunk(ctx, session, []byte("large"), 8)
	assert.ErrorContains(t, errE, "chunk larger than file")

	errE = s.DiscardUpload(ctx, session)
	assert.NoError(t, errE, "% -+#.1v", errE)
}
