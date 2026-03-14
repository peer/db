package store_test

import (
	"bytes"
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

	internal "gitlab.com/peerdb/peerdb/internal/store"
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

			testTop(t, testCase[*internal.TestData, *internal.TestMetadata, *internal.TestPatch]{
				InsertData:      &internal.TestData{Data: 123, Patch: false},
				InsertMetadata:  &internal.TestMetadata{Metadata: "foobar"},
				UpdateData:      &internal.TestData{Data: 123, Patch: true},
				UpdateMetadata:  &internal.TestMetadata{Metadata: "zoofoo"},
				UpdatePatch:     &internal.TestPatch{Patch: true},
				ReplaceData:     &internal.TestData{Data: 345, Patch: false},
				ReplaceMetadata: &internal.TestMetadata{Metadata: "another"},
				DeleteData:      nil,
				DeleteMetadata:  &internal.TestMetadata{Metadata: "admin"},
				CommitMetadata:  &internal.TestMetadata{Metadata: "commit"},
				NoPatches:       []*internal.TestPatch{},
				UpdatePatches:   []*internal.TestPatch{{Patch: true}},
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
				InsertData:      internal.ToRawMessagePtr(`{"data": 123}`),
				InsertMetadata:  internal.ToRawMessagePtr(`{"metadata": "foobar"}`),
				UpdateData:      internal.ToRawMessagePtr(`{"data": 123, "patch": true}`),
				UpdateMetadata:  internal.ToRawMessagePtr(`{"metadata": "zoofoo"}`),
				UpdatePatch:     internal.ToRawMessagePtr(`{"patch": true}`),
				ReplaceData:     internal.ToRawMessagePtr(`{"data": 345}`),
				ReplaceMetadata: internal.ToRawMessagePtr(`{"metadata": "another"}`),
				DeleteData:      nil,
				DeleteMetadata:  internal.ToRawMessagePtr(`{"metadata": "admin"}`),
				CommitMetadata:  internal.ToRawMessagePtr(`{"metadata": "commit"}`),
				NoPatches:       []*json.RawMessage{},
				UpdatePatches:   []*json.RawMessage{internal.ToRawMessagePtr(`{"patch": true}`)},
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
	*internal.LockableSlice[store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]],
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
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	listener := internal.NewListener(dbpool)

	s := &store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		Prefix:       prefix,
		DataType:     dataType,
		MetadataType: dataType,
		PatchType:    dataType,
	}

	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	internal.StartListener(ctx, listener)

	// Allow the listener goroutine to connect and register LISTEN before the test makes commits.
	time.Sleep(100 * time.Millisecond)

	channelContents := new(internal.LockableSlice[store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]])

	go func() {
		for {
			select {
			case c := <-s.Committed.Get():
				channelContents.Append(c)
			case <-ctx.Done():
				return
			}
		}
	}()

	return ctx, s, channelContents
}

func testTop[Data, Metadata, Patch any](t *testing.T, d testCase[Data, Metadata, Patch], dataType string) { //nolint:maintidx
	t.Helper()

	ctx, s, channelContents := initDatabase[Data, Metadata, Metadata, Metadata, Metadata, Patch](t, dataType)

	_, _, _, errE := s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	expectedID := identifier.New()

	insertVersion, errE := s.Insert(ctx, expectedID, d.InsertData, d.InsertMetadata, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), insertVersion.Revision)
	}

	data, metadata, errE := s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, version, errE := s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, version)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		insertVersion.Changeset,
	})

	time.Sleep(100 * time.Millisecond)
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

	data, metadata, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
	}

	data, metadata, version, errE = s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, version)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	time.Sleep(100 * time.Millisecond)
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

	data, metadata, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, replaceVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
	}

	data, metadata, version, errE = s.GetLatest(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
		assert.Equal(t, replaceVersion, version)
	}

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		replaceVersion.Changeset,
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	time.Sleep(100 * time.Millisecond)
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

	data, metadata, errE = s.Get(ctx, expectedID, insertVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, updateVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, replaceVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
	}

	data, metadata, errE = s.Get(ctx, expectedID, deleteVersion)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)

	data, metadata, version, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)

	testChanges(t, ctx, s, expectedID, []identifier.Identifier{
		deleteVersion.Changeset,
		replaceVersion.Changeset,
		updateVersion.Changeset,
		insertVersion.Changeset,
	})

	time.Sleep(100 * time.Millisecond)
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

	data, metadata, version, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)

	data, metadata, version, errE = s.GetLatest(ctx, newID)
	assert.NotErrorIs(t, errE, store.ErrValueDeleted) //nolint:testifylint
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Nil(t, data)
	assert.Nil(t, metadata)
	assert.Empty(t, version)

	time.Sleep(100 * time.Millisecond)
	c = channelContents.Prune()
	assert.Empty(t, c)

	changesets, errE := s.Commit(ctx, changeset, d.CommitMetadata)
	require.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

	data, metadata, version, errE = s.GetLatest(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)

	data, metadata, errE = s.Get(ctx, newID, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, version, errE = s.GetLatest(ctx, newID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
	}

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		newVersion.Changeset,
	})

	data, metadata, errE = s.Get(ctx, newID, store.Version{
		Changeset: changeset.ID(),
		Revision:  1,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	time.Sleep(100 * time.Millisecond)
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

	data, metadata, errE = s.Get(ctx, newID2, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, version, errE = s.GetLatest(ctx, newID2)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
	}

	testChanges(t, ctx, s, newID2, []identifier.Identifier{
		newVersion.Changeset,
	})

	time.Sleep(100 * time.Millisecond)
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
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	ids := []identifier.Identifier{}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := range 6000 {
		newID := identifier.New()
		_, errE = changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)

		ids = append(ids, newID)
	}

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	page1, errE := s.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, store.MaxPageLength)

	page2, errE := s.List(ctx, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1000)

	inserted := []identifier.Identifier{}
	inserted = append(inserted, page1...)
	inserted = append(inserted, page2...)

	ids = sortIDs(ids...)

	assert.Equal(t, ids, inserted)

	time.Sleep(100 * time.Millisecond)
	c := channelContents.Prune()
	assert.Len(t, c, 1)

	v, errE := s.View(ctx, "unknown")
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = v.List(ctx, nil)
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

	changes := []store.Change{}
	changes = append(changes, csPage1...)
	changes = append(changes, csPage2...)

	expected := []store.Change{}
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

	ctx, s, channelContents := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changesets := []identifier.Identifier{}

	newID := identifier.New()
	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	changesets = append(changesets, version.Changeset)

	var changeset store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]
	for i := range 6000 {
		changeset, errE = s.Begin(ctx)
		require.NoError(t, errE, "% -+#.1v", errE)

		version, errE = changeset.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
		require.NoError(t, errE, "%d % -+#.1v", i, errE)

		changesets = append(changesets, version.Changeset)
	}

	// We commit only once (the last changeset in the chain) for test to run faster.
	committed, errE := s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, committed, 6000)

	time.Sleep(500 * time.Millisecond)
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

	changes := []identifier.Identifier{}
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE := changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	_, errE = changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Update(ctx, newID, newVersion.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Delete(ctx, newID, newVersion.Changeset, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Update(ctx, newID, newVersion.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Merge(ctx, newID, []identifier.Identifier{newVersion.Changeset}, internal.DummyData, []json.RawMessage{internal.DummyData}, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Replace(ctx, newID, newVersion.Changeset, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)
}

func TestCycles(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	newVersion, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	// This is not possible for two reasons:
	// Every changeset can have only one change per value ID.
	// Parent changeset must contain a change for the same value ID - fails here.
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// Some insert, to make changeset exist.
	_, errE = changeset.Insert(ctx, identifier.New(), internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()
	secondID := identifier.New()

	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Insert(ctx, secondID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, secondID, changeset1.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Update(ctx, newID, changeset2.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesets, errE := s.Commit(ctx, changeset1, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(
		t,
		[]store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{changeset1, changeset2},
		changesets,
	)
}

func TestGetCurrent(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	_, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, errE = v.GetLatest(ctx, newID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, _, _, errE = s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestGet(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	// View does not really exist.
	_, _, errE = v.Get(ctx, newID, version)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	// Value at existing changeset does not exist for arbitrary ID.
	_, _, errE = s.Get(ctx, identifier.New(), version)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Value at arbitrary changeset does not exist for existing ID.
	_, _, errE = s.Get(ctx, newID, store.Version{
		Changeset: identifier.New(),
		Revision:  1,
	})
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestMultipleViews(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, errE := s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	_, _, errE = s.Get(ctx, newID, version)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view.
	updated2, errE := s.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = s.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the second (child) view.
	_, _, errE = v.Get(ctx, newID, updated2)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// Committing from the main view into the second (child) view should not be possible
	// because that would introduce two versions of the same value.
	changeset, errE := s.Changeset(ctx, updated2.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// Committing from the second (child) view into the main view should not be possible
	// because that would introduce two versions of the same value.
	changeset, errE = s.Changeset(ctx, updated.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, mainView, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// But we can merge into the main view.
	merged, errE := s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{updated2.Changeset, updated.Changeset},
		internal.DummyData,
		[]json.RawMessage{internal.DummyData, internal.DummyData},
		internal.DummyData,
		internal.DummyData,
	)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be merged.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	_, _, errE = s.Get(ctx, newID, merged)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// We can now commit the merged changeset into the second (child) view.
	changeset, errE = s.Changeset(ctx, merged.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be merged.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	_, _, errE = v.Get(ctx, newID, merged)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})
}

func TestChangeAcrossViews(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, errE := s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	_, _, errE = s.Get(ctx, newID, version)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view by using the change from the second (child) view.
	// This should commit two changesets to the main view.
	updated2, errE := s.Update(ctx, newID, updated.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = s.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})

	// It should now be possible to get the previously updated version as well in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should stay the previously updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the second (child) view.
	_, _, errE = v.Get(ctx, newID, updated2)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We can explicitly update the second (child) view with the new changeset from the main view.
	changeset, errE := s.Changeset(ctx, updated2.Changeset)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be updated.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = v.Get(ctx, newID, updated2)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})
}

func TestView(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	v, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	v2, errE := v.Create(ctx, "child", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", internal.DummyData)
	require.ErrorIs(t, errE, store.ErrConflict)

	errE = v2.Release(ctx, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE = s.View(ctx, "notexist")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child2", internal.DummyData)
	require.ErrorIs(t, errE, store.ErrViewNotFound)

	errE = v.Release(ctx, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

func TestDuplicateValues(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Inserting another value with same ID should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = s.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Updating an old value should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)
}

func TestDiscardAfterCommit(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)
}

func TestEmptyChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)

	errE = changeset.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestDiscardInUseChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, newID, changeset.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB1.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB2.Update(ctx, newID, changesetB1.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(
		ctx, newID,
		[]identifier.Identifier{changesetA.ID(), changesetB2.ID()},
		internal.DummyData,
		[]json.RawMessage{internal.DummyData, internal.DummyData},
		internal.DummyData,
		internal.DummyData,
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(
		ctx, newID,
		[]identifier.Identifier{changesetA.ID(), changesetB.ID()},
		internal.DummyData,
		[]json.RawMessage{internal.DummyData, internal.DummyData},
		internal.DummyData,
		internal.DummyData,
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	anotherVersion, errE := s.Insert(ctx, identifier.New(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	version, errE := changeset.Insert(ctx, newID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "unknown")
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Commit(ctx, v, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Insert(ctx, identifier.New(), internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Update(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Merge(ctx, newID, []identifier.Identifier{version.Changeset}, internal.DummyData, []json.RawMessage{internal.DummyData}, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Replace(ctx, newID, version.Changeset, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	_, errE = changeset.Delete(ctx, newID, version.Changeset, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)

	// The number of parent changesets have to match the number of patches.
	_, errE = s.Merge(ctx, newID, []identifier.Identifier{version.Changeset}, internal.DummyData, nil, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{identifier.New()},
		internal.DummyData,
		[]json.RawMessage{internal.DummyData},
		internal.DummyData,
		internal.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{anotherVersion.Changeset},
		internal.DummyData,
		[]json.RawMessage{internal.DummyData},
		internal.DummyData,
		internal.DummyData,
	)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Replace(ctx, newID, identifier.New(), internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Replace(ctx, newID, anotherVersion.Changeset, internal.DummyData, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent has to exist.
	_, errE = s.Delete(ctx, newID, identifier.New(), internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// The parent changeset has to contain a change for newID.
	_, errE = s.Delete(ctx, newID, anotherVersion.Changeset, internal.DummyData, internal.DummyData)
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	changeset, errE = s.Changeset(ctx, identifier.New())
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Changes(ctx, nil)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)
}

func TestParallelChange(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	firstID := identifier.New()
	secondID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, firstID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, secondID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Update(ctx, firstID, changeset.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, secondID, changeset.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No error because both changesets are changing different values from the same parent changeset.

	_, errE = s.Commit(ctx, changeset1, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset2, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
}

func TestCommittedOrdering(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	const n = 10
	for range n {
		id := identifier.New()
		_, errE := s.Insert(ctx, id, internal.DummyData, internal.DummyData, internal.DummyData)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	time.Sleep(100 * time.Millisecond)
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

	ctx, s, channelContents := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Prepare a chain: first insert two values in separate changesets,
	// then commit only the second, which also commits the first.
	firstID := identifier.New()
	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Insert(ctx, firstID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	secondID := identifier.New()
	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, firstID, changeset1.ID(), internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Insert(ctx, secondID, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Committing changeset2 also commits changeset1 (its uncommitted ancestor).
	committed, errE := s.Commit(ctx, changeset2, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, committed, 2)

	time.Sleep(100 * time.Millisecond)
	c := channelContents.Prune()
	// One CommittedChangesets per commit: the commit contains both changesets.
	require.Len(t, c, 1)
	assert.Positive(t, c[0].Seq)
	assert.Equal(t, store.MainView, c[0].View.Name())
	assert.Len(t, c[0].Changesets, 2)
}

func TestCommitLog(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Empty log initially.
	entries, errE := s.CommitLog(ctx, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, entries)

	// Make two separate commits.
	id1 := identifier.New()
	v1, errE := s.Insert(ctx, id1, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	id2 := identifier.New()
	v2, errE := s.Insert(ctx, id2, internal.DummyData, internal.DummyData, internal.DummyData)
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
	_, errE = cs.Insert(ctx, id3, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	id4 := identifier.New()
	_, errE = cs.Insert(ctx, id4, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Commit(ctx, cs, internal.DummyData)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	// Commit to main view.
	idMain := identifier.New()
	vMain, errE := s.Insert(ctx, idMain, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Create a child view and commit to it.
	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)
	childView, errE := mainView.Create(ctx, "child", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	idChild := identifier.New()
	vChild, errE := childView.Insert(ctx, idChild, internal.DummyData, internal.DummyData, internal.DummyData)
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

	// Release the "child" name — the view is now unnamed.
	errE = childView.Release(ctx, internal.DummyData)
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
	newChildView, errE := mainView.Create(ctx, "child", internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Commit to the new "child" view.
	idChild2 := identifier.New()
	vChild2, errE := newChildView.Insert(ctx, idChild2, internal.DummyData, internal.DummyData, internal.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Filtering by "child" now returns only the new commit — old commits are still unnamed.
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

	listener := internal.NewListener(dbpool)

	s := &store.Store[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage]{
		Prefix:        prefix,
		CommittedSize: 1,
		DataType:      "jsonb",
		MetadataType:  "jsonb",
		PatchType:     "jsonb",
	}

	errE = s.Init(ctx, dbpool, listener)
	require.NoError(t, errE, "% -+#.1v", errE)

	internal.StartListener(ctx, listener)

	// Allow the listener goroutine to connect and register LISTEN before the test makes commits.
	time.Sleep(100 * time.Millisecond)

	// Insert an initial document to confirm the channel is working.
	id1 := identifier.New()
	_, errE = s.Insert(ctx, id1, json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case <-s.Committed.Get():
		default:
			assert.Fail(c, "commit notification not yet received")
		}
	}, 5*time.Second, 10*time.Millisecond)

	// Save the current channel before simulating a listener reconnection.
	oldCh := s.Committed.Get()

	// Simulate a listener reconnection by calling HandleBacklog directly.
	// In production this is triggered when pgxlisten reconnects after a connection drop.
	// It should close the old channel (signaling consumers that notifications may have been
	// missed) and create a new one.
	err := s.HandleBacklog(ctx, s.Prefix+"CommittedChangesets", nil)
	require.NoError(t, errE, "% -+#.1v", err) // This is still errors.E.

	// Old channel must be closed so that consumers know to take corrective action.
	select {
	case _, ok := <-oldCh:
		require.False(t, ok, "old channel should be closed after HandleBacklog")
	case <-time.After(time.Second):
		t.Fatal("old channel was not closed by HandleBacklog")
	}

	// A new channel must be created.
	newCh := s.Committed.Get()
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
