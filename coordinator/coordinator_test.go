package coordinator_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
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
	BeginMetadata   Metadata
	Append1Data     Data
	Append1Metadata Metadata
	Append2Data     Data
	Append2Metadata Metadata
	Append3Data     Data
	Append3Metadata Metadata
	Append4Data     Data
	Append4Metadata Metadata
	EndMetadata     Metadata
}

func TestHappyPath(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testHappyPath(t, testCase[*internal.TestData, *internal.TestMetadata]{
				BeginMetadata:   &internal.TestMetadata{Metadata: "begin"},
				Append1Data:     &internal.TestData{Data: 123, Patch: false},
				Append1Metadata: &internal.TestMetadata{Metadata: "append1"},
				Append2Data:     nil,
				Append2Metadata: &internal.TestMetadata{Metadata: "append2"},
				Append3Data:     &internal.TestData{Data: 345, Patch: false},
				Append3Metadata: &internal.TestMetadata{Metadata: "append3"},
				Append4Data:     nil,
				Append4Metadata: &internal.TestMetadata{Metadata: "append4"},
				EndMetadata:     &internal.TestMetadata{Metadata: "end"},
			}, dataType)

			testHappyPath(t, testCase[json.RawMessage, json.RawMessage]{
				BeginMetadata:   json.RawMessage(`{"metadata": "begin"}`),
				Append1Data:     json.RawMessage(`{"data": 123}`),
				Append1Metadata: json.RawMessage(`{"metadata": "append1"}`),
				Append2Data:     nil,
				Append2Metadata: json.RawMessage(`{"metadata": "append2"}`),
				Append3Data:     json.RawMessage(`{"data": 345}`),
				Append3Metadata: json.RawMessage(`{"metadata": "append3"}`),
				Append4Data:     nil,
				Append4Metadata: json.RawMessage(`{"metadata": "append4"}`),
				EndMetadata:     json.RawMessage(`{"metadata": "end"}`),
			}, dataType)

			testHappyPath(t, testCase[*json.RawMessage, *json.RawMessage]{
				BeginMetadata:   internal.ToRawMessagePtr(`{"metadata": "begin"}`),
				Append1Data:     internal.ToRawMessagePtr(`{"data": 123}`),
				Append1Metadata: internal.ToRawMessagePtr(`{"metadata": "append1"}`),
				Append2Data:     nil,
				Append2Metadata: internal.ToRawMessagePtr(`{"metadata": "append2"}`),
				Append3Data:     internal.ToRawMessagePtr(`{"data": 345}`),
				Append3Metadata: internal.ToRawMessagePtr(`{"metadata": "append3"}`),
				Append4Data:     nil,
				Append4Metadata: internal.ToRawMessagePtr(`{"metadata": "append4"}`),
				EndMetadata:     internal.ToRawMessagePtr(`{"metadata": "end"}`),
			}, dataType)

			testHappyPath(t, testCase[[]byte, []byte]{
				BeginMetadata:   []byte(`{"metadata": "begin"}`),
				Append1Data:     []byte(`{"data": 123}`),
				Append1Metadata: []byte(`{"metadata": "append1"}`),
				Append2Data:     nil,
				Append2Metadata: []byte(`{"metadata": "append2"}`),
				Append3Data:     []byte(`{"data": 345}`),
				Append3Metadata: []byte(`{"metadata": "append3"}`),
				Append4Data:     nil,
				Append4Metadata: []byte(`{"metadata": "append4"}`),
				EndMetadata:     []byte(`{"metadata": "end"}`),
			}, dataType)
		})
	}
}

func initDatabase[Data, Metadata any](
	t *testing.T, dataType string,
	endCallback func(ctx context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E),
) (
	context.Context,
	*coordinator.Coordinator[Data, Metadata, Metadata, Metadata],
	*internal.LockableSlice[coordinator.AppendedOperation],
	*internal.LockableSlice[identifier.Identifier],
) {
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
	}, nil)
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

	c := &coordinator.Coordinator[Data, Metadata, Metadata, Metadata]{
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

func testHappyPath[Data, Metadata any](t *testing.T, d testCase[Data, Metadata], dataType string) {
	t.Helper()

	endedSessions := []identifier.Identifier{}
	ctx, c, appendedChannelContents, endedChannelContents := initDatabase[Data, Metadata](
		t, dataType,
		func(_ context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E) {
			endedSessions = append(endedSessions, session)
			return metadata, nil
		},
	)

	session, errE := c.Begin(ctx, d.BeginMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)

	i, errE := c.Append(ctx, session, d.Append1Data, d.Append1Metadata, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), i)

	time.Sleep(10 * time.Millisecond)
	appended := appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 1,
		}, appended[0])
	}

	i, errE = c.Append(ctx, session, d.Append2Data, d.Append2Metadata, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), i)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 2,
		}, appended[0])
	}

	operation := int64(3)
	i, errE = c.Append(ctx, session, d.Append3Data, d.Append3Metadata, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(3), i)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 3,
		}, appended[0])
	}

	operation = 4
	i, errE = c.Append(ctx, session, d.Append4Data, d.Append4Metadata, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(4), i)

	time.Sleep(10 * time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.AppendedOperation{
			Session:   session,
			Operation: 4,
		}, appended[0])
	}

	operations, errE := c.List(ctx, session, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{4, 3, 2, 1}, operations)

	data, metadata, errE := c.GetData(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append1Data, data)
	assert.Equal(t, d.Append1Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, d.Append2Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 3)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append3Data, data)
	assert.Equal(t, d.Append3Metadata, metadata)

	data, metadata, errE = c.GetData(ctx, session, 4)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Nil(t, data)
	assert.Equal(t, d.Append4Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append1Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append2Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 3)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append3Metadata, metadata)

	metadata, errE = c.GetMetadata(ctx, session, 4)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.Append4Metadata, metadata)

	beginMetadata, endMetadata, errE := c.Get(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.BeginMetadata, beginMetadata)
	assert.Nil(t, endMetadata)

	assert.Empty(t, endedSessions)
	_, errE = c.End(ctx, session, d.EndMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)

	if assert.Len(t, endedSessions, 1) {
		assert.Equal(t, session, endedSessions[0])
	}

	beginMetadata, endMetadata, errE = c.Get(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
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

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, c, _, _ := initDatabase[json.RawMessage, json.RawMessage](t, "jsonb", nil)

	_, _, errE := c.Get(ctx, identifier.New())
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, errE = c.End(ctx, identifier.New(), internal.DummyData)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, errE = c.Append(ctx, identifier.New(), internal.DummyData, internal.DummyData, nil)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	operation := int64(1)
	_, errE = c.Append(ctx, identifier.New(), internal.DummyData, internal.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, _, errE = c.GetData(ctx, identifier.New(), 1)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, errE = c.GetMetadata(ctx, identifier.New(), 1)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	session, errE := c.Begin(ctx, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	i, errE := c.Append(ctx, session, internal.DummyData, internal.DummyData, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), i)

	_, errE = c.Append(ctx, session, internal.DummyData, internal.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrConflict)

	_, _, errE = c.GetData(ctx, session, 2)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)

	_, errE = c.GetMetadata(ctx, session, 2)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)

	_, errE = c.End(ctx, session, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = c.Append(ctx, session, internal.DummyData, internal.DummyData, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	operation = 2
	_, errE = c.Append(ctx, session, internal.DummyData, internal.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	_, _, errE = c.GetData(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	_, errE = c.GetMetadata(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	_, errE = c.End(ctx, session, internal.DummyData)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	_, errE = c.List(ctx, session, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx, c, appendedChannelContents, _ := initDatabase[json.RawMessage, json.RawMessage](t, "jsonb", nil)

	operations := []int64{}

	session, errE := c.Begin(ctx, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	for range 6000 {
		o, errE := c.Append(ctx, session, internal.DummyData, internal.DummyData, nil)
		require.NoError(t, errE, "%d % -+#.1v", errE)

		operations = append(operations, o)
	}

	page1, errE := c.List(ctx, session, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, coordinator.MaxPageLength)

	page2, errE := c.List(ctx, session, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1000)

	allPages := []int64{}
	allPages = append(allPages, page1...)
	allPages = append(allPages, page2...)

	slices.Sort(operations)
	slices.Reverse(operations)

	assert.Equal(t, operations, allPages)

	time.Sleep(10 * time.Millisecond)
	appended := appendedChannelContents.Prune()
	assert.Len(t, appended, 6000)

	_, errE = c.List(ctx, identifier.New(), nil)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	// Having no more values is not an error.
	page3, errE := c.List(ctx, session, &page2[999])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page3)

	// Using unknown before operation is an error.
	before := int64(10000)
	_, errE = c.List(ctx, session, &before)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)
}
