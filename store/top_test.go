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

	if os.Getenv("POSTGRES") == "" {
		t.Skip("POSTGRES is not available")
	}

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

func testTop[Data, Metadata, Patch any](t *testing.T, d testCase[Data, Metadata, Patch], dataType string) { //nolint:maintidx
	t.Helper()

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

	_, _, _, errE = s.GetCurrent(ctx, identifier.New()) //nolint:dogsled
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

	data, metadata, version, errE := s.GetCurrent(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, insertVersion, version)
	}

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c := channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, insertVersion.Changeset, c[0].Identifier)
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, insertVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, insertVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.InsertData, changes[0].Data)
					assert.Equal(t, d.InsertMetadata, changes[0].Metadata)
					assert.Equal(t, d.NoPatches, changes[0].Patches)
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

	data, metadata, version, errE = s.GetCurrent(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.UpdateData, data)
		assert.Equal(t, d.UpdateMetadata, metadata)
		assert.Equal(t, updateVersion, version)
	}

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, updateVersion.Changeset, c[0].Identifier)
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, updateVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, updateVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.UpdateData, changes[0].Data)
					assert.Equal(t, d.UpdateMetadata, changes[0].Metadata)
					assert.Equal(t, d.UpdatePatches, changes[0].Patches)
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

	data, metadata, version, errE = s.GetCurrent(ctx, expectedID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.ReplaceData, data)
		assert.Equal(t, d.ReplaceMetadata, metadata)
		assert.Equal(t, replaceVersion, version)
	}

	// We sleep to make sure all changesets are retrieved.
	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, replaceVersion.Changeset, c[0].Identifier)
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, replaceVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, replaceVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.ReplaceData, changes[0].Data)
					assert.Equal(t, d.ReplaceMetadata, changes[0].Metadata)
					assert.Equal(t, d.NoPatches, changes[0].Patches)
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

	data, metadata, version, errE = s.GetCurrent(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, deleteVersion.Changeset, c[0].Identifier)
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, expectedID, changes[0].ID)
					assert.Equal(t, deleteVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, deleteVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.DeleteData, changes[0].Data)
					assert.Equal(t, d.DeleteMetadata, changes[0].Metadata)
					assert.Equal(t, d.NoPatches, changes[0].Patches)
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
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	data, metadata, version, errE = s.GetCurrent(ctx, expectedID)
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Equal(t, d.DeleteData, data)
	assert.Equal(t, d.DeleteMetadata, metadata)
	assert.Equal(t, deleteVersion, version)

	data, metadata, version, errE = s.GetCurrent(ctx, newID)
	assert.NotErrorIs(t, errE, store.ErrValueDeleted)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)
	assert.Nil(t, data)
	assert.Nil(t, metadata)
	assert.Empty(t, version)

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	assert.Empty(t, c)

	// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).
	changesets, errE := changeset.Commit(ctx, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

	data, metadata, version, errE = s.GetCurrent(ctx, expectedID)
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

	data, metadata, version, errE = s.GetCurrent(ctx, newID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
	}

	data, metadata, errE = s.Get(ctx, newID, store.Version{
		Changeset: changeset.Identifier,
		Revision:  1,
	})
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, newVersion.Changeset, c[0].Identifier)
		changeset, errE := c[0].WithStore(ctx, s) //nolint:govet
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx)
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.InsertData, changes[0].Data)
					assert.Equal(t, d.InsertMetadata, changes[0].Metadata)
					assert.Equal(t, d.NoPatches, changes[0].Patches)
				}
			}
		}
	}

	newID = identifier.New()

	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE = changeset.Insert(ctx, newID, d.InsertData, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	// This time we recreate the changeset object.
	changeset, errE = s.Changeset(ctx, changeset.Identifier)
	require.NoError(t, errE, "% -+#.1v", errE)

	// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).
	changesets, errE = changeset.Commit(ctx, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)
	if assert.Len(t, changesets, 1) {
		assert.Equal(t, changeset, changesets[0])
	}

	data, metadata, errE = s.Get(ctx, newID, newVersion)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
	}

	data, metadata, version, errE = s.GetCurrent(ctx, newID)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, d.InsertData, data)
		assert.Equal(t, d.InsertMetadata, metadata)
		assert.Equal(t, newVersion, version)
	}

	time.Sleep(10 * time.Millisecond)
	c = channelContents.Prune()
	if assert.Len(t, c, 1) {
		assert.Equal(t, newVersion.Changeset, c[0].Identifier)
		changeset, errE = c[0].WithStore(ctx, s)
		if assert.NoError(t, errE, "% -+#.1v", errE) {
			changes, errE := changeset.Changes(ctx) //nolint:govet
			if assert.NoError(t, errE, "% -+#.1v", errE) {
				if assert.Len(t, changes, 1) {
					assert.Equal(t, newID, changes[0].ID)
					assert.Equal(t, newVersion.Changeset, changes[0].Version.Changeset)
					assert.Equal(t, newVersion.Revision, changes[0].Version.Revision)
					assert.Equal(t, d.InsertData, changes[0].Data)
					assert.Equal(t, d.InsertMetadata, changes[0].Metadata)
					assert.Equal(t, d.NoPatches, changes[0].Patches)
				}
			}
		}
	}

	newID = identifier.New()

	// Test errors.
	changeset, errE = s.Begin(ctx)
	require.NoError(t, errE, "% -+#.1v", errE)

	newVersion, errE = changeset.Insert(ctx, newID, d.InsertData, d.InsertMetadata)
	if assert.NoError(t, errE, "% -+#.1v", errE) {
		assert.Equal(t, int64(1), newVersion.Revision)
	}

	_, errE = changeset.Insert(ctx, newID, d.InsertData, d.InsertMetadata)
	assert.ErrorIs(t, errE, store.ErrConflict)

	errE = changeset.Discard(ctx)
	assert.NoError(t, errE, "% -+#.1v", errE)
}
