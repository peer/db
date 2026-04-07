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

func TestRefFilterGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc3"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Create a session with a ref filter so we can look up the filter by ID.
	session := createSession(t, ctx, search.SessionData{
		View:  "",
		Query: "",
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref: &search.RefFilter{
				To:      []search.ToValue{{ID: target1}},
				Missing: false,
			},
		}},
	})

	results, metadata, errE := session.Filters[0].Ref.Get(ctx, getSearchService, session.ToQueryExcluding(*session.Filters[0].ID), session.Filters[0].Prop[0])
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results are sorted by count descending: target1 (count 2) first, target2 (count 1) second.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 2},
		{ID: target2.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetInactiveIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target2, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	// Create a session without any filters (inactive filter scenario).
	session := createSession(t, ctx, search.SessionData{})

	// Query for ref filter values using the session's full query and prop from outside the session.
	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(), refProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results order is non-deterministic when counts are equal.
	assert.ElementsMatch(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1},
		{ID: target2.String(), Count: 1},
	}, results)
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// Doc with the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	// Doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: nil,
			Has:       nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	// Another doc without the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc3"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: nil,
			Has:       nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(), refProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Results should include target1 (count 1) and __MISSING__ (count 2), sorted by count descending.
	assert.Equal(t, []search.RefFilterResult{
		{ID: search.MissingRefFilterID, Count: 2},
		{ID: target1.String(), Count: 1},
	}, results)
	// Total includes the missing bucket.
	assert.Equal(t, "2", metadata["total"])
}

func TestRefFilterGetNoMissingIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")

	// All docs have the ref prop.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("refDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil, String: nil, HTML: nil, Amount: nil, Time: nil, Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop: refProp, PropDisplay: nil, PropNaming: nil,
				To: target1, ToDisplay: nil, ToNaming: nil, ToPath: nil, ToDisplayPath: nil,
			}},
			Has: nil, None: nil, Unknown: nil,
			SubReference: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	f := search.RefFilter{}
	results, metadata, errE := f.Get(ctx, getSearchService, session.ToQuery(), refProp)
	require.NoError(t, errE, "% -+#.1v", errE)

	// No missing bucket since all documents have the prop.
	assert.Equal(t, []search.RefFilterResult{
		{ID: target1.String(), Count: 1},
	}, results)
	assert.Equal(t, "1", metadata["total"])
}

func TestRefFilterGetNotFoundIntegration(t *testing.T) {
	t.Parallel()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()
	nonExistentID := identifier.From("nonExistent")

	_, errE := search.GetSession(ctx, nonExistentID)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}
