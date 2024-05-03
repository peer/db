package coordinator_test

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
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/coordinator"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

func initDatabase[Data, Metadata any](
	t *testing.T, dataType string,
	endCallback func(ctx context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E),
) (context.Context, *coordinator.Coordinator[Data, Metadata], *internal.LockableSlice[coordinator.AppendedOperation], *internal.LockableSlice[identifier.Identifier]) {
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

	appendedChannel := make(chan coordinator.AppendedOperation)
	t.Cleanup(func() { close(appendedChannel) })
	endedChannel := make(chan identifier.Identifier)
	t.Cleanup(func() { close(endedChannel) })

	appendedChannelContents := new(internal.LockableSlice[coordinator.AppendedOperation])

	go func() {
		for o := range appendedChannel {
			appendedChannelContents.Append(o)
		}
	}()

	endedChannelContents := new(internal.LockableSlice[identifier.Identifier])

	go func() {
		for s := range endedChannel {
			endedChannelContents.Append(s)
		}
	}()

	c := &coordinator.Coordinator[Data, Metadata]{
		Prefix:       prefix,
		Appended:     appendedChannel,
		Ended:        endedChannel,
		DataType:     dataType,
		MetadataType: dataType,
		EndCallback:  endCallback,
	}

	errE = c.Init(ctx, dbpool)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, c, appendedChannelContents, endedChannelContents
}

func TestTop(t *testing.T) {
	t.Parallel()

	endedSessions := []identifier.Identifier{}
	ctx, c, appendedChannelContents, endedChannelContents := initDatabase[json.RawMessage, json.RawMessage](t, "jsonb", func(ctx context.Context, session identifier.Identifier, metadata json.RawMessage) (json.RawMessage, errors.E) {
		endedSessions = append(endedSessions, session)
		return metadata, nil
	})

	session, errE := c.Begin(ctx, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	d := json.RawMessage(internal.DummyData)
	i, errE := c.Push(ctx, session, &d, internal.DummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), i)

	time.Sleep(10 * time.Millisecond)
	appended := appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 1,
		}, appended[0])
	}

	i, errE = c.Push(ctx, session, nil, internal.DummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), i)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 2,
		}, appended[0])
	}

	d = json.RawMessage(internal.DummyData)
	errE = c.Set(ctx, session, 3, &d, internal.DummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 3,
		}, appended[0])
	}

	errE = c.Set(ctx, session, 4, nil, internal.DummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 4,
		}, appended[0])
	}

	operations, errE := c.List(ctx, session, nil)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{4, 3, 2, 1}, operations)

	data, metadata, errE := c.GetData(ctx, session, 1)
	assert.NoError(t, errE, "% -+#.1v", errE)
	e := json.RawMessage(internal.DummyData)
	assert.Equal(t, &e, data)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	data, metadata, errE = c.GetData(ctx, session, 2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	data, metadata, errE = c.GetData(ctx, session, 3)
	assert.NoError(t, errE, "% -+#.1v", errE)
	e = json.RawMessage(internal.DummyData)
	assert.Equal(t, &e, data)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	data, metadata, errE = c.GetData(ctx, session, 4)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	metadata, errE = c.GetMetadata(ctx, session, 1)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	metadata, errE = c.GetMetadata(ctx, session, 2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	metadata, errE = c.GetMetadata(ctx, session, 3)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	metadata, errE = c.GetMetadata(ctx, session, 4)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(internal.DummyData), metadata)

	beginMetadata, endMetadata, errE := c.Get(ctx, session)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(internal.DummyData), beginMetadata)
	assert.Nil(t, endMetadata)

	assert.Len(t, endedSessions, 0)
	errE = c.End(ctx, session, internal.DummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	if assert.Len(t, endedSessions, 1) {
		assert.Equal(t, session, endedSessions[0])
	}

	time.Sleep(10 * time.Millisecond)
	ended := endedChannelContents.Prune()
	if assert.Len(t, ended, 1) {
		assert.Equal(t, session, ended[0])
	}

	// Nothing new since the last time.
	appended = appendedChannelContents.Prune()
	assert.Empty(t, appended)
}
