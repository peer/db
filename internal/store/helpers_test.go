package store_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func TestTestDataScanBytesAndBytesValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestData{Data: 42, Patch: true}
	b, err := original.BytesValue()
	require.NoError(t, err)

	var decoded internalStore.TestData
	err = decoded.ScanBytes(b)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestDataScanTextAndTextValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestData{Data: 99, Patch: false}
	tv, err := original.TextValue()
	require.NoError(t, err)
	assert.True(t, tv.Valid)

	var decoded internalStore.TestData
	err = decoded.ScanText(tv)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestMetadataScanBytesAndBytesValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestMetadata{Metadata: "hello"}
	b, err := original.BytesValue()
	require.NoError(t, err)

	var decoded internalStore.TestMetadata
	err = decoded.ScanBytes(b)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestMetadataScanTextAndTextValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestMetadata{Metadata: "world"}
	tv, err := original.TextValue()
	require.NoError(t, err)
	assert.True(t, tv.Valid)

	var decoded internalStore.TestMetadata
	err = decoded.ScanText(tv)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestPatchScanBytesAndBytesValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestPatch{Patch: true}
	b, err := original.BytesValue()
	require.NoError(t, err)

	var decoded internalStore.TestPatch
	err = decoded.ScanBytes(b)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestPatchScanTextAndTextValue(t *testing.T) {
	t.Parallel()

	original := internalStore.TestPatch{Patch: false}
	tv, err := original.TextValue()
	require.NoError(t, err)
	assert.True(t, tv.Valid)

	var decoded internalStore.TestPatch
	err = decoded.ScanText(tv)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestTestDataScanTextInvalidJSON(t *testing.T) {
	t.Parallel()

	var decoded internalStore.TestData
	err := decoded.ScanText(pgtype.Text{String: "not json", Valid: true})
	assert.EqualError(t, err, "invalid character 'o' in literal null (expecting 'u')")
}

func TestTestMetadataScanBytesInvalidJSON(t *testing.T) {
	t.Parallel()

	var decoded internalStore.TestMetadata
	err := decoded.ScanBytes([]byte("not json"))
	assert.EqualError(t, err, "invalid character 'o' in literal null (expecting 'u')")
}

func TestTestPatchScanBytesInvalidJSON(t *testing.T) {
	t.Parallel()

	var decoded internalStore.TestPatch
	err := decoded.ScanBytes([]byte("{invalid"))
	assert.EqualError(t, err, "invalid character 'i' looking for beginning of object key string")
}

func TestToRawMessagePtr(t *testing.T) {
	t.Parallel()

	result := internalStore.ToRawMessagePtr(`{"key":"value"}`)
	require.NotNil(t, result)
	assert.Equal(t, json.RawMessage(`{"key":"value"}`), *result) //nolint:testifylint
}

func TestDummyData(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []byte(`{}`), internalStore.DummyData)
}

func TestLockableSliceAppendAndLen(t *testing.T) {
	t.Parallel()

	ls := new(internalStore.LockableSlice[int])
	assert.Equal(t, 0, ls.Len())

	ls.Append(1)
	ls.Append(2)
	ls.Append(3)
	assert.Equal(t, 3, ls.Len())
}

func TestLockableSlicePrune(t *testing.T) {
	t.Parallel()

	ls := new(internalStore.LockableSlice[string])
	ls.Append("a")
	ls.Append("b")

	pruned := ls.Prune()
	assert.Equal(t, []string{"a", "b"}, pruned)
	assert.Equal(t, 0, ls.Len())

	// Prune again returns nil.
	pruned2 := ls.Prune()
	assert.Nil(t, pruned2)
}

func TestLockableSliceConcurrent(t *testing.T) {
	t.Parallel()

	ls := new(internalStore.LockableSlice[int])
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Go(func() {
			ls.Append(i)
		})
	}

	wg.Wait()
	assert.Equal(t, 100, ls.Len())

	pruned := ls.Prune()
	assert.Len(t, pruned, 100)
	assert.Equal(t, 0, ls.Len())
}
