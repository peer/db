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

type testCase[Data, Metadata any] struct {
	BeginMetadata Metadata
	Push1Data     Data
	Push1Metadata Metadata
	Push2Data     Data
	Push2Metadata Metadata
	Set1Data      Data
	Set1Metadata  Metadata
	Set2Data      Data
	Set2Metadata  Metadata
	EndMetadata   Metadata
}

func TestTop(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		dataType := dataType

		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testTop(t, testCase[*internal.TestData, *internal.TestMetadata]{
				BeginMetadata: &internal.TestMetadata{Metadata: "begin"},
				Push1Data:     &internal.TestData{Data: 123, Patch: false},
				Push1Metadata: &internal.TestMetadata{Metadata: "push1"},
				Push2Data:     nil,
				Push2Metadata: &internal.TestMetadata{Metadata: "push2"},
				Set1Data:      &internal.TestData{Data: 345, Patch: false},
				Set1Metadata:  &internal.TestMetadata{Metadata: "set1"},
				Set2Data:      nil,
				Set2Metadata:  &internal.TestMetadata{Metadata: "set2"},
				EndMetadata:   &internal.TestMetadata{Metadata: "end"},
			}, dataType)

			testTop(t, testCase[json.RawMessage, json.RawMessage]{
				BeginMetadata: json.RawMessage(`{"metadata": "begin"}`),
				Push1Data:     json.RawMessage(`{"data": 123}`),
				Push1Metadata: json.RawMessage(`{"metadata": "push1"}`),
				Push2Data:     nil,
				Push2Metadata: json.RawMessage(`{"metadata": "push2"}`),
				Set1Data:      json.RawMessage(`{"data": 345}`),
				Set1Metadata:  json.RawMessage(`{"metadata": "set1"}`),
				Set2Data:      nil,
				Set2Metadata:  json.RawMessage(`{"metadata": "set2"}`),
				EndMetadata:   json.RawMessage(`{"metadata": "end"}`),
			}, dataType)

			testTop(t, testCase[*json.RawMessage, *json.RawMessage]{
				BeginMetadata: internal.ToRawMessagePtr(`{"metadata": "begin"}`),
				Push1Data:     internal.ToRawMessagePtr(`{"data": 123}`),
				Push1Metadata: internal.ToRawMessagePtr(`{"metadata": "push1"}`),
				Push2Data:     nil,
				Push2Metadata: internal.ToRawMessagePtr(`{"metadata": "push2"}`),
				Set1Data:      internal.ToRawMessagePtr(`{"data": 345}`),
				Set1Metadata:  internal.ToRawMessagePtr(`{"metadata": "set1"}`),
				Set2Data:      nil,
				Set2Metadata:  internal.ToRawMessagePtr(`{"metadata": "set2"}`),
				EndMetadata:   internal.ToRawMessagePtr(`{"metadata": "end"}`),
			}, dataType)

			testTop(t, testCase[[]byte, []byte]{
				BeginMetadata: []byte(`{"metadata": "begin"}`),
				Push1Data:     []byte(`{"data": 123}`),
				Push1Metadata: []byte(`{"metadata": "push1"}`),
				Push2Data:     nil,
				Push2Metadata: []byte(`{"metadata": "push2"}`),
				Set1Data:      []byte(`{"data": 345}`),
				Set1Metadata:  []byte(`{"metadata": "set1"}`),
				Set2Data:      nil,
				Set2Metadata:  []byte(`{"metadata": "set2"}`),
				EndMetadata:   []byte(`{"metadata": "end"}`),
			}, dataType)
		})
	}
}

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

func testTop[Data, Metadata any](t *testing.T, d testCase[Data, Metadata], dataType string) {
	t.Helper()

	endedSessions := []identifier.Identifier{}
	ctx, c, appendedChannelContents, endedChannelContents := initDatabase[Data, Metadata](t, dataType, func(ctx context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E) {
		endedSessions = append(endedSessions, session)
		return metadata, nil
	})

	session, errE := c.Begin(ctx, d.BeginMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)

	i, errE := c.Push(ctx, session, d.Push1Data, d.Push1Metadata)
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

	i, errE = c.Push(ctx, session, d.Push2Data, d.Push2Metadata)
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

	errE = c.Set(ctx, session, 3, d.Set1Data, d.Set1Metadata)
	assert.NoError(t, errE, "% -+#.1v", errE)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 3,
		}, appended[0])
	}

	errE = c.Set(ctx, session, 4, d.Set2Data, d.Set2Metadata)
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
	assert.Equal(t, d.Push1Data, data)
	assert.Equal(t, d.Push1Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, d.Push2Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 3)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Set1Data, data)
	assert.Equal(t, d.Set1Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 4)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, d.Set2Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 1)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Push1Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Push2Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 3)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Set1Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 4)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Set2Metadata, metadata)

	beginMetadata, endMetadata, errE := c.Get(ctx, session)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.BeginMetadata, beginMetadata)
	assert.Nil(t, endMetadata)

	assert.Len(t, endedSessions, 0)
	errE = c.End(ctx, session, d.EndMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)

	if assert.Len(t, endedSessions, 1) {
		assert.Equal(t, session, endedSessions[0])
	}

	beginMetadata, endMetadata, errE = c.Get(ctx, session)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.BeginMetadata, beginMetadata)
	assert.Equal(t, d.EndMetadata, endMetadata)

	time.Sleep(10 * time.Millisecond)
	ended := endedChannelContents.Prune()
	if assert.Len(t, ended, 1) {
		assert.Equal(t, session, ended[0])
	}

	// Nothing new since the last time.
	appended = appendedChannelContents.Prune()
	assert.Empty(t, appended)
}
