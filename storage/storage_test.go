package storage_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/storage"
	"gitlab.com/peerdb/peerdb/store"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

func initDatabase(t *testing.T) (context.Context, *storage.Storage, *internal.LockableSlice[store.CommittedChangeset[[]byte, json.RawMessage, store.None]]) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	schema := identifier.New().String()
	prefix := identifier.New().String() + "_"

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.EnsureSchema(ctx, tx, schema)
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	channel := make(chan store.CommittedChangeset[[]byte, json.RawMessage, store.None])
	t.Cleanup(func() { close(channel) })

	channelContents := new(internal.LockableSlice[store.CommittedChangeset[[]byte, json.RawMessage, store.None]])

	go func() {
		for co := range channel {
			channelContents.Append(co)
		}
	}()

	s := &storage.Storage{
		Prefix:    prefix,
		Committed: channel,
	}

	errE = s.Init(ctx, dbpool)
	require.NoError(t, errE, "% -+#.1v", errE)

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

	errE = s.EndUpload(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	time.Sleep(10 * time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		changeset, errE := c[0].WithStore(ctx, s.Store()) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, session, changes[0].ID)
				}
			}
		}
	}

	data, metadata, _, errE := s.Store().GetLatest(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []byte("bafooqrxzy"), data)

	var m struct {
		At        time.Time `json:"at"`
		Size      int64     `json:"size"`
		MediaType string    `json:"mediaType"`
		Filename  string    `json:"filename"`
	}
	errE = x.UnmarshalWithoutUnknownFields(metadata, &m)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(10), m.Size)
	assert.Equal(t, "text/plain", m.MediaType)
	assert.Equal(t, "test.txt", m.Filename)
	assert.False(t, m.At.IsZero())
}
