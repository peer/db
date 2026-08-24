package store_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/store"
)

// TestListMetadata tests filtering List by metadata: only values whose latest committed version's
// metadata contains the filter are listed.
func TestListMetadata(t *testing.T) {
	t.Parallel()

	ctx, s, _, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	idA := identifier.New()
	idAB := identifier.New()
	idB := identifier.New()
	idNone := identifier.New()

	versionA, errE := s.Insert(ctx, idA, testutils.DummyData, json.RawMessage(`{"instanceOf":["A"]}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	versionAB, errE := s.Insert(ctx, idAB, testutils.DummyData, json.RawMessage(`{"instanceOf":["A","B"]}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, idB, testutils.DummyData, json.RawMessage(`{"instanceOf":["B"]}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Insert(ctx, idNone, testutils.DummyData, json.RawMessage(`{}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	filterA := json.RawMessage(`{"instanceOf":["A"]}`)

	ids, errE := s.List(ctx, filterA, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, sortIDs(idA, idAB), ids)

	ids, errE = s.List(ctx, json.RawMessage(`{"instanceOf":["B"]}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, sortIDs(idAB, idB), ids)

	ids, errE = s.List(ctx, json.RawMessage(`{"instanceOf":["A","B"]}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{idAB}, ids)

	// An empty filter matches every metadata, so this lists the same values as a nil filter.
	ids, errE = s.List(ctx, json.RawMessage(`{}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, sortIDs(idA, idAB, idB, idNone), ids)

	// Keyset pagination works together with the filter.
	both := sortIDs(idA, idAB)
	ids, errE = s.List(ctx, filterA, &both[0])
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, both[1:], ids)

	// Using unknown after ID is an error.
	unknownID := identifier.New()
	_, errE = s.List(ctx, filterA, &unknownID)
	assert.ErrorIs(t, errE, store.ErrValueNotFound)

	// The filter matches against the latest version's metadata.
	_, errE = s.Update(ctx, idA, versionA.Changeset, testutils.DummyData, testutils.DummyData, json.RawMessage(`{"instanceOf":["B"]}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	ids, errE = s.List(ctx, filterA, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{idAB}, ids)

	// A deleted value is listed by the metadata recorded by the delete.
	_, errE = s.Delete(ctx, idAB, versionAB.Changeset, json.RawMessage(`{"deleted":true}`), testutils.DummyData)
	require.NoError(t, errE, "% -+#.1v", errE)

	ids, errE = s.List(ctx, filterA, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, ids)

	ids, errE = s.List(ctx, json.RawMessage(`{"deleted":true}`), nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []identifier.Identifier{idAB}, ids)
}
