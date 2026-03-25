package search_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

func TestFiltersGetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	refTarget := identifier.From("refTarget")
	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unitID")
	timeProp := identifier.From("timeProp")

	ten := 10.0
	twenty := 20.0
	t1000 := float64(1000)
	t2000 := float64(2000)

	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:dupl
		ID: identifier.From("filterDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop:        amountProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Unit:        &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &ten,
					LessThan:           nil,
					LessThanOrEqual:    &ten,
				},
				From:        &ten,
				FromDisplay: "",
				To:          &ten,
				ToDisplay:   "",
			}},
			Time: internalSearch.TimeClaims{{
				Prop:        timeProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &t1000,
					LessThan:           nil,
					LessThanOrEqual:    &t1000,
				},
				From:        &t1000,
				FromDisplay: "",
				To:          &t1000,
				ToDisplay:   "",
			}},
			Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{ //nolint:dupl
		ID: identifier.From("filterDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop:        amountProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Unit:        &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &twenty,
					LessThan:           nil,
					LessThanOrEqual:    &twenty,
				},
				From:        &twenty,
				FromDisplay: "",
				To:          &twenty,
				ToDisplay:   "",
			}},
			Time: internalSearch.TimeClaims{{
				Prop:        timeProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &t2000,
					LessThan:           nil,
					LessThanOrEqual:    &t2000,
				},
				From:        &t2000,
				FromDisplay: "",
				To:          &t2000,
				ToDisplay:   "",
			}},
			Link: nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: nil,
	}
	createSession(t, ctx, session)

	filterResults, metadata, errE := search.FiltersGet(ctx, getSearchService, session)
	require.NoError(t, errE)

	// We should have 3 filters: rel, amount, and time.
	assert.Len(t, filterResults, 3)
	assert.Equal(t, "3", metadata["total"])

	// All filters have count 2. Sort by type for deterministic comparison.
	sort.Slice(filterResults, func(i, j int) bool {
		return filterResults[i].Type < filterResults[j].Type
	})

	// Verify each filter has the expected ID, count, and type.
	types := map[string]bool{}
	for _, fr := range filterResults {
		types[fr.Type] = true
		assert.Equal(t, int64(2), fr.Count)
	}
	assert.True(t, types["ref"])
	assert.True(t, types["amount"])
	assert.True(t, types["time"])

	// Verify IDs match expected props.
	ids := map[string]string{}
	for _, fr := range filterResults {
		ids[fr.Type] = fr.ID
	}
	assert.Equal(t, refProp.String(), ids["ref"])
	assert.Equal(t, amountProp.String(), ids["amount"])
	assert.Equal(t, timeProp.String(), ids["time"])
}

func TestFiltersGetWithQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	refTarget := identifier.From("refTarget")
	stringProp := identifier.From("stringProp")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("queryDoc1"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String: internalSearch.StringClaims{{
				Prop:        stringProp,
				PropDisplay: nil,
				PropNaming:  nil,
				String:      map[string]string{"en": "searchable text"},
			}},
			HTML:   nil,
			Amount: nil,
			Time:   nil,
			Link:   nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
		},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("queryDoc2"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String: internalSearch.StringClaims{{
				Prop:        stringProp,
				PropDisplay: nil,
				PropNaming:  nil,
				String:      map[string]string{"en": "other content"},
			}},
			HTML:   nil,
			Amount: nil,
			Time:   nil,
			Link:   nil,
			Reference: internalSearch.ReferenceClaims{{
				Prop:          refProp,
				PropDisplay:   nil,
				PropNaming:    nil,
				To:            refTarget,
				ToDisplay:     nil,
				ToNaming:      nil,
				ToPath:        nil,
				ToDisplayPath: nil,
				Reference:     nil,
			}},
			Has:     nil,
			None:    nil,
			Unknown: nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "searchable",
		Filters: nil,
	}
	createSession(t, ctx, session)

	filterResults, _, errE := search.FiltersGet(ctx, getSearchService, session)
	require.NoError(t, errE)

	// With query "searchable", only 1 doc matches, so rel filter should have count 1.
	for _, fr := range filterResults {
		if fr.Type == "ref" && fr.ID == refProp.String() {
			assert.Equal(t, int64(1), fr.Count)
		}
	}
}

func TestFiltersGetAmountMissingUnitIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	ten := 10.0

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID: identifier.From("noUnitDoc"),
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop:        amountProp,
				PropDisplay: nil,
				PropNaming:  nil,
				Unit:        nil,
				Range: internalSearch.RangeFloat{
					GreaterThan:        nil,
					GreaterThanOrEqual: &ten,
					LessThan:           nil,
					LessThanOrEqual:    &ten,
				},
				From:        &ten,
				FromDisplay: "",
				To:          &ten,
				ToDisplay:   "",
			}},
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
		},
	})
	refreshIndex(t, ctx, esClient, index)

	session := &search.Session{
		ID:      nil,
		Version: 0,
		View:    "",
		Query:   "",
		Filters: nil,
	}
	createSession(t, ctx, session)

	filterResults, _, errE := search.FiltersGet(ctx, getSearchService, session)
	require.NoError(t, errE)

	// Should have exactly one amount filter with empty unit and count 1.
	assert.Len(t, filterResults, 1)
	assert.Equal(t, search.FilterResult{
		ID:    amountProp.String(),
		Count: int64(1),
		Type:  "amount",
		Unit:  "",
	}, filterResults[0])
}
