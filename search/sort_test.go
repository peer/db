package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/search"
)

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
