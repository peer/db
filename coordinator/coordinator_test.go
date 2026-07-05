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
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
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
	CompleteData     Metadata
	CompleteMetadata Metadata
}

func TestHappyPath(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testHappyPath(t, testCase[*testutils.TestData, *testutils.TestMetadata]{
				BeginMetadata:    &testutils.TestMetadata{Metadata: "begin"},
				Append1Data:      &testutils.TestData{Data: 123, Patch: false},
				Append1Metadata:  &testutils.TestMetadata{Metadata: "append1"},
				Append2Data:      nil,
				Append2Metadata:  &testutils.TestMetadata{Metadata: "append2"},
				Append3Data:      &testutils.TestData{Data: 345, Patch: false},
				Append3Metadata:  &testutils.TestMetadata{Metadata: "append3"},
				Append4Data:      nil,
				Append4Metadata:  &testutils.TestMetadata{Metadata: "append4"},
				EndMetadata:      &testutils.TestMetadata{Metadata: "end"},
				CompleteData:     &testutils.TestMetadata{Metadata: "data"},
				CompleteMetadata: &testutils.TestMetadata{Metadata: "complete"},
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
				CompleteData:     json.RawMessage(`{"metadata": "data"}`),
				CompleteMetadata: json.RawMessage(`{"metadata": "complete"}`),
			}, dataType)

			testHappyPath(t, testCase[*json.RawMessage, *json.RawMessage]{
				BeginMetadata:    testutils.ToRawMessagePtr(`{"metadata": "begin"}`),
				Append1Data:      testutils.ToRawMessagePtr(`{"data": 123}`),
				Append1Metadata:  testutils.ToRawMessagePtr(`{"metadata": "append1"}`),
				Append2Data:      nil,
				Append2Metadata:  testutils.ToRawMessagePtr(`{"metadata": "append2"}`),
				Append3Data:      testutils.ToRawMessagePtr(`{"data": 345}`),
				Append3Metadata:  testutils.ToRawMessagePtr(`{"metadata": "append3"}`),
				Append4Data:      nil,
				Append4Metadata:  testutils.ToRawMessagePtr(`{"metadata": "append4"}`),
				EndMetadata:      testutils.ToRawMessagePtr(`{"metadata": "end"}`),
				CompleteData:     testutils.ToRawMessagePtr(`{"metadata": "data"}`),
				CompleteMetadata: testutils.ToRawMessagePtr(`{"metadata": "complete"}`),
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
				CompleteData:     []byte(`{"metadata": "data"}`),
				CompleteMetadata: []byte(`{"metadata": "complete"}`),
			}, dataType)
		})
	}
}

func initDatabase[Data, Metadata any](
	t *testing.T, dataType string,
	completeSession func(context.Context, identifier.Identifier) (Metadata, errors.E),
	completeSessionTx func(context.Context, identifier.Identifier, Metadata) (Metadata, errors.E),
	completeSessionOnErrorTx func(context.Context, identifier.Identifier, error) (Metadata, errors.E),
) (
	context.Context,
	*coordinator.Coordinator[Data, Metadata, Metadata, Metadata, Metadata, Metadata],
	*testutils.LockableSlice[coordinator.OperationAppended],
	*testutils.LockableSlice[coordinator.SessionStateChanged],
) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
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

	r, errE := internalStore.NewRiver(ctx, logger, nil, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	if completeSessionOnErrorTx == nil {
		// Tests that do not trigger a permanent completion error do not exercise this path. If one ever
		// does without configuring it, fail loudly rather than completing with NULL metadata.
		completeSessionOnErrorTx = func(_ context.Context, _ identifier.Identifier, _ error) (Metadata, errors.E) {
			var zero Metadata
			return zero, errors.New("completeSessionOnErrorTx not configured for this test")
		}
	}

	c := &coordinator.Coordinator[Data, Metadata, Metadata, Metadata, Metadata, Metadata]{
		Prefix:          prefix,
		DataType:        dataType,
		MetadataType:    dataType,
		CompleteSession: completeSession,
		CompleteSessionTx: func(ctx context.Context, _ pgx.Tx, session identifier.Identifier, data Metadata) (Metadata, errors.E) {
			return completeSessionTx(ctx, session, data)
		},
		CompleteSessionOnErrorTx: func(ctx context.Context, _ pgx.Tx, session identifier.Identifier, completeErr error) (Metadata, errors.E) {
			return completeSessionOnErrorTx(ctx, session, completeErr)
		},
	}

	errE = c.Init(ctx, dbpool, listener, r)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = r.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-r.Client.Stopped()
	})

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	appendedChannelContents := new(testutils.LockableSlice[coordinator.OperationAppended])

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

	changedChannelContents := new(testutils.LockableSlice[coordinator.SessionStateChanged])

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

	completedSessions := new(testutils.LockableSlice[identifier.Identifier])
	ctx, c, appendedChannelContents, changedChannelContents := initDatabase[Data, Metadata](
		t, dataType,
		func(_ context.Context, _ identifier.Identifier) (Metadata, errors.E) {
			return d.CompleteData, nil
		},
		func(_ context.Context, session identifier.Identifier, data Metadata) (Metadata, errors.E) {
			assert.Equal(t, d.CompleteData, data)
			completedSessions.Append(session)
			return d.CompleteMetadata, nil
		},
		nil,
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

	operations, errE := c.ListDesc(ctx, session, nil)
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
		nil,
		func(_ context.Context, _ identifier.Identifier, _ json.RawMessage) (json.RawMessage, errors.E) {
			return testutils.DummyData, nil
		},
		nil,
	)

	_, _, _, errE := c.Get(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	errE = c.End(ctx, identifier.New(), testutils.DummyData)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, errE = c.Append(ctx, identifier.New(), testutils.DummyData, testutils.DummyData, nil)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	operation := int64(1)
	_, errE = c.Append(ctx, identifier.New(), testutils.DummyData, testutils.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, _, errE = c.GetData(ctx, identifier.New(), 1)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	_, errE = c.GetMetadata(ctx, identifier.New(), 1)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	session, errE := c.Begin(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	i, errE := c.Append(ctx, session, testutils.DummyData, testutils.DummyData, &operation)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), i)

	_, errE = c.Append(ctx, session, testutils.DummyData, testutils.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrConflict)

	_, _, errE = c.GetData(ctx, session, 2)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)

	_, errE = c.GetMetadata(ctx, session, 2)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)

	errE = c.End(ctx, session, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = c.Append(ctx, session, testutils.DummyData, testutils.DummyData, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	operation = 2
	_, errE = c.Append(ctx, session, testutils.DummyData, testutils.DummyData, &operation)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	// Operations are still accessible after End and before Complete (only deleted after Complete).
	// The completion job may have already run, so both nil and ErrAlreadyCompleted are valid.
	_, _, errE = c.GetData(ctx, session, 1)
	assert.True(t, errE == nil || errors.Is(errE, coordinator.ErrAlreadyCompleted), "% -+#.1v", errE)

	_, errE = c.GetMetadata(ctx, session, 1)
	assert.True(t, errE == nil || errors.Is(errE, coordinator.ErrAlreadyCompleted), "% -+#.1v", errE)

	errE = c.End(ctx, session, testutils.DummyData)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyEnded)

	ops, errE := c.ListDesc(ctx, session, nil)
	assert.True(t, errE == nil || errors.Is(errE, coordinator.ErrAlreadyCompleted), "% -+#.1v", errE)
	if errE == nil {
		assert.Equal(t, []int64{1}, ops)
	}

	// Wait for the River job to complete the session (operations are deleted after Complete).
	require.Eventually(t, func() bool { return changedChannelContents.Len() >= 2 }, 5*time.Second, 50*time.Millisecond)
	changedChannelContents.Prune()

	// Operations are no longer accessible after Complete.
	_, _, errE = c.GetData(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	_, errE = c.GetMetadata(ctx, session, 1)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	_, errE = c.ListDesc(ctx, session, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx, c, appendedChannelContents, _ := initDatabase[json.RawMessage, json.RawMessage](
		t, "jsonb",
		nil,
		func(_ context.Context, _ identifier.Identifier, _ json.RawMessage) (json.RawMessage, errors.E) {
			return testutils.DummyData, nil
		},
		nil,
	)

	operations := []int64{} //nolint:prealloc

	session, errE := c.Begin(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := range 6000 {
		o, errE := c.Append(ctx, session, testutils.DummyData, testutils.DummyData, nil)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)

		operations = append(operations, o)
	}

	page1, errE := c.ListDesc(ctx, session, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, coordinator.MaxPageLength)

	page2, errE := c.ListDesc(ctx, session, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1000)

	allPages := make([]int64, 0, len(page1)+len(page2))
	allPages = append(allPages, page1...)
	allPages = append(allPages, page2...)

	slices.Sort(operations)
	slices.Reverse(operations)

	assert.Equal(t, operations, allPages)

	require.Eventually(t, func() bool { return appendedChannelContents.Len() >= 6000 }, 30*time.Second, 10*time.Millisecond)
	appended := appendedChannelContents.Prune()
	assert.Len(t, appended, 6000)

	_, errE = c.ListDesc(ctx, identifier.New(), nil)
	assert.ErrorIs(t, errE, coordinator.ErrSessionNotFound)

	// Having no more values is not an error.
	page3, errE := c.ListDesc(ctx, session, &page2[999])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page3)

	// Using unknown before operation is an error.
	before := int64(10000)
	_, errE = c.ListDesc(ctx, session, &before)
	assert.ErrorIs(t, errE, coordinator.ErrOperationNotFound)
}

// TestCompleteSessionOnError verifies that when completion fails with a permanent error, the
// CompleteSessionOnErrorTx callback runs: the session is still completed (its operations are deleted)
// with the on-error metadata, and the callback receives the failing error.
func TestCompleteSessionOnError(t *testing.T) {
	t.Parallel()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	completeErrCh := make(chan error, 1)

	ctx, c, _, _ := initDatabase[json.RawMessage, json.RawMessage](
		t, "jsonb",
		nil,
		func(_ context.Context, _ identifier.Identifier, _ json.RawMessage) (json.RawMessage, errors.E) {
			// Completion fails deterministically, so the job is cancelled instead of retried.
			return nil, errors.WrapWith(errors.New("boom"), coordinator.ErrInvalidSessionData)
		},
		func(_ context.Context, _ identifier.Identifier, completeErr error) (json.RawMessage, errors.E) {
			select {
			case completeErrCh <- completeErr:
			default:
			}
			return json.RawMessage(`{"errored": true}`), nil
		},
	)

	session, errE := c.Begin(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = c.Append(ctx, session, testutils.DummyData, testutils.DummyData, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = c.End(ctx, session, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The normal completion fails, but the on-error completion still completes the session.
	var completeMetadata json.RawMessage
	require.Eventually(t, func() bool {
		_, _, completeMetadata, errE = c.Get(ctx, session)
		return errE == nil && completeMetadata != nil
	}, 5*time.Second, 10*time.Millisecond)
	assert.JSONEq(t, `{"errored": true}`, string(completeMetadata))

	// The session's operations were deleted when it completed.
	_, errE = c.ListDesc(ctx, session, nil)
	assert.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	// The callback received the permanent error that failed completion.
	select {
	case completeErr := <-completeErrCh:
		assert.ErrorIs(t, completeErr, coordinator.ErrInvalidSessionData)
	default:
		t.Error("CompleteSessionOnErrorTx was not called")
	}
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

	r, errE := internalStore.NewRiver(ctx, logger, nil, dbpool, schema)
	require.NoError(t, errE, "% -+#.1v", errE)

	c := &coordinator.Coordinator[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{
		Prefix:       prefix,
		AppendedSize: 1,
		ChangedSize:  1,
		DataType:     "jsonb",
		MetadataType: "jsonb",
		CompleteSessionTx: func(_ context.Context, _ pgx.Tx, _ identifier.Identifier, _ json.RawMessage) (json.RawMessage, errors.E) {
			return json.RawMessage(`{}`), nil
		},
		CompleteSessionOnErrorTx: func(_ context.Context, _ pgx.Tx, _ identifier.Identifier, _ error) (json.RawMessage, errors.E) {
			return json.RawMessage(`{}`), nil
		},
	}

	errE = c.Init(ctx, dbpool, listener, r)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = r.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Cleanup(func() {
		// Wait for the client to stop.
		<-r.Client.Stopped()
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
	err := c.HandleBacklog(ctx, schema+"_"+c.Prefix+"Operation", nil)
	require.NoError(t, err, "% -+#.1v", err) // This is still errors.E.

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
	err = c.HandleBacklog(ctx, schema+"_"+c.Prefix+"Session", nil)
	require.NoError(t, err, "% -+#.1v", err) // This is still errors.E.

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

// TestSessionNotificationSurvivesSchemaRename verifies that the coordinator's session NOTIFY channel tracks the
// schema its functions run in (via current_schema()) rather than a name baked in at creation, so it keeps
// working after a blue-green schema rename. We rename the schema, seed a session, call EndSession with the
// renamed schema's search_path (as the app does), and require the NOTIFY on the renamed schema's channel; the
// pre-fix function would notify the old schema name and the wait would time out.
func TestSessionNotificationSurvivesSchemaRename(t *testing.T) {
	t.Parallel()

	ctx, c, _, _ := initDatabase[json.RawMessage, json.RawMessage](
		t, "jsonb",
		nil,
		func(_ context.Context, _ identifier.Identifier, _ json.RawMessage) (json.RawMessage, errors.E) {
			return testutils.DummyData, nil
		},
		nil,
	)
	prefix := c.Prefix

	conn, err := pgx.Connect(ctx, os.Getenv("POSTGRES"))
	require.NoError(t, err)
	defer conn.Close(context.Background()) //nolint:errcheck
	listenConn, err := pgx.Connect(ctx, os.Getenv("POSTGRES"))
	require.NoError(t, err)
	defer listenConn.Close(context.Background()) //nolint:errcheck

	// initDatabase does not expose the schema it created its objects in; find it from the Sessions table (the
	// prefix is unique per test).
	var oldSchema string
	err = conn.QueryRow(ctx, `SELECT schemaname FROM pg_tables WHERE tablename = $1`, prefix+"Sessions").Scan(&oldSchema)
	require.NoError(t, err)

	newSchema := "s" + strings.ToLower(identifier.New().String())
	_, err = conn.Exec(ctx, `ALTER SCHEMA "`+oldSchema+`" RENAME TO "`+newSchema+`"`)
	require.NoError(t, err)

	// Seed a not-yet-ended session in the renamed schema so EndSession has something to end.
	session := identifier.New().String()
	_, err = conn.Exec(ctx, `INSERT INTO "`+newSchema+`"."`+prefix+`Sessions" ("session", "beginMetadata") VALUES ($1, $2)`,
		session, json.RawMessage(`{}`))
	require.NoError(t, err)

	// The channel a coordinator configured for the renamed schema listens on (Schema + "_" + Prefix + "Session").
	wantChannel := newSchema + "_" + prefix + "Session"
	_, err = listenConn.Exec(ctx, `LISTEN "`+wantChannel+`"`)
	require.NoError(t, err)

	// Call EndSession with search_path set to the renamed schema, exactly as the app's connection has it, so
	// current_schema() inside the function resolves to it. With the fix the function notifies wantChannel; without
	// it the function keeps notifying the pre-rename schema's channel and the wait below times out.
	_, err = conn.Exec(ctx, `SET search_path TO "`+newSchema+`"`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `SELECT "`+prefix+`EndSession"($1, $2)`, session, json.RawMessage(`{}`))
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	notification, err := listenConn.WaitForNotification(waitCtx)
	require.NoError(t, err, "expected a NOTIFY on the renamed schema's channel; without the fix EndSession notifies the pre-rename schema name")
	assert.Equal(t, wantChannel, notification.Channel)
}
