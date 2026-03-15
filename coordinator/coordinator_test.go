package coordinator_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
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
	BeginMetadata    Metadata
	Append1Data      Data
	Append1Metadata  Metadata
	Append2Data      Data
	Append2Metadata  Metadata
	Append3Data      Data
	Append3Metadata  Metadata
	Append4Data      Data
	Append4Metadata  Metadata
	EndMetadata      Metadata
	CompleteMetadata Metadata
}

func TestHappyPath(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testHappyPath(t, testCase[*internal.TestData, *internal.TestMetadata]{
				BeginMetadata:    &internal.TestMetadata{Metadata: "begin"},
				Append1Data:      &internal.TestData{Data: 123, Patch: false},
				Append1Metadata:  &internal.TestMetadata{Metadata: "append1"},
				Append2Data:      nil,
				Append2Metadata:  &internal.TestMetadata{Metadata: "append2"},
				Append3Data:      &internal.TestData{Data: 345, Patch: false},
				Append3Metadata:  &internal.TestMetadata{Metadata: "append3"},
				Append4Data:      nil,
				Append4Metadata:  &internal.TestMetadata{Metadata: "append4"},
				EndMetadata:      &internal.TestMetadata{Metadata: "end"},
				CompleteMetadata: &internal.TestMetadata{Metadata: "complete"},
			}, dataType)

			testHappyPath(t, testCase[json.RawMessage, json.RawMessage]{
				BeginMetadata:    json.RawMessage(`{"metadata": "begin"}`),
				Append1Data:      json.RawMessage(`{"data": 123}`),
				Append1Metadata:  json.RawMessage(`{"metadata": "append1"}`),
				Append2Data:      nil,
				Append2Metadata:  json.RawMessage(`{"metadata": "append2"}`),
				Append3Data:      json.RawMessage(`{"data": 345}`),
				Append3Metadata:  json.RawMessage(`{"metadata": "append3"}`),
				Append4Data:      nil,
				Append4Metadata:  json.RawMessage(`{"metadata": "append4"}`),
				EndMetadata:      json.RawMessage(`{"metadata": "end"}`),
				CompleteMetadata: json.RawMessage(`{"metadata": "complete"}`),
			}, dataType)

			testHappyPath(t, testCase[*json.RawMessage, *json.RawMessage]{
				BeginMetadata:    internal.ToRawMessagePtr(`{"metadata": "begin"}`),
				Append1Data:      internal.ToRawMessagePtr(`{"data": 123}`),
				Append1Metadata:  internal.ToRawMessagePtr(`{"metadata": "append1"}`),
				Append2Data:      nil,
				Append2Metadata:  internal.ToRawMessagePtr(`{"metadata": "append2"}`),
				Append3Data:      internal.ToRawMessagePtr(`{"data": 345}`),
				Append3Metadata:  internal.ToRawMessagePtr(`{"metadata": "append3"}`),
				Append4Data:      nil,
				Append4Metadata:  internal.ToRawMessagePtr(`{"metadata": "append4"}`),
				EndMetadata:      internal.ToRawMessagePtr(`{"metadata": "end"}`),
				CompleteMetadata: internal.ToRawMessagePtr(`{"metadata": "complete"}`),
			}, dataType)

			testHappyPath(t, testCase[[]byte, []byte]{
				BeginMetadata:    []byte(`{"metadata": "begin"}`),
				Append1Data:      []byte(`{"data": 123}`),
				Append1Metadata:  []byte(`{"metadata": "append1"}`),
				Append2Data:      nil,
				Append2Metadata:  []byte(`{"metadata": "append2"}`),
				Append3Data:      []byte(`{"data": 345}`),
				Append3Metadata:  []byte(`{"metadata": "append3"}`),
				Append4Data:      nil,
				Append4Metadata:  []byte(`{"metadata": "append4"}`),
				EndMetadata:      []byte(`{"metadata": "end"}`),
				CompleteMetadata: []byte(`{"metadata": "complete"}`),
			}, dataType)
		})
	}
}

func initDatabase[Data, Metadata any](
	t *testing.T, dataType string,
	completeSession func(context.Context, identifier.Identifier) (Metadata, errors.E),
) (
	context.Context,
	*coordinator.Coordinator[Data, Metadata, Metadata, Metadata, Metadata],
	*internal.LockableSlice[coordinator.OperationAppended],
	*internal.LockableSlice[coordinator.SessionStateChanged],
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

	c := &coordinator.Coordinator[Data, Metadata, Metadata, Metadata, Metadata]{
		Prefix:          prefix,
		DataType:        dataType,
		MetadataType:    dataType,
		CompleteSession: completeSession,
	}

	errE = c.Init(ctx, dbpool, listener, schema, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := riverClient.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-riverClient.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	appendedChannelContents := new(internal.LockableSlice[coordinator.OperationAppended])

	go func() {
		for {
			ch, _ := c.Appended.Get(ctx)
			select {
			case o, ok := <-ch:
				if ok {
					appendedChannelContents.Append(o)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	changedChannelContents := new(internal.LockableSlice[coordinator.SessionStateChanged])

	go func() {
		for {
			ch, _ := c.Changed.Get(ctx)
			select {
			case s, ok := <-ch:
				if ok {
					changedChannelContents.Append(s)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return ctx, c, appendedChannelContents, changedChannelContents
}

func testHappyPath[Data, Metadata any](t *testing.T, d testCase[Data, Metadata], dataType string) {
	t.Helper()

	completedSessions := new(internal.LockableSlice[identifier.Identifier])
	ctx, c, appendedChannelContents, changedChannelContents := initDatabase[Data, Metadata](
		t, dataType,
		func(_ context.Context, session identifier.Identifier) (Metadata, errors.E) {
			completedSessions.Append(session)
			return d.CompleteMetadata, nil
		},
	)

	session, errE := c.Begin(ctx, d.BeginMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)

	i, errE := c.Append(ctx, session, d.Append1Data, d.Append1Metadata, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), i)

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	appended := appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.OperationAppended{
			Session:   session,
			Operation: 1,
		}, appended[0])
	}

	i, errE = c.Append(ctx, session, d.Append2Data, d.Append2Metadata, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), i)

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.OperationAppended{
			Session:   session,
			Operation: 2,
		}, appended[0])
	}

	operation := int64(3)
	i, errE = c.Append(ctx, session, d.Append3Data, d.Append3Metadata, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(3), i)

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.OperationAppended{
			Session:   session,
			Operation: 3,
		}, appended[0])
	}

	operation = 4
	i, errE = c.Append(ctx, session, d.Append4Data, d.Append4Metadata, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(4), i)

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	appended = appendedChannelContents.Prune()
	if assert.Len(t, appended, 1) {
		assert.Equal(t, coordinator.OperationAppended{
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

	beginMetadata, endMetadata, completeMetadata, errE := c.Get(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.BeginMetadata, beginMetadata)
	assert.Nil(t, endMetadata)
	assert.Nil(t, completeMetadata)

	assert.Empty(t, completedSessions.Prune())
	errE = c.End(ctx, session, d.EndMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Eventually(t, func() bool { return completedSessions.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	completed := completedSessions.Prune()
	if assert.Len(t, completed, 1) {
		assert.Equal(t, session, completed[0])
	}

	// Wait for the "completed" notification so we know the DB transaction has committed.
	require.Eventually(t, func() bool { return changedChannelContents.Len() >= 2 }, 5*time.Second, 10*time.Millisecond)

	beginMetadata, endMetadata, completeMetadata, errE = c.Get(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, d.BeginMetadata, beginMetadata)
	assert.Equal(t, d.EndMetadata, endMetadata)
	assert.Equal(t, d.CompleteMetadata, completeMetadata)

	changed := changedChannelContents.Prune()
	if assert.Len(t, changed, 2) {
		assert.Equal(t, session, changed[0].Session)
		assert.Equal(t, coordinator.SessionStateEnded, changed[0].State)
		assert.Equal(t, session, changed[1].Session)
		assert.Equal(t, coordinator.SessionStateCompleted, changed[1].State)
	}

	// Nothing new since the last time.
	appended = appendedChannelContents.Prune()
	assert.Empty(t, appended)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, c, _, changedChannelContents := initDatabase[json.RawMessage, json.RawMessage](
		t, "jsonb",
		func(_ context.Context, _ identifier.Identifier) (json.RawMessage, errors.E) {
			return internal.DummyData, nil
		},
	)

	_, _, _, errE := c.Get(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	errE = c.End(ctx, identifier.New(), internal.DummyData)
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

	errE = c.End(ctx, session, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = c.Append(ctx, session, internal.DummyData, internal.DummyData, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	operation = 2
	_, errE = c.Append(ctx, session, internal.DummyData, internal.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	// Operations are still accessible after End (only deleted after Complete).
	_, _, errE = c.GetData(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = c.GetMetadata(ctx, session, 1)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = c.End(ctx, session, internal.DummyData)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	ops, errE := c.List(ctx, session, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []int64{1}, ops)

	// Wait for the River job to complete the session (operations are deleted after Complete).
	require.Eventually(t, func() bool { return changedChannelContents.Len() >= 2 }, 5*time.Second, 50*time.Millisecond)
	changedChannelContents.Prune()

	// Operations are no longer accessible after Complete.
	_, _, errE = c.GetData(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	_, errE = c.GetMetadata(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	_, errE = c.List(ctx, session, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx, c, appendedChannelContents, _ := initDatabase[json.RawMessage, json.RawMessage](
		t, "jsonb",
		func(_ context.Context, _ identifier.Identifier) (json.RawMessage, errors.E) {
			return internal.DummyData, nil
		},
	)

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

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 6000 }, 30*time.Second, 10*time.Millisecond)
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

func TestNotifyRecovery(t *testing.T) {
	t.Parallel()

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

	c := &coordinator.Coordinator[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{
		Prefix:          prefix,
		AppendedSize:    1,
		ChangedSize:     1,
		DataType:        "jsonb",
		MetadataType:    "jsonb",
		CompleteSession: func(_ context.Context, _ identifier.Identifier) (json.RawMessage, errors.E) {
			return json.RawMessage(`{}`), nil
		},
	}

	errE = c.Init(ctx, dbpool, listener, schema, riverClient, workers)
	require.NoError(t, errE, "% -+#.1v", errE)

	err := riverClient.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-riverClient.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	session, errE := c.Begin(ctx, json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	// Append an initial operation to confirm the Appended channel is working.
	_, errE = c.Append(ctx, session, json.RawMessage(`{}`), json.RawMessage(`{}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(tc *assert.CollectT) {
		c, errE := c.Appended.Get(t.Context())
		require.NoError(t, errE, "% -+#.1v", errE)
		select {
		case <-c:
		default:
			assert.Fail(tc, "appended notification not yet received")
		}
	}, 5*time.Second, 10*time.Millisecond)

	// Simulate a reconnection on the OperationAppended channel.
	oldAppendedCh, errE := c.Appended.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	err = c.HandleBacklog(ctx, c.Prefix+"OperationAppended", nil)
	require.NoError(t, errE, "% -+#.1v", err) // This is still errors.E.

	// Old Appended channel must be closed.
	select {
	case _, ok := <-oldAppendedCh:
		require.False(t, ok, "old appended channel should be closed after HandleBacklog")
	case <-time.After(time.Second):
		t.Fatal("old appended channel was not closed by HandleBacklog")
	}

	// A new Appended channel must be created.
	newAppendedCh, errE := c.Appended.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEqual(t, oldAppendedCh, newAppendedCh, "HandleBacklog should create a new Appended channel")

	// Appended operations after the reconnection must arrive on the new channel.
	_, errE = c.Append(ctx, session, json.RawMessage(`{}`), json.RawMessage(`{}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(tc *assert.CollectT) {
		select {
		case <-newAppendedCh:
		default:
			assert.Fail(tc, "appended notification not yet received on new channel")
		}
	}, 5*time.Second, 10*time.Millisecond)

	// Simulate a reconnection on the SessionStateChanged channel.
	oldChangedCh, errE := c.Changed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	err = c.HandleBacklog(ctx, c.Prefix+"SessionStateChanged", nil)
	require.NoError(t, errE, "% -+#.1v", err) // This is still errors.E.

	// Old Changed channel must be closed.
	select {
	case _, ok := <-oldChangedCh:
		require.False(t, ok, "old ended channel should be closed after HandleBacklog")
	case <-time.After(time.Second):
		t.Fatal("old ended channel was not closed by HandleBacklog")
	}

	// A new Changed channel must be created.
	newEndedCh, errE := c.Changed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEqual(t, oldChangedCh, newEndedCh, "HandleBacklog should create a new Changed channel")

	// End the session; the notification must arrive on the new channel.
	errE = c.End(ctx, session, json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(tc *assert.CollectT) {
		select {
		case <-newEndedCh:
		default:
			assert.Fail(tc, "ended notification not yet received on new channel")
		}
	}, 5*time.Second, 10*time.Millisecond)
}
