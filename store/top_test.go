package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

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

type testMetadata struct {
	Metadata string
}

func (t *testMetadata) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t testMetadata) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
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
	})

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
	})

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
	})

	// TODO: Make metadata as "string" work. See: https://github.com/jackc/pgx/issues/1977
	testTop(t, testCase[[]byte, json.RawMessage, any]{
		InsertData:      []byte(`{"data": 123}`),
		InsertMetadata:  json.RawMessage(`{"metadata": "foobar"}`),
		UpdateData:      []byte(`{"data": 123, "patch": true}`),
		UpdateMetadata:  json.RawMessage(`{"metadata": "zoofoo"}`),
		UpdatePatch:     nil,
		ReplaceData:     []byte(`{"data": 345}`),
		ReplaceMetadata: json.RawMessage(`{"metadata": "another"}`),
		DeleteData:      nil,
		DeleteMetadata:  json.RawMessage(`{"metadata": "admin"}`),
		CommitMetadata:  json.RawMessage(`{"metadata": "commit"}`),
	})
}

func testTop[Data, Metadata, Patch any](t *testing.T, d testCase[Data, Metadata, Patch]) { //nolint:maintidx
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	schema := identifier.New().String()

	dbpool, errE := internal.InitPostgres(ctx, os.Getenv("POSTGRES"), logger, func(context.Context) (string, string) {
		return schema, "123"
	})
	require.NoError(t, errE, "% -+#.1v", errE)

	s := &store.Store[Data, Metadata, Patch]{
		Schema: schema,
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

	// TODO: Provide a way to access commit metadata (e.g., list all commits for a view).
	errE = changeset.Commit(ctx, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)

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
	errE = changeset.Commit(ctx, d.CommitMetadata)
	assert.NoError(t, errE, "% -+#.1v", errE)

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
}
