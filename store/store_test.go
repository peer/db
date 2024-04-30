package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

var dummyData = []byte(`{}`) //nolint:gochecknoglobals

type testData struct {
	Data  int
	Patch bool
}

func (t *testData) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t testData) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *testData) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t testData) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

type testMetadata struct {
	Metadata string
}

func (t *testMetadata) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t testMetadata) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *testMetadata) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t testMetadata) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

type testPatch struct {
	Patch bool
}

func (t *testPatch) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t testPatch) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *testPatch) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t testPatch) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

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

func toRawMessagePtr(data string) *json.RawMessage {
	j := json.RawMessage(data)
	return &j
}

func TestTop(t *testing.T) {
	t.Parallel()

	for _, dataType := range []string{"jsonb", "bytea", "text"} {
		dataType := dataType

		t.Run(dataType, func(t *testing.T) {
			t.Parallel()

			testTop(t, testCase[*testData, *testMetadata, *testPatch]{
				InsertData:      &testData{Data: 123, Patch: false},
				InsertMetadata:  &testMetadata{Metadata: "foobar"},
				UpdateData:      &testData{Data: 123, Patch: true},
				UpdateMetadata:  &testMetadata{Metadata: "zoofoo"},
				UpdatePatch:     &testPatch{Patch: true},
				ReplaceData:     &testData{Data: 345, Patch: false},
				ReplaceMetadata: &testMetadata{Metadata: "another"},
				DeleteData:      nil,
				DeleteMetadata:  &testMetadata{Metadata: "admin"},
				CommitMetadata:  &testMetadata{Metadata: "commit"},
				NoPatches:       []*testPatch{},
				UpdatePatches:   []*testPatch{{Patch: true}},
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
				InsertData:      toRawMessagePtr(`{"data": 123}`),
				InsertMetadata:  toRawMessagePtr(`{"metadata": "foobar"}`),
				UpdateData:      toRawMessagePtr(`{"data": 123, "patch": true}`),
				UpdateMetadata:  toRawMessagePtr(`{"metadata": "zoofoo"}`),
				UpdatePatch:     toRawMessagePtr(`{"patch": true}`),
				ReplaceData:     toRawMessagePtr(`{"data": 345}`),
				ReplaceMetadata: toRawMessagePtr(`{"metadata": "another"}`),
				DeleteData:      nil,
				DeleteMetadata:  toRawMessagePtr(`{"metadata": "admin"}`),
				CommitMetadata:  toRawMessagePtr(`{"metadata": "commit"}`),
				NoPatches:       []*json.RawMessage{},
				UpdatePatches:   []*json.RawMessage{toRawMessagePtr(`{"patch": true}`)},
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

type lockableSlice[T any] struct {
	data []T
	mu   sync.Mutex
}

func (l *lockableSlice[T]) Append(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, v)
}

func (l *lockableSlice[T]) Prune() []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := slices.Clone(l.data)
	l.data = nil
	return c
}

func initDatabase[Data, Metadata, Patch any](
	t *testing.T, dataType string,
) (context.Context, *store.Store[Data, Metadata, Patch], *lockableSlice[store.CommittedChangeset[Data, Metadata, Patch]]) {
	t.Helper()

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	schema := identifier.New().String()

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "tests"
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	channel := make(chan store.CommittedChangeset[Data, Metadata, Patch])
	t.Cleanup(func() { close(channel) })

	channelContents := new(lockableSlice[store.CommittedChangeset[Data, Metadata, Patch]])

	go func() {
		for c := range channel {
			channelContents.Append(c)
		}
	}()
	s := &store.Store[Data, Metadata, Patch]{
		Schema:       schema,
		Committed:    channel,
		DataType:     dataType,
		MetadataType: dataType,
		PatchType:    dataType,
	}

	errE = s.Init(ctx, dbpool)
	require.NoError(t, errE, "% -+#.1v", errE)

	return ctx, s, channelContents
}

func testTop[Data, Metadata, Patch any](t *testing.T, d testCase[Data, Metadata, Patch], dataType string) { //nolint:maintidx
	t.Helper()

	ctx, s, channelContents := initDatabase[Data, Metadata, Patch](t, dataType)

	_, _, _, errE := s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	expectedID := identifier.New()

	insertVersion, errE := s.Insert(ctx, expectedID, d.InsertData, d.InsertMetadata)
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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, insertVersion.Changeset, c[0].Changeset.ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, insertVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, insertVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	updateVersion, errE := s.Update(ctx, expectedID, insertVersion.Changeset, d.UpdateData, d.UpdatePatch, d.UpdateMetadata)
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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, updateVersion.Changeset, c[0].Changeset.ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, updateVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, updateVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	replaceVersion, errE := s.Replace(ctx, expectedID, updateVersion.Changeset, d.ReplaceData, d.ReplaceMetadata)
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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, replaceVersion.Changeset, c[0].Changeset.ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, replaceVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, replaceVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	deleteVersion, errE := s.Delete(ctx, expectedID, replaceVersion.Changeset, d.DeleteMetadata)
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

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, deleteVersion.Changeset, c[0].Changeset.ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx)
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
	assert.NotErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Nil(t, data)
	assert.Nil(t, metadata)
	assert.Empty(t, version)

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	assert.Empty(t, c)

	changesets, errE := s.Commit(ctx, changeset, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)
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

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, newVersion.Changeset, c[0].Changeset.ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changeset.Changes(ctx)
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
	assert.NoError(t, errE, "% -+#.1v", errE)
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

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, store.MainView, c[0].View.Name())
		assert.Equal(t, newVersion.Changeset, c[0].Changeset.ID())
		committedChangeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := committedChangeset.Changeset.Changes(ctx)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	ids := []identifier.Identifier{}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	for i := 0; i < 6000; i++ {
		newID := identifier.New()
		_, errE = changeset.Insert(ctx, newID, dummyData, dummyData)
		require.NoError(t, errE, "%d % -+#.1v", errE)

		ids = append(ids, newID)
	}

	_, errE = s.Commit(ctx, changeset, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	page1, errE := s.List(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, 5000)

	page2, errE := s.List(ctx, &page1[4999])
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page2, 1000)

	inserted := []identifier.Identifier{}
	inserted = append(inserted, page1...)
	inserted = append(inserted, page2...)

	ids = sortIDs(ids...)

	assert.Equal(t, ids, inserted)

	v, errE := s.View(ctx, "unknown")
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		_, errE = v.List(ctx, nil)
		assert.ErrorIs(t, errE, store.ErrViewNotFound)
	}

	// Having no more values is not an error.
	page3, errE := s.List(ctx, &page2[999])
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, page3, 0)

	// Using unknown after ID is an error.
	newID := identifier.New()
	_, errE = s.List(ctx, &newID)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestChangesPagination(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changesets := []identifier.Identifier{}

	newID := identifier.New()
	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	changesets = append(changesets, version.Changeset)

	var changeset store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage]
	for i := 0; i < 6000; i++ {
		changeset, errE = s.Begin(ctx)
		require.NoError(t, errE, "% -+#.1v", errE)

		version, errE = changeset.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
		require.NoError(t, errE, "%d % -+#.1v", errE)

		changesets = append(changesets, version.Changeset)
	}

	_, errE = s.Commit(ctx, changeset, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	page1, errE := s.Changes(ctx, newID, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, page1, 5000)

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
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, page3, 0)

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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE := changeset.Insert(ctx, newID, dummyData, dummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	_, errE = changeset.Insert(ctx, newID, dummyData, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Delete(ctx, newID, changeset.ID(), dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = changeset.Update(ctx, newID, changeset.ID(), dummyData, dummyData, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)
}

func TestCycles(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	newVersion, errE := s.Insert(ctx, newID, dummyData, dummyData)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), dummyData, dummyData, dummyData)
	// This is not possible for two reasons:
	// Every changeset can have only one change per value ID.
	// Parent changeset must contain a change for the same value ID - fails here.
	assert.ErrorIs(t, errE, store.ErrParentInvalid)

	// Some insert, to make changeset exist.
	_, errE = changeset.Insert(ctx, identifier.New(), dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// We use changeset.ID() for parent changeset, to try to make a zero length cycle.
	_, errE = changeset.Update(ctx, newID, changeset.ID(), dummyData, dummyData, dummyData)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()
	secondID := identifier.New()

	changeset1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Insert(ctx, secondID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	changeset2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Update(ctx, secondID, changeset1.ID(), dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset2.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset1.Update(ctx, newID, changeset2.ID(), dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	changesets, errE := s.Commit(ctx, changeset1, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []store.Changeset[json.RawMessage, json.RawMessage, json.RawMessage]{changeset1, changeset2}, changesets)
}

func TestGetCurrent(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	_, errE := s.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, _, _, errE = v.GetLatest(ctx, newID) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	_, _, _, errE = s.GetLatest(ctx, identifier.New()) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
}

func TestGet(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	v, errE := s.View(ctx, "notexist")
	assert.NoError(t, errE, "% -+#.1v", errE)

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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, errE := s.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	_, _, errE = s.Get(ctx, newID, version)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view.
	updated2, errE := s.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = s.Get(ctx, newID, updated2)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

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
	assert.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// Committing from the second (child) view into the main view should not be possible
	// because that would introduce two versions of the same value.
	changeset, errE = s.Changeset(ctx, updated.Changeset)
	assert.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, mainView, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	// But we can merge into the main view.
	merged, errE := s.Merge(
		ctx,
		newID,
		[]identifier.Identifier{updated2.Changeset, updated.Changeset},
		dummyData,
		[]json.RawMessage{dummyData, dummyData},
		dummyData,
	)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be merged.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	_, _, errE = s.Get(ctx, newID, merged)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})

	// The version in the second (child) view should be what was there before.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// We can now commit the merged changeset into the second (child) view.
	changeset, errE = s.Changeset(ctx, merged.Changeset)
	assert.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be merged.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, merged, latest)
	_, _, errE = v.Get(ctx, newID, merged)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		merged.Changeset,
		sortIDs(updated.Changeset, updated2.Changeset)[0],
		sortIDs(updated.Changeset, updated2.Changeset)[1],
		version.Changeset,
	})
}

func TestChangeAcrossViews(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	mainView, errE := s.View(ctx, store.MainView)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// We create another (child) view.
	v, errE := mainView.Create(ctx, "second", dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// We update the value in the second (child view).
	updated, errE := v.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should be what was there before.
	_, _, latest, errE := s.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, version, latest)
	_, _, errE = s.Get(ctx, newID, version)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		version.Changeset,
	})

	// The version in the second (child) view should be the new updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We update the value in the main view by using the change from the second (child) view.
	// This should commit two changesets to the main view.
	updated2, errE := s.Update(ctx, newID, updated.Changeset, dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the main view should now be updated.
	_, _, latest, errE = s.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = s.Get(ctx, newID, updated2)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChanges(t, ctx, s, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})

	// It should now be possible to get the previously updated version as well in the main view.
	_, _, errE = s.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should stay the previously updated version.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated, latest)
	_, _, errE = v.Get(ctx, newID, updated)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated.Changeset,
		version.Changeset,
	})

	// It should not be possible to get the new updated value in the second (child) view.
	_, _, errE = v.Get(ctx, newID, updated2)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// We can explicitly update the second (child) view with the new changeset from the main view.
	changeset, errE := s.Changeset(ctx, updated2.Changeset)
	assert.NoError(t, errE, "% -+#.1v", errE)
	_, errE = changeset.Commit(ctx, v, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// The version in the second (child) view should now be updated.
	_, _, latest, errE = v.GetLatest(ctx, newID)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, updated2, latest)
	_, _, errE = v.Get(ctx, newID, updated2)
	assert.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, newID, []identifier.Identifier{
		updated2.Changeset,
		updated.Changeset,
		version.Changeset,
	})
}

func TestView(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	v, errE := s.View(ctx, store.MainView)
	assert.NoError(t, errE, "% -+#.1v", errE)

	v2, errE := v.Create(ctx, "child", dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	errE = v2.Release(ctx, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child", dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	v, errE = s.View(ctx, "notexist")
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = v.Create(ctx, "child2", dummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)

	errE = v.Release(ctx, dummyData)
	assert.ErrorIs(t, errE, store.ErrViewNotFound)
}

func TestDuplicateValues(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// Inserting another value with same ID should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Insert(ctx, newID, dummyData, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)

	_, errE = s.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// Updating an old value should error when using top-level methods
	// which auto-commit to original view.
	_, errE = s.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	assert.ErrorIs(t, errE, store.ErrConflict)
}

func TestDiscardAfterCommit(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changeset.Insert(ctx, newID, dummyData, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, dummyData)
	assert.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	assert.ErrorIs(t, errE, store.ErrAlreadyCommitted)
}

func TestEmptyChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	changeset, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = s.Commit(ctx, changeset, dummyData)
	assert.ErrorIs(t, errE, store.ErrChangesetNotFound)

	errE = changeset.Discard(ctx)
	assert.NoError(t, errE, "% -+#.1v", errE)

	errE = changeset.Discard(ctx)
	assert.NoError(t, errE, "% -+#.1v", errE)
}

func sortIDs(ids ...identifier.Identifier) []identifier.Identifier {
	slices.SortFunc(ids, func(a, b identifier.Identifier) int {
		return bytes.Compare(a[:], b[:])
	})
	return ids
}

func testChanges[Data, Metadata, Patch any](
	t *testing.T, ctx context.Context, s *store.Store[Data, Metadata, Patch], //nolint:revive
	id identifier.Identifier, expected []identifier.Identifier,
) {
	t.Helper()

	v, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

	testChangesView(t, ctx, v, id, expected)
}

func testChangesView[Data, Metadata, Patch any](
	t *testing.T, ctx context.Context, v store.View[Data, Metadata, Patch], //nolint:revive
	id identifier.Identifier, expected []identifier.Identifier,
) {
	t.Helper()

	changes, errE := v.Changes(ctx, id, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, expected, changes)

	for i, c := range changes {
		c := c
		cs, errE := v.Changes(ctx, id, &c)
		assert.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, changes[i+1:], cs, "%d %#v", i, c)
	}
}

func TestMultiplePathsToSameChangeset(t *testing.T) {
	t.Parallel()

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB1, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB1.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB2, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB2.Update(ctx, newID, changesetB1.ID(), dummyData, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(ctx, newID, []identifier.Identifier{changesetA.ID(), changesetB2.ID()}, dummyData, []json.RawMessage{dummyData, dummyData}, dummyData)
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

	ctx, s, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	newID := identifier.New()

	version, errE := s.Insert(ctx, newID, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetA, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetA.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	changesetB, errE := s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	_, errE = changesetB.Update(ctx, newID, version.Changeset, dummyData, dummyData, dummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	merged, errE := s.Merge(ctx, newID, []identifier.Identifier{changesetA.ID(), changesetB.ID()}, dummyData, []json.RawMessage{dummyData, dummyData}, dummyData)
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
