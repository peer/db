package search_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestRelFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	relProp := identifier.From("relProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("relDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Reference: nil,
			Relation: internalSearch.RelationClaims{{
				Prop: relProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil, Relation: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("relDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Reference: nil,
			Relation: internalSearch.RelationClaims{{
				Prop: relProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil, Relation: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("relDoc3"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Reference: nil,
			Relation: internalSearch.RelationClaims{{
				Prop: relProp, PropDisplay: nil, PropNaming: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil, Relation: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
	createSession(t, ctx, session)

	results, metadata, errE := search.RelFilterGet(ctx, getSearchService, *session.ID, relProp)
	require.NoError(t, errE)

	// Results are sorted by count descending: target1 (count 2) first, target2 (count 1) second.
	assert.Equal(t, []search.RelFilterResult{
		{ID: target1.String(), Count: 2},
		{ID: target2.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

func TestRelFilterGetNotFoundIntegration(t *testing.T) {
	t.Parallel()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()
	nonExistentID := identifier.From("nonExistent")
	prop := identifier.From("prop")

	_, _, errE := search.RelFilterGet(ctx, nil, nonExistentID, prop)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}
