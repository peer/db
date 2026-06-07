package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

type testCase[Data, Metadata, Patch any] struct {
	InsertData      Data
	InsertMetadata  Metadata
	UpdateData      Data
	UpdateMetadata  Metadata
	UpdatePatch     Patch
	ReplaceData     Data
	ReplaceMetadata Metadata
	DeleteData      Data
	DeleteMetadata  Metadata
	CommitMetadata  Metadata
	NoPatches       []Patch
	UpdatePatches   []Patch
}

func TestTop(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testTop(t, testCase[*testutils.TestData, *testutils.TestMetadata, *testutils.TestPatch]{
				InsertData:      &testutils.TestData{Data: 123, Patch: false},
				InsertMetadata:  &testutils.TestMetadata{Metadata: "foobar"},
				UpdateData:      &testutils.TestData{Data: 123, Patch: true},
				UpdateMetadata:  &testutils.TestMetadata{Metadata: "zoofoo"},
				UpdatePatch:     &testutils.TestPatch{Patch: true},
				ReplaceData:     &testutils.TestData{Data: 345, Patch: false},
				ReplaceMetadata: &testutils.TestMetadata{Metadata: "another"},
				DeleteData:      nil,
				DeleteMetadata:  &testutils.TestMetadata{Metadata: "admin"},
				CommitMetadata:  &testutils.TestMetadata{Metadata: "commit"},
				NoPatches:       []*testutils.TestPatch{},
				UpdatePatches:   []*testutils.TestPatch{{Patch: true}},
			}, dataType)

			testTop(t, testCase[json.RawMessage, json.RawMessage, json.RawMessage]{
				InsertData:      json.RawMessage(`{"data": 123}`),
				InsertMetadata:  json.RawMessage(`{"metadata": "foobar"}`),
				UpdateData:      json.RawMessage(`{"data": 123, "patch": true}`),
				UpdateMetadata:  json.RawMessage(`{"metadata": "zoofoo"}`),
				UpdatePatch:     json.RawMessage(`{"patch": true}`),
				ReplaceData:     json.RawMessage(`{"data": 345}`),
				ReplaceMetadata: json.RawMessage(`{"metadata": "another"}`),
				DeleteData:      nil,
				DeleteMetadata:  json.RawMessage(`{"metadata": "admin"}`),
				CommitMetadata:  json.RawMessage(`{"metadata": "commit"}`),
				NoPatches:       []json.RawMessage{},
				UpdatePatches:   []json.RawMessage{json.RawMessage(`{"patch": true}`)},
			}, dataType)

			testTop(t, testCase[*json.RawMessage, *json.RawMessage, *json.RawMessage]{
				InsertData:      testutils.ToRawMessagePtr(`{"data": 123}`),
				InsertMetadata:  testutils.ToRawMessagePtr(`{"metadata": "foobar"}`),
				UpdateData:      testutils.ToRawMessagePtr(`{"data": 123, "patch": true}`),
				UpdateMetadata:  testutils.ToRawMessagePtr(`{"metadata": "zoofoo"}`),
				UpdatePatch:     testutils.ToRawMessagePtr(`{"patch": true}`),
				ReplaceData:     testutils.ToRawMessagePtr(`{"data": 345}`),
				ReplaceMetadata: testutils.ToRawMessagePtr(`{"metadata": "another"}`),
				DeleteData:      nil,
				DeleteMetadata:  testutils.ToRawMessagePtr(`{"metadata": "admin"}`),
				CommitMetadata:  testutils.ToRawMessagePtr(`{"metadata": "commit"}`),
				NoPatches:       []*json.RawMessage{},
				UpdatePatches:   []*json.RawMessage{testutils.ToRawMessagePtr(`{"patch": true}`)},
			}, dataType)

			testTop(t, testCase[[]byte, []byte, store.None]{
				InsertData:      []byte(`{"data": 123}`),
				InsertMetadata:  []byte(`{"metadata": "foobar"}`),
				UpdateData:      []byte(`{"data": 123, "patch": true}`),
				UpdateMetadata:  []byte(`{"metadata": "zoofoo"}`),
				UpdatePatch:     nil,
				ReplaceData:     []byte(`{"data": 345}`),
				ReplaceMetadata: []byte(`{"metadata": "another"}`),
				DeleteData:      nil,
				DeleteMetadata:  []byte(`{"metadata": "admin"}`),
				CommitMetadata:  []byte(`{"metadata": "commit"}`),
				NoPatches:       nil,
				UpdatePatches:   nil,
			}, dataType)
		})
	}
}

func initDatabase[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any](
	t *testing.T, dataType string,
) (
	context.Context, *store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
	*testutils.LockableSlice[store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]],
	*pgxpool.Pool,
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

	s := &store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		Schema:       schema,
		Prefix:       prefix,
		DataType:     dataType,
		MetadataType: dataType,
		PatchType:    dataType,
	}

	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	channelContents := new(testutils.LockableSlice[store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]])

	go func() {
		for {
			ch, _ := s.Committed.Get(ctx)
			select {
			case c, ok := <-ch:
				if ok {
					channelContents.Append(c)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return ctx, s, channelContents, dbpool
}

func testTop[Data, Metadata, Patch any](t *testing.T, d testCase[Data, Metadata, Patch], dataType string) { //nolint:maintidx
	t.Helper()

	ctx, s, channelContents, _ := initDatabase[Data, Metadata, Metadata, Metadata, Metadata, Patch](t, dataType)

	_, _, _, _, errE := s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	expectedID := identifier.New()

	insertVersion, errE := s.Insert(ctx, expectedID, d.InsertData, d.InsertMetadata, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), insertVersion.Revision)
	}

	data, metadata, resolvedVersion, parentChangesets, errE := s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, version, parentChangesets, errE := s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, version)
		assert.Empty(t, parentChangesets)
	}

	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		insertVersion.Changeset,
	})

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, insertVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, insertVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, insertVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	updateVersion, errE := s.Update(ctx, expectedID, insertVersion.Changeset, d.UpdateData, d.UpdatePatch, d.UpdateMetadata, d.UpdateMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), updateVersion.Revision)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: insertVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, version)
		assert.Equal(t, []store.Version{{Changeset: insertVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, updateVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, updateVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, updateVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	replaceVersion, errE := s.Replace(ctx, expectedID, updateVersion.Changeset, d.ReplaceData, d.ReplaceMetadata, d.ReplaceMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), replaceVersion.Revision)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: insertVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, replaceVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
		assert.Equal(t, replaceVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: updateVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
		assert.Equal(t, replaceVersion, version)
		assert.Equal(t, []store.Version{{Changeset: updateVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		replaceVersion.Changeset,
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, replaceVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, replaceVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, replaceVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	deleteVersion, errE := s.Delete(ctx, expectedID, replaceVersion.Changeset, d.DeleteMetadata, d.DeleteMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), deleteVersion.Revision)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: insertVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, replaceVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
		assert.Equal(t, replaceVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: updateVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, expectedID, deleteVersion)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: replaceVersion.Changeset, Revision: 0}}, parentChangesets)

	// Use returned parentChangesets to fetch the version before deletion.
	if assert.Len(t, parentChangesets, 1) {
		data, metadata, resolvedVersion, _, errE = s.Get(ctx, expectedID, parentChangesets[0])
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			assert.Equal(t, d.ReplaceData, data)
			assert.Equal(t, d.ReplaceMetadata, metadata)
			assert.Equal(t, replaceVersion, resolvedVersion)
		}
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)
	assert.Equal(t, []store.Version{{Changeset: replaceVersion.Changeset, Revision: 0}}, parentChangesets)

	// Use returned parentChangesets from GetLatest to fetch the version before deletion.
	if assert.Len(t, parentChangesets, 1) {
		data, metadata, resolvedVersion, _, errE = s.Get(ctx, expectedID, parentChangesets[0])
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			assert.Equal(t, d.ReplaceData, data)
			assert.Equal(t, d.ReplaceMetadata, metadata)
			assert.Equal(t, replaceVersion, resolvedVersion)
		}
	}

	// After deletion the value is no longer counted as alive even though it
	// remains in List (which includes deleted IDs).
	count, errE = s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(0), count)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		deleteVersion.Changeset,
		replaceVersion.Changeset,
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, deleteVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, deleteVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, deleteVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	newID := identifier.New()

	// Test manual changeset management.
	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE := changeset.Insert(ctx, newID, d.InsertData, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, changeset.ID(), newVersion.Changeset)
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)
	assert.Equal(t, []store.Version{{Changeset: replaceVersion.Changeset, Revision: 0}}, parentChangesets)

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, newID)
	assert.NotErrorIs(t, errE, store.ErrValueDeleted) //nolint:testifylint
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Nil(t, data)
	assert.Nil(t, metadata)
	assert.Empty(t, version)
	assert.Empty(t, parentChangesets)

	time.Sleep(100 * time.Millisecond)
	c = channelContents.Prune()
	assert.Empty(t, c)

	changesets, errE := s.Commit(ctx, changeset, d.CommitMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)
	assert.Equal(t, []store.Version{{Changeset: replaceVersion.Changeset, Revision: 0}}, parentChangesets)

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, newID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
		assert.Empty(t, parentChangesets)
	}

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		newVersion.Changeset,
	})

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, store.Version{
		Changeset: changeset.ID(),
		Revision:  1,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, newVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	newID2 := identifier.New()

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE = changeset.Insert(ctx, newID2, d.InsertData, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, changeset.ID(), newVersion.Changeset)
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	// This time we recreate the changeset object.
	changeset, errE = s.Changeset(ctx, changeset.ID())
	require.NoError(t, errE, "% -+#.1v", errE)

	changesets, errE = s.Commit(ctx, changeset, d.CommitMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID2, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	data, metadata, version, parentChangesets, errE = s.GetLatest(ctx, newID2)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
		assert.Empty(t, parentChangesets)
	}

	testChanges(t, ctx, s, newID2, []identifier.Identifier{
		newVersion.Changeset,
	})

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) { //nolint:dupl
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, newVersion.Changeset, c[0].Changesets[0].ID())
		assert.Positive(t, c[0].Seq)
		committed, errE := c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committed.Changesets[0].Changes(ctx, nil)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID2, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	ids, errE := s.List(ctx, nil)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.ElementsMatch(t, []identifier.Identifier{expectedID, newID, newID2}, ids)
	}

	// List includes the deleted expectedID; Count excludes it.
	count, errE = s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(2), count)
	}
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	ids := []identifier.Identifier{}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := range 6000 {
		newID := identifier.New()
		_, errE = changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)

		ids = append(ids, newID)
	}

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(6000), count)
	}

	page1, errE := s.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, store.MaxPageLength)

	page2, errE := s.List(ctx, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1000)

	inserted := make([]identifier.Identifier, 0, len(page1)+len(page2))
	inserted = append(inserted, page1...)
	inserted = append(inserted, page2...)

	ids = sortIDs(ids...)

	assert.Equal(t, ids, inserted)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	assert.Len(t, c, 1)

	v, errE := s.View(ctx, "unknown")
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = v.List(ctx, nil)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
	_, errE = v.Count(ctx, false)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	// Having no more values is not an error.
	page3, errE := s.List(ctx, &page2[999])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page3)

	// Using unknown after ID is an error.
	newID := identifier.New()
	_, errE = s.List(ctx, &newID)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	csPage1, errE := changeset.Changes(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, csPage1, store.MaxPageLength)

	csPage2, errE := changeset.Changes(ctx, &csPage1[4999].ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, csPage2, 1000)

	changes := make([]store.Change, 0, len(csPage1)+len(csPage2))
	changes = append(changes, csPage1...)
	changes = append(changes, csPage2...)

	expected := make([]store.Change, 0, len(ids))
	for _, id := range ids {
		expected = append(expected, store.Change{
			ID: id,
			Version: store.Version{
				Changeset: changeset.ID(),
				Revision:  1,
			},
		})
	}

	assert.Equal(t, expected, changes)

	// Having no more changes is not an error.
	csPage3, errE := changeset.Changes(ctx, &csPage2[999].ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, csPage3)

	// Using unknown after ID is an error.
	newID = identifier.New()
	_, errE = changeset.Changes(ctx, &newID)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Changes(ctx, nil)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)

	_, errE = changeset.Changes(ctx, &newID)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

func TestChangesPagination(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changesets := []identifier.Identifier{} //nolint:prealloc

	newID := identifier.New()
	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	changesets = append(changesets, version.Changeset)

	var changeset store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]
	for i := range 6000 {
		changeset, errE = s.Begin(ctx)
		require.NoError(t, errE, "% -+#.1v", errE)

		version, errE = changeset.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)

		changesets = append(changesets, version.Changeset)
	}

	// We commit only once (the last changeset in the chain) for test to run faster.
	committed, errE := s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, committed, 6000)

	require.Eventually(t, func() bool { return channelContents.Len() >= 2 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	// One CommittedChangesets per commit: initial insert (1 changeset) + big commit (6000 changesets).
	assert.Len(t, c, 2)
	if len(c) == 2 {
		assert.Len(t, c[0].Changesets, 1)
		assert.Len(t, c[1].Changesets, 6000)
	}

	page1, errE := s.Changes(ctx, newID, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, store.MaxPageLength)

	page2, errE := s.Changes(ctx, newID, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1001)

	changes := make([]identifier.Identifier, 0, len(page1)+len(page2))
	changes = append(changes, page1...)
	changes = append(changes, page2...)
	slices.Reverse(changes)

	assert.Equal(t, changesets, changes)

	changesetID := identifier.New()

	v, errE := s.View(ctx, "unknown")
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		_, errE = v.Changes(ctx, newID, nil)
		assert.ErrorIs(t, errE, store.ErrViewNotFound)
		// Same for the code path with after changeset.
		_, errE = v.Changes(ctx, newID, &changesetID)
		assert.ErrorIs(t, errE, store.ErrViewNotFound)
	}

	// Having no more changes is not an error.
	page3, errE := s.Changes(ctx, newID, &page2[1000])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page3)

	// Changes for unknown value is an error.
	_, errE = s.Changes(ctx, identifier.New(), nil)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	// Same for the code path with after changeset.
	_, errE = s.Changes(ctx, identifier.New(), &changesetID)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Using unknown after changeset is an error.
	_, errE = s.Changes(ctx, newID, &changesetID)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

func TestTwoChangesToSameValueInOneChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE := changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	_, errE = changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Update(ctx, newID, newVersion.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Delete(ctx, newID, newVersion.Changeset, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Update(ctx, newID, newVersion.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Merge(
		ctx, newID, []identifier.Identifier{newVersion.Changeset}, testutils.DummyData, []json.RawMessage{testutils.DummyData}, testutils.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Replace(ctx, newID, newVersion.Changeset, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)
}

func TestCycles(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	newVersion, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	// This is not possible for two reasons:
	// Every changeset can have only one change per value ID.
	// Parent changeset must contain a change for the same value ID - fails here.
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// Some insert, to make changeset exist.
	_, errE = changeset.Insert(ctx, identifier.New(), testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	// This is not possible for two reasons:
	// Every changeset can have only one change per value ID - fails here.
	// Parent changeset must contain a change for the same value ID.
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// Longer cycles are not possible because of construction. Parent changesets always have to
	// exist before new changesets can be made. And it is not possible to update a changeset
	// to close a cycle.
}

func TestInterdependentChangesets(t *testing.T) {
	// It is uncommon but we do support this.

	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()
	secondID := identifier.New()

	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Insert(ctx, secondID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, secondID, changeset1.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Update(ctx, newID, changeset2.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesets, errE := s.Commit(ctx, changeset1, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(
		t,
		[]store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{changeset1, changeset2},
		changesets,
	)

	// Both changesets were committed together (interdependent), so we have
	// two alive values now.
	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(2), count)
	}
}

func TestGetCurrent(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	_, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, _, errE = v.GetLatest(ctx, newID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, _, _, _, errE = s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestGet(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	// View does not really exist.
	_, _, _, _, errE = v.Get(ctx, newID, version) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	// Value at existing changeset does not exist for arbitrary ID.
	_, _, _, _, errE = s.Get(ctx, identifier.New(), version) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Value at arbitrary changeset does not exist for existing ID.
	_, _, _, _, errE = s.Get(ctx, newID, store.Version{ //nolint:dogsled
		Changeset: identifier.New(),
		Revision:  1,
	})
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestMultipleViews(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, parentChangesets, errE := s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	assert.Empty(t, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE := s.Get(ctx, newID, version)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, resolvedVersion)
	assert.Empty(t, parentChangesets)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, _, _, errE = s.Get(ctx, newID, updated) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view.
	updated2, errE := s.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, parentChangesets, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the second (child) view.
	_, _, _, _, errE = v.Get(ctx, newID, updated2) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Committing from the main view into the second (child) view should not be possible
	// because that would introduce two versions of the same value.
	changeset, errE := s.Changeset(ctx, updated2.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// Committing from the second (child) view into the main view should not be possible
	// because that would introduce two versions of the same value.
	changeset, errE = s.Changeset(ctx, updated.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, mainView, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// But we can merge into the main view.
	merged, errE := s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{updated2.Changeset, updated.Changeset},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData, testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be merged.
	_, _, latest, parentChangesets, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	assert.Equal(t, []store.Version{{Changeset: updated2.Changeset, Revision: 0}, {Changeset: updated.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, merged)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: updated2.Changeset, Revision: 0}, {Changeset: updated.Changeset, Revision: 0}}, parentChangesets)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// We can now commit the merged changeset into the second (child) view.
	changeset, errE = s.Changeset(ctx, merged.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be merged.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	assert.Equal(t, []store.Version{{Changeset: updated2.Changeset, Revision: 0}, {Changeset: updated.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, merged)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: updated2.Changeset, Revision: 0}, {Changeset: updated.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})

	// Both views ultimately resolve to the same merged value: one alive
	// value in each view, exercising the closest-view-in-path resolution.
	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
	count, errE = v.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
}

func TestChangeAcrossViews(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, parentChangesets, errE := s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	assert.Empty(t, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE := s.Get(ctx, newID, version)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, resolvedVersion)
	assert.Empty(t, parentChangesets)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, _, _, errE = s.Get(ctx, newID, updated) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view by using the change from the second (child) view.
	// This should commit two changesets to the main view.
	updated2, errE := s.Update(ctx, newID, updated.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, parentChangesets, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	assert.Equal(t, []store.Version{{Changeset: updated.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: updated.Changeset, Revision: 0}}, parentChangesets)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})

	// It should now be possible to get the previously updated version as well in the main view.
	_, _, resolvedVersion, parentChangesets, errE = s.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	// The version in the second (child) view should stay the previously updated version.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the second (child) view.
	_, _, _, _, errE = v.Get(ctx, newID, updated2) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We can explicitly update the second (child) view with the new changeset from the main view.
	changeset, errE := s.Changeset(ctx, updated2.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be updated.
	_, _, latest, parentChangesets, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	assert.Equal(t, []store.Version{{Changeset: updated.Changeset, Revision: 0}}, parentChangesets)
	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, resolvedVersion)
	assert.Equal(t, []store.Version{{Changeset: updated.Changeset, Revision: 0}}, parentChangesets)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})

	// Both views resolve to the same alive value (no deletions in this test).
	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
	count, errE = v.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
}

func TestView(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	v, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	v2, errE := v.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", testutils.DummyData)
	require.ErrorIs(t, errE, store.ErrConflict)

	errE = v2.Release(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE = s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child2", testutils.DummyData)
	require.ErrorIs(t, errE, store.ErrViewNotFound)

	errE = v.Release(ctx, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = v.Count(ctx, false)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

func TestDuplicateValues(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Inserting another value with same ID should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = s.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Updating an old value should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// Despite the failed updates and the conflicting insert above, we have
	// exactly one alive value in the store.
	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), count)
	}
}

func TestDiscardAfterCommit(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)
}

func TestEmptyChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)

	errE = changeset.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	count, errE := s.Count(ctx, false)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(0), count)
	}
}

func TestDiscardInUseChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, newID, changeset.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrInUse)

	errE = changeset2.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func sortIDs(ids ...identifier.Identifier) []identifier.Identifier {
	slices.SortFunc(ids, func(a, b identifier.Identifier) int {
		return bytes.Compare(a[:], b[:])
	})
	return ids
}

func testChanges[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any](
	t *testing.T, ctx context.Context, s *store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], //nolint:revive
	id identifier.Identifier, expected []identifier.Identifier,
) {
	t.Helper()

	v, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, id, expected)
}

func testChangesView[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any](
	t *testing.T, ctx context.Context, v store.View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], //nolint:revive
	id identifier.Identifier, expected []identifier.Identifier,
) {
	t.Helper()

	changes, errE := v.Changes(ctx, id, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, expected, changes)

	for i, c := range changes {
		cs, errE := v.Changes(ctx, id, &c)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, changes[i+1:], cs, "%d %#v", i, c)
	}
}

func TestMultiplePathsToSameChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB1.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB2.Update(ctx, newID, changesetB1.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(
		ctx, newID,
		[]identifier.Identifier{changesetA.ID(), changesetB2.ID()},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData, testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		// Depth 0.
		merged.Changeset,
		// Depth 1.
		sortIDs(changesetB2.ID(), changesetA.ID())[0],
		sortIDs(changesetB2.ID(), changesetA.ID())[1],
		// Depth 2.
		sortIDs(version.Changeset, changesetB1.ID())[0],
		sortIDs(version.Changeset, changesetB1.ID())[1],
	})
}

func TestMultiplePathsSameLengthToSameChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(
		ctx, newID,
		[]identifier.Identifier{changesetA.ID(), changesetB.ID()},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData, testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		// Depth 0.
		merged.Changeset,
		// Depth 1.
		sortIDs(changesetA.ID(), changesetB.ID())[0],
		sortIDs(changesetA.ID(), changesetB.ID())[1],
		// Depth 2.
		version.Changeset,
	})
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	anotherVersion, errE := s.Insert(ctx, identifier.New(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	version, errE := changeset.Insert(ctx, newID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "unknown")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Commit(ctx, v, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = v.Count(ctx, false)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Insert(ctx, identifier.New(), testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Update(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Merge(
		ctx, newID, []identifier.Identifier{version.Changeset}, testutils.DummyData, []json.RawMessage{testutils.DummyData}, testutils.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Replace(ctx, newID, version.Changeset, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Delete(ctx, newID, version.Changeset, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	// The number of parent changesets have to match the number of patches.
	_, errE = s.Merge(ctx, newID, []identifier.Identifier{version.Changeset}, testutils.DummyData, nil, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{identifier.New()},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{anotherVersion.Changeset},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Replace(ctx, newID, identifier.New(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Replace(ctx, newID, anotherVersion.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Delete(ctx, newID, identifier.New(), testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Delete(ctx, newID, anotherVersion.Changeset, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	changeset, errE = s.Changeset(ctx, identifier.New())
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Changes(ctx, nil)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

func TestParallelChange(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	firstID := identifier.New()
	secondID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, firstID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, secondID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Update(ctx, firstID, changeset.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, secondID, changeset.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No error because both changesets are changing different values from the same parent changeset.

	_, errE = s.Commit(ctx, changeset1, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset2, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestCommittedOrdering(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	const n = 10
	for range n {
		id := identifier.New()
		_, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	require.Eventually(t, func() bool { return channelContents.Len() >= n }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	require.Len(t, c, n)

	// Check that seq numbers are positive and strictly increasing.
	for i := range c {
		assert.Positive(t, c[i].Seq, "seq at index %d should be positive", i)
	}
	for i := 1; i < len(c); i++ {
		assert.Greater(t, c[i].Seq, c[i-1].Seq, "seq at index %d should be greater than seq at index %d", i, i-1)
	}
}

func TestCommittedSeqSameForCommit(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Prepare a chain: first insert two values in separate changesets,
	// then commit only the second, which also commits the first.
	firstID := identifier.New()
	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Insert(ctx, firstID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	secondID := identifier.New()
	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, firstID, changeset1.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Insert(ctx, secondID, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Committing changeset2 also commits changeset1 (its uncommitted ancestor).
	committed, errE := s.Commit(ctx, changeset2, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, committed, 2)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	// One CommittedChangesets per commit: the commit contains both changesets.
	require.Len(t, c, 1)
	assert.Positive(t, c[0].Seq)
	assert.Equal(t, store.MainView, c[0].View.Name())
	assert.Len(t, c[0].Changesets, 2)
}

func TestCommitLog(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Empty log initially.
	entries, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, entries)

	// Make two separate commits.
	id1 := identifier.New()
	v1, errE := s.Insert(ctx, id1, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	id2 := identifier.New()
	v2, errE := s.Insert(ctx, id2, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Get all entries.
	entries, errE = s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, entries, 2) {
		// Entries are in increasing seq order.
		assert.Positive(t, entries[0].Seq)
		assert.Greater(t, entries[1].Seq, entries[0].Seq)

		// Both committed to the main view.
		assert.Equal(t, store.MainView, entries[0].View.Name())
		assert.Equal(t, store.MainView, entries[1].View.Name())

		// Each commit contains exactly one changeset.
		if assert.Len(t, entries[0].Changesets, 1) {
			assert.Equal(t, v1.Changeset, entries[0].Changesets[0].ID())
		}
		if assert.Len(t, entries[1].Changesets, 1) {
			assert.Equal(t, v2.Changeset, entries[1].Changesets[0].ID())
		}
	}

	// Pagination: entries after first seq returns only second.
	page2, errE := s.CommitLog(ctx, &entries[0].Seq, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, page2, 1) {
		assert.Equal(t, entries[1].Seq, page2[0].Seq)
		assert.Equal(t, v2.Changeset, page2[0].Changesets[0].ID())
	}

	// After last seq returns empty.
	page3, errE := s.CommitLog(ctx, &entries[1].Seq, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page3)

	// After a non-existent seq also returns empty (no error).
	unknown := entries[1].Seq + 1000
	page4, errE := s.CommitLog(ctx, &unknown, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page4)

	// Commit multiple changesets and verify they share one entry.
	cs, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	id3 := identifier.New()
	_, errE = cs.Insert(ctx, id3, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	id4 := identifier.New()
	_, errE = cs.Insert(ctx, id4, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Commit(ctx, cs, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	all, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, all, 3) {
		assert.Equal(t, entries[0].Seq, all[0].Seq)
		assert.Equal(t, entries[1].Seq, all[1].Seq)
		// Third entry has one changeset containing two values.
		if assert.Len(t, all[2].Changesets, 1) {
			assert.Equal(t, cs.ID(), all[2].Changesets[0].ID())
		}
	}
}

func TestCommitLogViewFilter(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Commit to main view.
	idMain := identifier.New()
	vMain, errE := s.Insert(ctx, idMain, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Create a child view and commit to it.
	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)
	childView, errE := mainView.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	idChild := identifier.New()
	vChild, errE := childView.Insert(ctx, idChild, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// All commits visible without filter.
	all, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, all, 2)
	assert.Equal(t, store.MainView, all[0].View.Name())
	assert.Equal(t, "child", all[1].View.Name())

	// Filter by "main" returns only the main commit.
	mainName := store.MainView
	mainEntries, errE := s.CommitLog(ctx, nil, &mainName)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, mainEntries, 1) {
		assert.Equal(t, store.MainView, mainEntries[0].View.Name())
		assert.Equal(t, vMain.Changeset, mainEntries[0].Changesets[0].ID())
	}

	// Filter by "child" returns only the child commit.
	childName := "child"
	childEntries, errE := s.CommitLog(ctx, nil, &childName)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, childEntries, 1) {
		assert.Equal(t, "child", childEntries[0].View.Name())
		assert.Equal(t, vChild.Changeset, childEntries[0].Changesets[0].ID())
	}

	// Release the "child" name - the view is now unnamed.
	errE = childView.Release(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// After release, filtering by "child" returns nothing (no view currently has that name).
	afterRelease, errE := s.CommitLog(ctx, nil, &childName)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, afterRelease)

	// Unfiltered result still has both entries, but the child commit now has an empty view name
	// because the current view name is NULL (the view was released).
	all2, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, all2, 2) {
		assert.Equal(t, store.MainView, all2[0].View.Name())
		// The view was released so its current name is empty.
		assert.Empty(t, all2[1].View.Name())
	}

	// Re-register the "child" name on a brand-new view (simulates a rename to a new view).
	newChildView, errE := mainView.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Commit to the new "child" view.
	idChild2 := identifier.New()
	vChild2, errE := newChildView.Insert(ctx, idChild2, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Filtering by "child" now returns only the new commit - old commits are still unnamed.
	renamed, errE := s.CommitLog(ctx, nil, &childName)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, renamed, 1) {
		assert.Equal(t, "child", renamed[0].View.Name())
		assert.Equal(t, vChild2.Changeset, renamed[0].Changesets[0].ID())
	}
}

func TestNotifyRecovery(t *testing.T) {
	t.Parallel()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	schema := "s" + strings.ToLower(identifier.New().String())
	prefix := identifier.New().String() + "_"

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

	s := &store.Store[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{
		Prefix:        prefix,
		CommittedSize: 1,
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
	}

	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = listener.Start(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert an initial document to confirm the channel is working.
	id1 := identifier.New()
	_, errE = s.Insert(ctx, id1, json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ch, errE := s.Committed.Get(ctx)
		require.NoError(t, errE, "% -+#.1v", errE)
		select {
		case <-ch:
		default:
			assert.Fail(c, "commit notification not yet received")
		}
	}, 5*time.Second, 10*time.Millisecond)

	// Save the current channel before simulating a listener reconnection.
	oldCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Simulate a listener reconnection by calling HandleBacklog directly.
	// In production this is triggered when pgxlisten reconnects after a connection drop.
	// It should close the old channel (signaling consumers that notifications may have been
	// missed) and create a new one.
	err := s.HandleBacklog(ctx, s.Schema+"_"+s.Prefix+"Commit", nil)
	require.NoError(t, errE, "% -+#.1v", err) // This is still errors.E.

	// Old channel must be closed so that consumers know to take corrective action.
	select {
	case _, ok := <-oldCh:
		require.False(t, ok, "old channel should be closed after HandleBacklog")
	case <-time.After(time.Second):
		t.Fatal("old channel was not closed by HandleBacklog")
	}

	// A new channel must be created.
	newCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEqual(t, oldCh, newCh, "HandleBacklog should create a new channel")

	// Commits after the reconnection must arrive on the new channel.
	id2 := identifier.New()
	_, errE = s.Insert(ctx, id2, json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case <-newCh:
		default:
			assert.Fail(c, "commit notification not yet received on new channel")
		}
	}, 5*time.Second, 10*time.Millisecond)
}

func TestUpdateExistingMetadata(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	insertData := json.RawMessage(`{"data": "original"}`)
	insertMetadata := json.RawMessage(`{"meta": "v1"}`)

	insertVersion, errE := s.Insert(ctx, id, insertData, insertMetadata, insertMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), insertVersion.Revision)

	// Update metadata only.
	newMetadata := json.RawMessage(`{"meta": "v2"}`)
	newVersion, errE := s.UpdateExistingMetadata(ctx, id, insertVersion, newMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), newVersion.Revision)
	assert.Equal(t, insertVersion.Changeset, newVersion.Changeset)

	// GetLatest should return updated metadata but same data.
	data, metadata, version, parentChangesets, errE := s.GetLatest(ctx, id)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newVersion, version)
	assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
	assert.Equal(t, json.RawMessage(`{"meta": "v2"}`), metadata)   //nolint:testifylint
	assert.Empty(t, parentChangesets)

	// Old version should still have original metadata.
	data, metadata, resolvedVersion, parentChangesets, errE := s.Get(ctx, id, insertVersion)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
	assert.Equal(t, json.RawMessage(`{"meta": "v1"}`), metadata)   //nolint:testifylint
	assert.Equal(t, insertVersion, resolvedVersion)
	assert.Empty(t, parentChangesets)
}

func TestGetRevisionZero(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	insertData := json.RawMessage(`{"data": "original"}`)
	insertMetadata := json.RawMessage(`{"meta": "v1"}`)

	insertVersion, errE := s.Insert(ctx, id, insertData, insertMetadata, insertMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), insertVersion.Revision)

	// Get with Revision 0 should return the latest (and only) revision.
	data, metadata, resolvedVersion, parentChangesets, errE := s.Get(ctx, id, store.Version{
		Changeset: insertVersion.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "v1"}`), metadata)   //nolint:testifylint
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Update metadata to create revision 2 on the same changeset.
	newMetadata := json.RawMessage(`{"meta": "v2"}`)
	newVersion, errE := s.UpdateExistingMetadata(ctx, id, insertVersion, newMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), newVersion.Revision)
	assert.Equal(t, insertVersion.Changeset, newVersion.Changeset)

	// Get with Revision 0 should now return revision 2 (the latest for this changeset).
	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, store.Version{
		Changeset: insertVersion.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "v2"}`), metadata)   //nolint:testifylint
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Get with explicit Revision 1 should still return the old metadata.
	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "v1"}`), metadata)   //nolint:testifylint
		assert.Equal(t, insertVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Get with explicit Revision 2 should return the new metadata.
	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "v2"}`), metadata)   //nolint:testifylint
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Update the value (creates a new changeset).
	updateData := json.RawMessage(`{"data": "updated"}`)
	updateMetadata := json.RawMessage(`{"meta": "u1"}`)
	updateVersion, errE := s.Update(ctx, id, newVersion.Changeset, updateData, json.RawMessage(`{}`), updateMetadata, updateMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), updateVersion.Revision)

	// Get with Revision 0 on the new changeset should return it.
	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, store.Version{
		Changeset: updateVersion.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "updated"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "u1"}`), metadata)  //nolint:testifylint
		assert.Equal(t, updateVersion, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: newVersion.Changeset, Revision: 0}}, parentChangesets)
	}

	// Get with Revision 0 on the old changeset should still return the latest revision of that changeset.
	data, metadata, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, store.Version{
		Changeset: insertVersion.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, json.RawMessage(`{"data": "original"}`), data) //nolint:testifylint
		assert.Equal(t, json.RawMessage(`{"meta": "v2"}`), metadata)   //nolint:testifylint
		assert.Equal(t, newVersion, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Get with Revision 0 on a non-existent changeset should return ErrValueNotFound.
	_, _, _, _, errE = s.Get(ctx, id, store.Version{ //nolint:dogsled
		Changeset: identifier.New(),
		Revision:  0,
	})
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestGetRevisionZeroView(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()

	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Get with Revision 0 on the child view should return the updated version.
	_, _, resolvedVersion, parentChangesets, errE := v.Get(ctx, id, store.Version{
		Changeset: updated.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, updated, resolvedVersion)
		assert.Equal(t, []store.Version{{Changeset: version.Changeset, Revision: 0}}, parentChangesets)
	}

	// Get with Revision 0 on the main view should not find the child's changeset.
	_, _, _, _, errE = s.Get(ctx, id, store.Version{ //nolint:dogsled
		Changeset: updated.Changeset,
		Revision:  0,
	})
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Get with Revision 0 on the original changeset should work on both views.
	_, _, resolvedVersion, parentChangesets, errE = s.Get(ctx, id, store.Version{
		Changeset: version.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, version, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	_, _, resolvedVersion, parentChangesets, errE = v.Get(ctx, id, store.Version{
		Changeset: version.Changeset,
		Revision:  0,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, version, resolvedVersion)
		assert.Empty(t, parentChangesets)
	}

	// Get with Revision 0 on a non-existent view should return ErrViewNotFound.
	notExist, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)
	_, _, _, _, errE = notExist.Get(ctx, id, store.Version{ //nolint:dogsled
		Changeset: version.Changeset,
		Revision:  0,
	})
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

func TestUpdateExistingMetadataRevisionMismatch(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	insertVersion, errE := s.Insert(ctx, id, json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	// Update to create revision 2.
	v2, errE := s.UpdateExistingMetadata(ctx, id, insertVersion, json.RawMessage(`{"meta": "v2"}`))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), v2.Revision)

	// Try to update using the old revision - should fail with revision mismatch.
	_, errE = s.UpdateExistingMetadata(ctx, id, insertVersion, json.RawMessage(`{"meta": "v3"}`))
	assert.ErrorIs(t, errE, store.ErrRevisionMismatch)
}

func TestUpdateExistingMetadataChangesetNotFound(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Try to update a non-existent changeset.
	fakeVersion := store.Version{
		Changeset: identifier.New(),
		Revision:  1,
	}
	_, errE := s.UpdateExistingMetadata(ctx, identifier.New(), fakeVersion, json.RawMessage(`{}`))
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

func TestUpdateExistingMetadataNonExistentRevision(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	insertVersion, errE := s.Insert(ctx, id, json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	// Use the correct changeset but a revision that doesn't exist in the Changes table.
	// The outer SELECT's WHERE clause matches 0 rows, so the stored function is never called.
	// Without the RowsAffected check this would silently succeed.
	nonExistentVersion := store.Version{
		Changeset: insertVersion.Changeset,
		Revision:  99,
	}
	_, errE = s.UpdateExistingMetadata(ctx, id, nonExistentVersion, json.RawMessage(`{"meta": "new"}`))
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

// TestCountViewNotFound covers Count on a view name that does not exist.
// All other view read operations return ErrViewNotFound in this case; Count
// should too.
func TestCountViewNotFound(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	v, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Count(ctx, false)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = v.Count(ctx, true)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

// TestCountAcrossViewsWithDeletions exercises the multi-view + delete
// interactions of Count. Each scenario maps directly to a corner of the
// closest-view-in-path resolution that GetLatest uses.
func TestCountAcrossViewsWithDeletions(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	child, errE := mainView.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Scenario 1: alive in main, deleted in child.
	idA := identifier.New()
	vA, errE := s.Insert(ctx, idA, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = child.Delete(ctx, idA, vA.Changeset, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainCount, errE := s.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), mainCount, "main counts alive A")

	childCount, errE := child.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), childCount, "child shadows main with delete")

	// Scenario 2: deleted in main, the SAME id is then re-introduced into the
	// child by committing a new update from the parent of the delete.
	// (We piggy-back on the same idA since main already has it.)
	deleteMainV, errE := s.Delete(ctx, idA, vA.Changeset, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainCount, errE = s.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), mainCount, "main now sees A as deleted")

	// child's delete still shadows for the child view.
	childCount, errE = child.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), childCount, "child still sees A as deleted")

	// Scenario 3: insert a fresh id and override the main version in the child.
	idB := identifier.New()
	vB, errE := s.Insert(ctx, idB, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = child.Update(ctx, idB, vB.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainCount, errE = s.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), mainCount, "main counts B (A is deleted)")

	childCount, errE = child.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), childCount, "child counts B (A is deleted; B shadowed)")

	// With includeDeleted=true Count must match the set of ids returned by List
	// for the same view, regardless of which versions have been deleted.
	mainList, errE := s.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	mainCount, errE = s.Count(ctx, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(len(mainList)), mainCount, "main Count(true) matches List")

	childList, errE := child.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	childCount, errE = child.Count(ctx, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(len(childList)), childCount, "child Count(true) matches List")

	// Suppress unused variable for clarity.
	_ = deleteMainV
}

// TestListIncludesDeletedCountIncludeDeletedFlag locks in the documented
// contract between List and Count: List always returns ids even after their
// latest version is deleted; Count(ctx, false) filters them out, while
// Count(ctx, true) matches List by including deleted ids.
func TestListIncludesDeletedCountIncludeDeletedFlag(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Delete(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	list, errE := s.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Contains(t, list, id, "List includes deleted id")

	count, errE := s.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), count, "Count(includeDeleted=false) excludes deleted id")

	countWithDeleted, errE := s.Count(ctx, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(len(list)), countWithDeleted, "Count(includeDeleted=true) matches List")
	assert.Equal(t, int64(1), countWithDeleted, "Count(includeDeleted=true) includes deleted id")
}

// TestStaleViewAfterRelease confirms that an in-memory View object that has
// since been released is correctly recognized as no longer existing.
func TestStaleViewAfterRelease(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	child, errE := mainView.Create(ctx, "child", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Insert via the child view so there is data to query.
	id := identifier.New()
	version, errE := child.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = child.Release(ctx, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// All read operations on the stale View should return ErrViewNotFound.
	_, _, _, _, errE = child.GetLatest(ctx, id) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, _, _, _, errE = child.Get(ctx, id, version) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = child.List(ctx, nil)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = child.Count(ctx, false)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = child.Changes(ctx, id, nil)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	// Releasing an already-released name should also fail.
	errE = child.Release(ctx, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	// Creating a sub-view of a released view should fail too.
	_, errE = child.Create(ctx, "grandchild", testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

// TestInsertAfterDeleteConflict documents how the store handles an attempt to
// "resurrect" a deleted id via a fresh Insert (which has no parent changeset
// and therefore conflicts with the existing depth=0 delete row).
func TestInsertAfterDeleteConflict(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()

	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	deleteVersion, errE := s.Delete(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A bare Insert has no parent and cannot legally produce a new depth=0
	// for an id that already has a depth=0 (currently deleted) row.
	_, errE = s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// The supported "resurrection" path is to Replace from the delete changeset.
	resurrected, errE := s.Replace(ctx, id, deleteVersion.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// And the value is now alive again.
	_, _, latest, _, errE := s.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, resurrected, latest)
}

// TestConcurrentCommitConflict exercises the EXCLUDE-constraint path: two
// concurrent commits both try to introduce a new depth=0 version for the same
// value from the same parent. Exactly one should succeed; the other should
// either succeed (if PostgreSQL serialized them as if sequential, which the
// constraint then rejects via a unique/exclusion violation surfaced as
// ErrConflict) or fail with ErrConflict.
func TestConcurrentCommitConflict(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Two independent uncommitted changesets, both updating the same value
	// from the same parent (so they are concurrent branches).
	csA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csA.Update(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	csB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csB.Update(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errs   []errors.E
		okWins int
	)
	commit := func(cs store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]) {
		_, errE := s.Commit(ctx, cs, testutils.DummyData)
		mu.Lock()
		defer mu.Unlock()
		if errE == nil {
			okWins++
		} else {
			errs = append(errs, errE)
		}
	}
	wg.Go(func() { commit(csA) })
	wg.Go(func() { commit(csB) })
	wg.Wait()

	assert.Equal(t, 1, okWins, "exactly one concurrent commit should succeed")
	if assert.Len(t, errs, 1) {
		assert.ErrorIs(t, errs[0], store.ErrConflict, "loser sees ErrConflict; got % -+#.1v", errs[0])
	}
}

// TestConcurrentUpdateExistingMetadata: two goroutines try to bump the same
// changeset+id metadata from revision 1. SSI + the explicit revision-mismatch
// check should ensure exactly one succeeds and the other observes the new
// revision or ErrRevisionMismatch.
func TestConcurrentUpdateExistingMetadata(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	version, errE := s.Insert(ctx, id, json.RawMessage(`{}`), json.RawMessage(`{"meta":"v1"}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		successes       int
		mismatchOrRetry int
	)
	bump := func(label string) {
		_, errE := s.UpdateExistingMetadata(ctx, id, version, json.RawMessage(`{"meta": "`+label+`"}`))
		mu.Lock()
		defer mu.Unlock()
		switch {
		case errE == nil:
			successes++
		case errors.Is(errE, store.ErrRevisionMismatch):
			mismatchOrRetry++
		default:
			t.Errorf("unexpected error from concurrent UpdateExistingMetadata: % -+#.1v", errE)
		}
	}
	wg.Go(func() { bump("a") })
	wg.Go(func() { bump("b") })
	wg.Wait()

	// Exactly one race winner. The other gets ErrRevisionMismatch after seeing
	// the bumped revision (or after SSI-retry detects the new revision).
	assert.Equal(t, 1, successes, "exactly one UpdateExistingMetadata succeeds")
	assert.Equal(t, 1, mismatchOrRetry, "loser gets ErrRevisionMismatch")

	// Final state: revision is 2.
	_, _, latest, _, errE := s.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(2), latest.Revision)
}

// TestDeepViewHierarchy validates path resolution through 4 view levels
// (main -> v1 -> v2 -> v3), including shadowing and deletion semantics across
// the chain.
func TestDeepViewHierarchy(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)
	v1, errE := mainView.Create(ctx, "v1", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	v2, errE := v1.Create(ctx, "v2", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	v3, errE := v2.Create(ctx, "v3", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Commit X only to main.
	id := identifier.New()
	versionMain, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// All four views see X (resolution through the path).
	for _, v := range []store.View[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{mainView, v1, v2, v3} {
		_, _, version, _, errE := v.GetLatest(ctx, id)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, versionMain, version, "view %s resolves to main", v.Name())

		count, errE := v.Count(ctx, false)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, int64(1), count, "view %s Count=1", v.Name())
	}

	// Override X in v2.
	versionV2, errE := v2.Update(ctx, id, versionMain.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// v3 now sees v2's version (v2 is the closest view in v3's path with X).
	_, _, gotV3, _, errE := v3.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionV2, gotV3, "v3 shadows main via v2")

	// v1 still sees main's version because v2 is NOT in v1's path.
	_, _, gotV1, _, errE := v1.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionMain, gotV1, "v1 unaffected by v2")

	// main is untouched.
	_, _, gotMain, _, errE := mainView.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionMain, gotMain)

	// Delete X in v1.
	_, errE = v1.Delete(ctx, id, versionMain.Changeset, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// v1 sees X as deleted (shadows main).
	_, _, _, _, errE = v1.GetLatest(ctx, id) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	countV1, errE := v1.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(0), countV1)

	// main still sees X alive.
	countMain, errE := mainView.Count(ctx, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), countMain)

	// v3 still sees v2's version (v2 closer than v1 in v3's path).
	_, _, gotV3, _, errE = v3.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionV2, gotV3, "v3 sees v2's version, not v1's delete")
}

// TestConcurrentCreateSameName: N goroutines simultaneously creating views
// with the same name -> exactly one succeeds, the rest get ErrConflict.
func TestConcurrentCreateSameName(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	const n = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		conflicts int
		other     []errors.E
	)
	for range n {
		wg.Go(func() {
			_, errE := mainView.Create(ctx, "racey", testutils.DummyData)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errE == nil:
				successes++
			case errors.Is(errE, store.ErrConflict):
				conflicts++
			default:
				other = append(other, errE)
			}
		})
	}
	wg.Wait()

	assert.Empty(t, other, "no unexpected errors")
	assert.Equal(t, 1, successes, "exactly one Create wins")
	assert.Equal(t, n-1, conflicts, "all losers see ErrConflict")
}

// TestNotificationLargePayloadFallback exercises the >7900-byte NOTIFY payload
// fallback path: when the trigger detects the payload is too large it sends
// only {"seq":N} and the handler fetches the full set from CommitLog.
//
// We construct a commit large enough that the inline payload would exceed the
// threshold, then verify the consumer still receives all changesets and the
// correct view name.
func TestNotificationLargePayloadFallback(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Drain the small initial notification.
	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	_ = channelContents.Prune()

	// Build a chain of ~320 uncommitted update changesets. Each identifier is
	// 22 characters; JSON-encoded with quotes+comma each contributes ~25 bytes,
	// so >320 changesets pushes the inline payload past the 7900-byte limit.
	const chainLen = 320
	var cs store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]
	for i := range chainLen {
		cs, errE = s.Begin(ctx)
		require.NoError(t, errE, "% -+#.1v", errE)
		version, errE = cs.Update(ctx, id, version.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)
	}

	// Committing the last changeset in the chain also commits all of its
	// uncommitted ancestors in one transaction.
	committed, errE := s.Commit(ctx, cs, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, committed, chainLen)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 10*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	require.Len(t, c, 1, "exactly one CommittedChangesets received")
	assert.Equal(t, store.MainView, c[0].View.Name(), "view name preserved via CommitLog fallback")
	assert.Len(t, c[0].Changesets, chainLen, "all changesets present after fallback fetch")
	assert.Positive(t, c[0].Seq)
}

// TestNotificationFallbackFailureCallsReset confirms that when the >7900-byte
// fallback DB query fails, the handler signals consumers via Reset() (a closed
// channel) so they know they must resync via the CommitLog API.
func TestNotificationFallbackFailureCallsReset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	oldCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Craft a notification that forces the fallback path (no "changesets" key)
	// with a seq that does not exist in CommitLog. The QueryRow Scan will
	// return pgx.ErrNoRows which the handler treats as a failure and calls
	// Reset() before propagating.
	notification := &pgconn.Notification{
		PID:     0,
		Channel: s.Schema + "_" + s.Prefix + "Commit",
		Payload: `{"seq":999999999}`,
	}
	err := s.HandleNotification(ctx, notification, nil)
	require.Error(t, err)

	// Old channel must be closed by the Reset() invocation.
	select {
	case _, ok := <-oldCh:
		require.False(t, ok, "old channel should be closed after fallback failure")
	case <-time.After(2 * time.Second):
		t.Fatal("old channel was not closed by fallback-failure Reset()")
	}

	// A new channel must be available.
	newCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEqual(t, oldCh, newCh, "Reset() created a new channel")
}

// TestUpdateExistingMetadataOnCommittedChangeset verifies that metadata can
// be updated on an already-committed changeset and that doing so bumps the
// revision; the changeset must remain committed (still cannot be discarded).
func TestUpdateExistingMetadataOnCommittedChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	// Use the jsonb canonical formatting (space after colon) so direct
	// json.RawMessage comparisons work after a round-trip through the column.
	insertVersion, errE := s.Insert(ctx, id, json.RawMessage(`{"data": "d"}`), json.RawMessage(`{"meta": "m1"}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, int64(1), insertVersion.Revision)

	// Insert auto-commits, so the changeset is committed.
	cs, errE := s.Changeset(ctx, insertVersion.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	errE = cs.Discard(ctx)
	require.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	// Bump metadata. Revision must increment.
	newVersion, errE := s.UpdateExistingMetadata(ctx, id, insertVersion, json.RawMessage(`{"meta": "m2"}`))
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, insertVersion.Changeset, newVersion.Changeset)
	assert.Equal(t, int64(2), newVersion.Revision)

	// Latest reflects the bumped revision and new metadata.
	_, metadata, latest, _, errE := s.GetLatest(ctx, id)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, newVersion, latest)
	assert.Equal(t, json.RawMessage(`{"meta": "m2"}`), metadata) //nolint:testifylint

	// Old revision is still retrievable with its original metadata.
	_, oldMetadata, resolved, _, errE := s.Get(ctx, id, insertVersion)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, insertVersion, resolved)
	assert.Equal(t, json.RawMessage(`{"meta": "m1"}`), oldMetadata) //nolint:testifylint

	// The changeset stays committed.
	errE = cs.Discard(ctx)
	require.ErrorIs(t, errE, store.ErrAlreadyCommitted)
}

// TestChangesPaginationDepthBoundary verifies pagination correctness when the
// after-cursor lands at the last changeset of one depth and the next page must
// begin with the first changeset of the next depth.
//
// Graph constructed:
//
//	insert (depth 2) ─┬─ cA (depth 1) ─┐
//	                  └─ cB (depth 1) ─┴─ merged (depth 0)
//
// Sort order inside Changes(): depth ASC, then changeset ASC. So:
//
//	[merged, sortIDs(cA,cB)[0], sortIDs(cA,cB)[1], insert]
//
// Pagination after the last of depth 1 should return just [insert].
func TestChangesPaginationDepthBoundary(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	insert, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	csA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csA.Update(ctx, id, insert.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	csB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csB.Update(ctx, id, insert.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(
		ctx, id,
		[]identifier.Identifier{csA.ID(), csB.ID()},
		testutils.DummyData,
		[]json.RawMessage{testutils.DummyData, testutils.DummyData},
		testutils.DummyData,
		testutils.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	sortedAB := sortIDs(csA.ID(), csB.ID())

	// Full listing in graph-traversal order: depth 0, depth 1 sorted, depth 2.
	all, errE := s.Changes(ctx, id, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, []identifier.Identifier{
		merged.Changeset,
		sortedAB[0],
		sortedAB[1],
		insert.Changeset,
	}, all)

	// After the *first* depth-1 changeset, we expect the second depth-1 then
	// the depth-2 insert.
	after0 := sortedAB[0]
	page, errE := s.Changes(ctx, id, &after0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{sortedAB[1], insert.Changeset}, page)

	// After the LAST depth-1 changeset, only the depth-2 insert remains.
	// This is the depth-boundary crossing: the WHERE clause must move to the
	// "distinctChangesets.depth > changesetDepth.depth" branch.
	after1 := sortedAB[1]
	page, errE = s.Changes(ctx, id, &after1)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{insert.Changeset}, page)

	// After the depth-2 insert there is nothing.
	page, errE = s.Changes(ctx, id, &insert.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, page)
}

// TestDiscardChain validates the in-use chain enforcement: a parent cannot be
// discarded while any descendant exists; descendants must be discarded first
// (in reverse dependency order).
func TestDiscardChain(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()

	// c1 (uncommitted) inserts X.
	c1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = c1.Insert(ctx, id, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// c2 (uncommitted) updates X from c1.
	c2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = c2.Update(ctx, id, c1.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// c3 (uncommitted) updates X from c2.
	c3, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = c3.Update(ctx, id, c2.ID(), testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// In-order discard fails because each has a descendant.
	errE = c1.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrInUse)
	errE = c2.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrInUse)

	// Discard in reverse dependency order.
	require.NoError(t, c3.Discard(ctx), "% -+#.1v", c3.Discard(ctx))
	require.NoError(t, c2.Discard(ctx), "% -+#.1v", c2.Discard(ctx))
	require.NoError(t, c1.Discard(ctx), "% -+#.1v", c1.Discard(ctx))

	// A second Discard of an already-discarded leaf is a no-op (not an error).
	require.NoError(t, c1.Discard(ctx), "% -+#.1v", c1.Discard(ctx))
}

// TestMergeWithPatchesDisabled: with Patch=store.None the patches parameter is
// ignored entirely; the length check between parentChangesets and patches is
// skipped, and nil patches are accepted.
func TestMergeWithPatchesDisabled(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[[]byte, []byte, []byte, []byte, []byte, store.None](t, "bytea")

	id := identifier.New()
	insert, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	csA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csA.Update(ctx, id, insert.Changeset, testutils.DummyData, nil, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	csB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = csB.Update(ctx, id, insert.Changeset, testutils.DummyData, nil, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// nil patches accepted because patches are disabled.
	merged, errE := s.Merge(
		ctx, id,
		[]identifier.Identifier{csA.ID(), csB.ID()},
		testutils.DummyData,
		nil,
		testutils.DummyData,
		testutils.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, latest, _, errE := s.GetLatest(ctx, id) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
}

// TestWithStoreIdempotent: WithStore on a View/Changeset/CommittedChangesets
// returns a functionally equivalent object when the store is already set.
func TestWithStoreIdempotent(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView2, errE := mainView.WithStore(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, mainView.Name(), mainView2.Name())

	id := identifier.New()
	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	cs, errE := s.Changeset(ctx, version.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	cs2, errE := cs.WithStore(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, cs.ID(), cs2.ID())

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	require.Len(t, c, 1)

	committed := c[0]
	require.Nil(t, committed.View.Store(), "received CommittedChangesets has nil store")

	withStore, errE := committed.WithStore(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, committed.Seq, withStore.Seq)
	require.Len(t, withStore.Changesets, len(committed.Changesets))
	for i := range committed.Changesets {
		assert.Equal(t, committed.Changesets[i].ID(), withStore.Changesets[i].ID())
		assert.NotNil(t, withStore.Changesets[i].Store(), "rehydrated changeset has store")
	}
	assert.Equal(t, committed.View.Name(), withStore.View.Name())
	assert.NotNil(t, withStore.View.Store(), "rehydrated view has store")

	// Idempotent: calling WithStore on the rehydrated value returns the same shape.
	again, errE := withStore.WithStore(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, withStore.Seq, again.Seq)
	assert.Len(t, again.Changesets, len(withStore.Changesets))
}

// TestResetFromUserCode: calling Reset() directly from user code closes the
// current channel (signaling consumers) and provisions a new one. Subsequent
// commits arrive on the new channel and are forwarded to the consumer running
// in initDatabase.
func TestResetFromUserCode(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	oldCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	s.Reset()

	// The previous channel must be closed so any holder of it (a slow consumer,
	// for instance) can detect the gap.
	select {
	case _, ok := <-oldCh:
		require.False(t, ok, "old channel should be closed after Reset()")
	case <-time.After(2 * time.Second):
		t.Fatal("old channel was not closed by Reset()")
	}

	// A new channel is now installed.
	newCh, errE := s.Committed.Get(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEqual(t, oldCh, newCh, "Reset() created a new channel")

	// Commits after Reset arrive on the new channel. The consumer goroutine
	// from initDatabase reads from the live channel and records the commit
	// into channelContents. We observe via that side rather than racing it
	// for the receive.
	id := identifier.New()
	_, errE = s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	require.Eventually(t, func() bool { return channelContents.Len() >= 1 }, 5*time.Second, 10*time.Millisecond)
	c := channelContents.Prune()
	require.Len(t, c, 1)
	assert.Equal(t, store.MainView, c[0].View.Name())
}

// TestReadOnlyRejectsWrites pins the contract that read-paths in the store
// use pgx.ReadOnly transactions and that PostgreSQL enforces it.
func TestReadOnlyRejectsWrites(t *testing.T) {
	t.Parallel()

	ctx, _, _, dbpool := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// We exercise the same RetryTransaction wrapper the store uses, with the
	// ReadOnly access mode, and try a write. PostgreSQL must reject it.
	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// CREATE TEMP TABLE is a write and is disallowed in a read-only transaction.
		_, err := tx.Exec(ctx, `CREATE TEMP TABLE rotest(id int)`)
		return internalStore.WithPgxError(err)
	})
	require.Error(t, errE)
	pgError, ok := errors.AsType[*pgconn.PgError](errE)
	require.True(t, ok, "expected pg error, got % -+#.1v", errE)
	// "25006" = read_only_sql_transaction.
	assert.Equal(t, "25006", pgError.Code, "PostgreSQL rejects writes in ReadOnly transactions")
}

// TestChangesetViewsAfterMainViewInsert verifies that Changeset.Views returns
// the MainView for a changeset that was committed there via Store.Insert, and
// that CommittedChangeset.Metadata returns the same commit metadata that was
// passed in.
func TestChangesetViewsAfterMainViewInsert(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	commitMeta := json.RawMessage(`{"who":"tester"}`)
	id := identifier.New()
	version, errE := s.Insert(ctx, id, testutils.DummyData, testutils.DummyData, commitMeta)
	require.NoError(t, errE, "% -+#.1v", errE)

	cs, errE := s.Changeset(ctx, version.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)

	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, views, 1, "changeset should be committed to exactly one view")
	assert.Equal(t, store.MainView, views[0].View().Name())
	assert.Equal(t, cs.ID(), views[0].ID(), "CommittedChangeset preserves the changeset id")
	assert.NotNil(t, views[0].View().Store(), "returned View carries the changeset's store")

	gotMeta, errE := views[0].Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, string(commitMeta), string(gotMeta))
}

// TestChangesetViewsUncommitted verifies that Views returns an empty slice
// (with no error) for a changeset that has not been committed anywhere.
func TestChangesetViewsUncommitted(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Begin a fresh changeset and put a change in it, but never commit.
	cs, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = cs.Insert(ctx, identifier.New(), testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, views, "uncommitted changeset has no committed views")
}

// TestChangesetViewsAcrossMultipleViews verifies that Views returns one
// CommittedChangeset per view when the same changeset is committed to multiple
// views, sorted by view name, each carrying its own commit metadata.
func TestChangesetViewsAcrossMultipleViews(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Set up an ancestor commit that subordinate views can branch from.
	parentID := identifier.New()
	parentVersion, errE := s.Insert(ctx, parentID, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)
	// Two sibling child views off MainView. We pick names so the ORDER BY in Views is observable.
	viewA, errE := mainView.Create(ctx, "a-view", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	viewB, errE := mainView.Create(ctx, "b-view", testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Build a fresh changeset that updates parentID; do NOT commit via the
	// convenience Update on a view (which auto-commits). Instead, prepare the
	// changeset and commit explicitly to two views with different metadata.
	cs, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = cs.Update(ctx, parentID, parentVersion.Changeset, testutils.DummyData, testutils.DummyData, testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	metaA := json.RawMessage(`{"view":"a"}`)
	metaB := json.RawMessage(`{"view":"b"}`)
	_, errE = cs.Commit(ctx, viewA, metaA)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = cs.Commit(ctx, viewB, metaB)
	require.NoError(t, errE, "% -+#.1v", errE)

	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, views, 2)
	// SQL orders by view name ascending.
	assert.Equal(t, "a-view", views[0].View().Name())
	assert.Equal(t, "b-view", views[1].View().Name())

	gotA, errE := views[0].Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, string(metaA), string(gotA))

	gotB, errE := views[1].Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, string(metaB), string(gotB))
}

// TestCommittedChangesetWithStore verifies that CommittedChangeset.WithStore
// rehydrates the store association while preserving the changeset id and view.
// Metadata is fetched lazily and remains accessible through the new store.
func TestCommittedChangesetWithStore(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	commitMeta := json.RawMessage(`{"hello":"world"}`)
	version, errE := s.Insert(ctx, identifier.New(), testutils.DummyData, testutils.DummyData, commitMeta)
	require.NoError(t, errE, "% -+#.1v", errE)

	cs, errE := s.Changeset(ctx, version.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	views, errE := cs.Views(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, views, 1)

	cc := views[0]
	rehydrated, errE := cc.WithStore(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, cc.ID(), rehydrated.ID())
	assert.Equal(t, cc.View().Name(), rehydrated.View().Name())
	require.NotNil(t, rehydrated.Store())

	// Metadata is still retrievable via the rehydrated CommittedChangeset.
	gotMeta, errE := rehydrated.Metadata(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, string(commitMeta), string(gotMeta))
}
