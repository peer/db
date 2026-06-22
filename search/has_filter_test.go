package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestHasFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	color := identifier.From("color")
	shape := identifier.From("shape")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc1"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: color, PropDisplay: map[string]string{"en": "Color"}}}, //nolint:exhaustruct
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:exhaustruct
		ID: identifier.From("hasDoc2"),
		Claims: internalSearch.ClaimTypes{ //nolint:exhaustruct
			Has: internalSearch.HasClaims{{Prop: shape, PropDisplay: map[string]string{"en": "Shape"}}}, //nolint:exhaustruct
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	enabledLanguages := internalSearch.EnabledLanguages(nil)
	f := search.HasFilter{}

	// Without a value query both has-properties are listed.
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(nil), "", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
		{ID: shape.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])

	// The value query (a prefix wildcard, as the frontend appends) narrows the facet to the matching property.
	results, metadata, errE = f.Get(ctx, getSearchService, session.ToQuery(nil), "col*", enabledLanguages)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.HasFilterResult{
		{ID: color.String(), Count: 1},
	}, results)
	assert.Equal(t, "1", metadata["total"])
}
