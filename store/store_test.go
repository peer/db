package store_test

import (
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

func initDatabase[Data, Metadata, Patch any](t *testing.T, dataType string) (
	context.Context, *store.Store[Data, Metadata, Patch], *lockableSlice[store.Changeset[Data, Metadata, Patch]],
) {
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

	channel := make(chan store.Changeset[Data, Metadata, Patch])
	t.Cleanup(func() { close(channel) })

	channelContents := new(lockableSlice[store.Changeset[Data, Metadata, Patch]])

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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, insertVersion.Changeset, c[0].ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, updateVersion.Changeset, c[0].ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
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

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, replaceVersion.Changeset, c[0].ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
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

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, deleteVersion.Changeset, c[0].ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
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

	mainView, errE := s.View(ctx, store.MainView)
	assert.NoError(t, errE, "% -+#.1v", errE)

	// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).
	changesets, errE := changeset.Commit(ctx, mainView, d.CommitMetadata)
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
		assert.Equal(t, newVersion.Changeset, c[0].ID())
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}

	newID = identifier.New()

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE = changeset.Insert(ctx, newID, d.InsertData, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, changeset.ID(), newVersion.Changeset)
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	// This time we recreate the changeset object.
	changeset, errE = s.Changeset(ctx, changeset.ID())
	require.NoError(t, errE, "% -+#.1v", errE)

	// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).
	changesets, errE = changeset.Commit(ctx, mainView, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

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

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, newVersion.Changeset, c[0].ID())
		changeset, errE = c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
				}
			}
		}
	}
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

	mainView, errE := s.View(ctx, store.MainView)
	require.NoError(t, errE, "% -+#.1v", errE)

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

	changesets, errE := changeset1.Commit(ctx, mainView, dummyData)
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

	// two versions in two views, the one in the other view should not be accessible.

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
