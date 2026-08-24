package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
)

// TestRepairMetadata tests that RepairMetadata calls repair for every stored change row, with deleted
// rows flagged and their data zeroed, and rewrites the metadata of the rows repair reports changed, in
// place: revisions and versions stay as they are and nothing is committed. A dry run only reports.
func TestRepairMetadata(t *testing.T) {
	t.Parallel()

	ctx, s, channelContents, _ := initDatabase[json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage](t, "jsonb")

	id := identifier.New()
	v1, errE := s.Insert(ctx, id, json.RawMessage(`{"v":1}`), json.RawMessage(`{"keep":"a"}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)
	v2, errE := s.Replace(ctx, id, v1.Changeset, json.RawMessage(`{"v":2}`), json.RawMessage(`{"keep":"b"}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)
	_, errE = s.Delete(ctx, id, v2.Changeset, json.RawMessage(`{"keep":"c"}`), json.RawMessage(`{}`))
	require.NoError(t, errE, "% -+#.1v", errE)

	// Wait for all commits to be observed, so the repair below can be asserted to commit nothing new.
	require.Eventually(t, func() bool { return channelContents.Len() >= 3 }, 10*time.Second, 10*time.Millisecond)
	channelContents.Prune()

	// The repair adds a derived field to every live row, deriving it from the row's data, and leaves
	// deletion rows alone.
	repair := func(_ identifier.Identifier, data json.RawMessage, deleted bool, metadata json.RawMessage) (json.RawMessage, bool, errors.E) {
		if deleted {
			assert.Empty(t, data)
			return metadata, false, nil
		}
		var content struct {
			V int `json:"v"`
		}
		errE := x.UnmarshalWithoutUnknownFields(data, &content)
		if errE != nil {
			return nil, false, errE
		}
		var m map[string]any
		errE = x.Unmarshal(metadata, &m)
		if errE != nil {
			return nil, false, errE
		}
		if derived, ok := m["derived"].(float64); ok && int(derived) == content.V {
			return metadata, false, nil
		}
		m["derived"] = content.V
		repaired, errE := x.MarshalWithoutEscapeHTML(m)
		if errE != nil {
			return nil, false, errE
		}
		return repaired, true, nil
	}

	// A dry run reports the two live rows as needing repair without writing anything.
	examined, repaired, errE := s.RepairMetadata(ctx, true, repair, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(3), examined)
	assert.Equal(t, int64(2), repaired)
	_, metadata, _, _, errE := s.Get(ctx, id, v1) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, `{"keep":"a"}`, string(metadata), "a dry run should not write the metadata")

	// The repair writes the two live rows, preserving what it does not derive.
	examined, repaired, errE = s.RepairMetadata(ctx, false, repair, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(3), examined)
	assert.Equal(t, int64(2), repaired)

	_, metadata, _, _, errE = s.Get(ctx, id, v1) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, `{"keep":"a","derived":1}`, string(metadata))
	_, metadata, _, _, errE = s.Get(ctx, id, v2) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.JSONEq(t, `{"keep":"b","derived":2}`, string(metadata))

	// The latest version is still the deletion, with its metadata untouched.
	_, metadata, _, _, errE = s.GetLatest(ctx, id) //nolint:dogsled
	assert.ErrorIs(t, errE, store.ErrValueDeleted)
	assert.JSONEq(t, `{"keep":"c"}`, string(metadata))

	// A further run finds nothing to repair, and no run committed anything.
	_, repaired, errE = s.RepairMetadata(ctx, false, repair, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Zero(t, repaired)
	assert.Zero(t, channelContents.Len())
}
