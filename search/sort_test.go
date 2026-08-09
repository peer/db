package search_test

import (
	"encoding/json"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/search"
)

// sortOptionJSON renders one built sort option as the JSON ElasticSearch receives for it.
func sortOptionJSON(t *testing.T, opt types.SortCombinationsVariant) string {
	t.Helper()

	data, err := json.Marshal(opt.SortCombinationsCaster())
	require.NoError(t, err)
	return string(data)
}

func TestBuildSort(t *testing.T) {
	t.Parallel()

	// Every sort ends with the document id so that the whole ordering is a total order and documents tying
	// on every other key do not fall through to the index's internal document order.
	const (
		idKey    = `{"id":{"order":"asc"}}`
		scoreKey = `{"_score":{"order":"desc"}}`
		timeKey  = `{"time":{"missing":"_last","order":"desc"}}`
		labelKey = `{"displaySort.en":{"missing":"_last","order":"asc","unmapped_type":"keyword"}}`
	)

	t.Run("no keys", func(t *testing.T) {
		t.Parallel()

		sorts := search.TestingBuildSort(nil, "en")
		require.Len(t, sorts, 4)
		assert.JSONEq(t, scoreKey, sortOptionJSON(t, sorts[0]))
		assert.JSONEq(t, timeKey, sortOptionJSON(t, sorts[1]))
		assert.JSONEq(t, labelKey, sortOptionJSON(t, sorts[2]))
		assert.JSONEq(t, idKey, sortOptionJSON(t, sorts[3]))
	})

	// Nothing but the id follows the keys a user chose. Each criterion of the default order is a column of
	// its own in the sort dialog, so it orders the results only when it was asked for.
	t.Run("with keys", func(t *testing.T) {
		t.Parallel()

		sorts := search.TestingBuildSort([]search.SortKey{{Type: search.SortLabel, Descending: true}}, "en") //nolint:exhaustruct
		require.Len(t, sorts, 2)
		assert.JSONEq(t, `{"displaySort.en":{"missing":"_last","order":"desc","unmapped_type":"keyword"}}`, sortOptionJSON(t, sorts[0]))
		assert.JSONEq(t, idKey, sortOptionJSON(t, sorts[1]))
	})

	t.Run("several keys", func(t *testing.T) {
		t.Parallel()

		sorts := search.TestingBuildSort([]search.SortKey{
			{Type: "ref", Prop: []string{"prop"}}, //nolint:exhaustruct
			{Type: search.SortTime},               //nolint:exhaustruct
		}, "en")
		require.Len(t, sorts, 3)
		assert.JSONEq(t, `{"time":{"missing":"_last","order":"asc"}}`, sortOptionJSON(t, sorts[1]))
		assert.JSONEq(t, idKey, sortOptionJSON(t, sorts[2]))
	})

	// A session whose columns are all unknown is a session without a sort, so it falls back to the default
	// order and not to the id alone.
	t.Run("unknown key skipped", func(t *testing.T) {
		t.Parallel()

		sorts := search.TestingBuildSort([]search.SortKey{{Type: "unknown"}}, "en") //nolint:exhaustruct
		require.Len(t, sorts, 4)
		assert.JSONEq(t, scoreKey, sortOptionJSON(t, sorts[0]))
		assert.JSONEq(t, timeKey, sortOptionJSON(t, sorts[1]))
		assert.JSONEq(t, labelKey, sortOptionJSON(t, sorts[2]))
		assert.JSONEq(t, idKey, sortOptionJSON(t, sorts[3]))
	})
}

func TestValidateSort(t *testing.T) {
	t.Parallel()

	refKey := func() search.SortKey {
		return search.SortKey{Type: "ref", Prop: []string{"prop"}} //nolint:exhaustruct
	}

	tests := []struct {
		name    string
		sort    []search.SortKey
		wantErr bool
	}{
		{
			name:    "empty",
			sort:    nil,
			wantErr: false,
		},
		{ //nolint:exhaustruct
			name: "grouped ref",
			sort: []search.SortKey{{Type: "ref", Prop: []string{"prop"}, Group: true}}, //nolint:exhaustruct
		},
		{ //nolint:exhaustruct
			name: "grouped and expanded ref",
			sort: []search.SortKey{{Type: "ref", Prop: []string{"prop"}, Group: true, Expand: true}}, //nolint:exhaustruct
		},
		{
			name:    "expand without group",
			sort:    []search.SortKey{{Type: "ref", Prop: []string{"prop"}, Expand: true}}, //nolint:exhaustruct
			wantErr: true,
		},
		{
			name: "second level expanded only",
			sort: []search.SortKey{
				refKey(),
				{Type: "ref", Prop: []string{"prop2"}, Group: true, Expand: true}, //nolint:exhaustruct
			},
			// The first column is not grouped, so the grouped second column does not form a leading run.
			wantErr: true,
		},
		{ //nolint:exhaustruct
			name: "both grouped, second expanded",
			sort: []search.SortKey{
				{Type: "ref", Prop: []string{"prop"}, Group: true},                //nolint:exhaustruct
				{Type: "ref", Prop: []string{"prop2"}, Group: true, Expand: true}, //nolint:exhaustruct
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errE := search.TestingValidateSort(tt.sort)
			if tt.wantErr {
				assert.Error(t, errE)
			} else {
				require.NoError(t, errE, "% -+#.1v", errE)
			}
		})
	}
}
