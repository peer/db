package search_test

import (
	"fmt"
	"testing"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/search"
)

// refClaim builds a reference claim for prop pointing to target with the given confidence.
func refClaim(prop, target identifier.Identifier, confidence document.Confidence) document.ReferenceClaim {
	return document.ReferenceClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
		Prop:      document.Reference{ID: prop},
		To:        document.Reference{ID: target},
	}
}

// stringClaim builds a string claim for prop with the given value and confidence.
func stringClaim(prop identifier.Identifier, value string, confidence document.Confidence) document.StringClaim {
	return document.StringClaim{
		CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: confidence},
		Prop:      document.Reference{ID: prop},
		String:    value,
	}
}

func TestDuplicatesQuery(t *testing.T) {
	t.Parallel()

	exclude := identifier.From("doc")
	instanceOf := identifier.From("instanceOf")
	class := identifier.From("class")
	name := identifier.From("name")
	ident := identifier.From("identifier")
	other := identifier.From("other")
	langs := []string{"en", "und"}

	// reverse is the must_not clause excluding candidates that assert they are distinct from this
	// document (the symmetric direction of DISTINCT_FROM); it is present in every query.
	reverse := fmt.Sprintf(
		`{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":%q}}},{"term":{"claims.ref.to":{"value":%q}}}]}}}}`,
		internalCore.DistinctFromPropID.String(), exclude.String(),
	)

	tests := []struct {
		Name string
		Doc  *document.D
		Want string
	}{
		{
			Name: "single reference",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					Reference: document.ReferenceClaims{refClaim(instanceOf, class, document.HighConfidence)},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],`+
					`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":"claims.ref","query":{"bool":{"must":[`+
					`{"term":{"claims.ref.prop":{"value":%q}}},{"term":{"claims.ref.to":{"value":%q}}}]}}}}}}]}}`,
				exclude.String(), reverse, instanceOf.String(), class.String(),
			),
		},
		{
			Name: "identifier and string weights",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					Identifier: document.IdentifierClaims{{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:      document.Reference{ID: ident},
						Value:     "Q42",
					}},
					String: document.StringClaims{stringClaim(name, "Berlin", document.HighConfidence)},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],"should":[`+
					`{"constant_score":{"boost":10,"filter":{"nested":{"path":"claims.id","query":{"bool":{"must":[`+
					`{"term":{"claims.id.prop":{"value":%q}}},{"match_phrase":{"claims.id.value":{"query":"Q42"}}}]}}}}}},`+
					`{"constant_score":{"boost":5,"filter":{"nested":{"path":"claims.string","query":{"bool":{"must":[`+
					`{"term":{"claims.string.prop":{"value":%q}}},`+
					`{"bool":{"minimum_should_match":1,"should":[`+
					`{"match":{"claims.string.string.en":{"fuzziness":"AUTO","operator":"and","query":"Berlin"}}},`+
					`{"match":{"claims.string.string.und":{"fuzziness":"AUTO","operator":"and","query":"Berlin"}}}]}}]}}}}}}]}}`,
				exclude.String(), reverse, ident.String(), name.String(),
			),
		},
		{
			// DISTINCT_FROM does not produce a scoring clause; its target is excluded from results instead.
			Name: "distinct from excludes target",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					String:    document.StringClaims{stringClaim(name, "Berlin", document.HighConfidence)},
					Reference: document.ReferenceClaims{refClaim(internalCore.DistinctFromPropID, other, document.HighConfidence)},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},{"term":{"id":{"value":%q}}},%s],"should":[`+
					`{"constant_score":{"boost":5,"filter":{"nested":{"path":"claims.string","query":{"bool":{"must":[`+
					`{"term":{"claims.string.prop":{"value":%q}}},`+
					`{"bool":{"minimum_should_match":1,"should":[`+
					`{"match":{"claims.string.string.en":{"fuzziness":"AUTO","operator":"and","query":"Berlin"}}},`+
					`{"match":{"claims.string.string.und":{"fuzziness":"AUTO","operator":"and","query":"Berlin"}}}]}}]}}}}}}]}}`,
				exclude.String(), other.String(), reverse, name.String(),
			),
		},
		{
			Name: "low confidence and duplicates skipped",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					Reference: document.ReferenceClaims{
						refClaim(instanceOf, class, document.HighConfidence),
						// Same property and target: deduplicated to a single clause.
						refClaim(instanceOf, class, document.HighConfidence),
						// Below LowConfidence: skipped.
						refClaim(name, class, document.Confidence(0.4)),
					},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],`+
					`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":"claims.ref","query":{"bool":{"must":[`+
					`{"term":{"claims.ref.prop":{"value":%q}}},{"term":{"claims.ref.to":{"value":%q}}}]}}}}}}]}}`,
				exclude.String(), reverse, instanceOf.String(), class.String(),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			query := search.TestingDuplicatesQuery(tt.Doc, langs, exclude)
			require.NotNil(t, query)
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, query))
		})
	}
}

// TestDuplicatesQueryNoClaims verifies that a document with no matchable claims yields a nil query,
// so there is nothing to search for.
func TestDuplicatesQueryNoClaims(t *testing.T) {
	t.Parallel()

	exclude := identifier.From("doc")
	langs := []string{"en", "und"}

	// Nil claims.
	doc := &document.D{CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}}, Claims: nil}
	assert.Nil(t, search.TestingDuplicatesQuery(doc, langs, exclude))

	// Only None and Unknown claims, which are not matched.
	doc = &document.D{
		CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
		Claims: &document.ClaimTypes{
			None: document.NoneClaims{{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: identifier.From("prop")},
			}},
		},
	}
	assert.Nil(t, search.TestingDuplicatesQuery(doc, langs, exclude))
}

// TestDuplicatesGetNoClaims verifies that DuplicatesGet short-circuits to an empty result without
// querying ElasticSearch when the document has no matchable claims.
func TestDuplicatesGetNoClaims(t *testing.T) {
	t.Parallel()

	exclude := identifier.From("doc")
	doc := &document.D{CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}}, Claims: nil}

	getSearchService := func() *esSearch.Search {
		t.Fatal("ElasticSearch must not be queried when there are no matchable claims")
		return nil
	}

	results, errE := search.DuplicatesGet(t.Context(), getSearchService, doc, exclude, []string{"en", "und"}, 5)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, results)
}
