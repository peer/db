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
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
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

func TestDuplicatesQuery(t *testing.T) { //nolint:maintidx
	t.Parallel()

	exclude := identifier.From("doc")
	instanceOf := identifier.From("instanceOf")
	class := identifier.From("class")
	name := identifier.From("name")
	ident := identifier.From("identifier")
	other := identifier.From("other")
	desc := identifier.From("description")
	amountProp := identifier.From("amountProp")
	timeProp := identifier.From("timeProp")
	langs := []string{"en", "und"}

	// An amount's precision window is symmetric around its value, so 10 with precision 2 occupies
	// [9, 11] and 20 occupies [19, 21].
	amountFrom := document.Amount("10")
	amountTo := document.Amount("20")
	amountPrecision := float64(2)
	timeFrom := document.Time("2020")
	timeTo := document.Time("2021")
	timePrecision := document.TimePrecisionYear

	// reverse is the must_not clause excluding candidates that assert they are distinct from this
	// document (the symmetric direction of DISTINCT_FROM); it is present in every query.
	reverse := fmt.Sprintf(
		`{"nested":{"path":"claims.rel","query":{"bool":{"must":[{"term":{"claims.rel.prop":{"value":%q}}},{"term":{"claims.rel.to":{"value":%q}}}]}}}}`,
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
					`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":"claims.rel","query":{"bool":{"must":[`+
					`{"term":{"claims.rel.prop":{"value":%q}}},{"term":{"claims.rel.to":{"value":%q}}}]}}}}}}]}}`,
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
			// HTML is stripped to plain text (as the indexer strips it) and matched per language with
			// operator AND; it contributes the low html weight.
			Name: "html body matched as stripped text",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					HTML: document.HTMLClaims{{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:      document.Reference{ID: desc},
						HTML:      "<p>Hello <strong>world</strong></p>",
					}},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],"should":[`+
					`{"constant_score":{"boost":1,"filter":{"nested":{"path":"claims.html","query":{"bool":{"must":[`+
					`{"term":{"claims.html.prop":{"value":%q}}},`+
					`{"bool":{"minimum_should_match":1,"should":[`+
					`{"match":{"claims.html.html.en":{"operator":"and","query":"Hello world"}}},`+
					`{"match":{"claims.html.html.und":{"operator":"and","query":"Hello world"}}}]}}]}}}}}}]}}`,
				exclude.String(), reverse, desc.String(),
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
					`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":"claims.rel","query":{"bool":{"must":[`+
					`{"term":{"claims.rel.prop":{"value":%q}}},{"term":{"claims.rel.to":{"value":%q}}}]}}}}}}]}}`,
				exclude.String(), reverse, instanceOf.String(), class.String(),
			),
		},
		{
			// A has claim matches rel records with the has claimType for the same property: the clause carries the
			// claimType discriminator because a bare property term would also match ref, none, and
			// unknown records.
			Name: "has claim matched by claim type and property",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					Has: document.HasClaims{{
						CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:      document.Reference{ID: other},
					}},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],`+
					`"should":[{"constant_score":{"boost":1,"filter":{"nested":{"path":"claims.rel","query":{"bool":{"must":[`+
					`{"term":{"claims.rel.claimType":{"value":"has"}}},{"term":{"claims.rel.prop":{"value":%q}}}]}}}}}}]}}`,
				exclude.String(), reverse, other.String(),
			),
		},
		{
			// An interval occupies the window from its lower bound's window to its upper bound's, so
			// candidates whose own window overlaps it match.
			Name: "amount interval spans both bounds",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					AmountInterval: document.AmountIntervalClaims{{ //nolint:exhaustruct
						CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:          document.Reference{ID: amountProp},
						From:          &amountFrom,
						FromPrecision: &amountPrecision,
						To:            &amountTo,
						ToPrecision:   &amountPrecision,
					}},
				},
			},
			Want: amountIntervalWant(exclude, reverse, amountProp, 9, 21),
		},
		{
			// The bounds are stated in decreasing order, which is the same interval.
			Name: "amount interval stated in decreasing order",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					AmountInterval: document.AmountIntervalClaims{{ //nolint:exhaustruct
						CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:          document.Reference{ID: amountProp},
						From:          &amountTo,
						FromPrecision: &amountPrecision,
						To:            &amountFrom,
						ToPrecision:   &amountPrecision,
					}},
				},
			},
			Want: amountIntervalWant(exclude, reverse, amountProp, 9, 21),
		},
		{
			// A bound stated as unknown collapses the interval to a point at the stated bound, which is
			// how the indexer stores it too, so the clause is the one a point claim there produces.
			Name: "amount interval with an unknown bound collapses to a point",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					AmountInterval: document.AmountIntervalClaims{{ //nolint:exhaustruct
						CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:          document.Reference{ID: amountProp},
						From:          &amountFrom,
						FromPrecision: &amountPrecision,
						ToIsUnknown:   true,
					}},
				},
			},
			Want: amountIntervalWant(exclude, reverse, amountProp, 9, 11),
		},
		{
			// A bound stated as absent leaves the interval unbounded on that side, which overlaps a large
			// part of the corpus and is no duplicate signal, so only the reference claim contributes.
			Name: "amount interval unbounded on one side contributes nothing",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					Reference: document.ReferenceClaims{refClaim(instanceOf, class, document.HighConfidence)},
					AmountInterval: document.AmountIntervalClaims{{ //nolint:exhaustruct
						CoreClaim:   document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:        document.Reference{ID: amountProp},
						FromIsNone:  true,
						To:          &amountTo,
						ToPrecision: &amountPrecision,
					}},
				},
			},
			Want: fmt.Sprintf(
				`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],`+
					`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":"claims.rel","query":{"bool":{"must":[`+
					`{"term":{"claims.rel.prop":{"value":%q}}},{"term":{"claims.rel.to":{"value":%q}}}]}}}}}}]}}`,
				exclude.String(), reverse, instanceOf.String(), class.String(),
			),
		},
		{
			// A time interval is matched the same way, against the time collection.
			Name: "time interval spans both bounds",
			Doc: &document.D{
				CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
				Claims: &document.ClaimTypes{
					TimeInterval: document.TimeIntervalClaims{{ //nolint:exhaustruct
						CoreClaim:     document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
						Prop:          document.Reference{ID: timeProp},
						From:          &timeFrom,
						FromPrecision: &timePrecision,
						To:            &timeTo,
						ToPrecision:   &timePrecision,
					}},
				},
			},
			// 2020-01-01T00:00:00Z through 2022-01-01T00:00:00Z, in seconds since the epoch.
			Want: timeIntervalWant(exclude, reverse, timeProp, 1577836800, 1640995200),
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

// amountIntervalWant is the expected query for a document whose only clause is an amount interval on
// prop occupying [from, to].
func amountIntervalWant(exclude identifier.Identifier, reverse string, prop identifier.Identifier, from, to int) string {
	return rangeIntervalWant("claims.amount", exclude, reverse, prop, from, to)
}

// timeIntervalWant is amountIntervalWant for a time interval.
func timeIntervalWant(exclude identifier.Identifier, reverse string, prop identifier.Identifier, from, to int) string {
	return rangeIntervalWant("claims.time", exclude, reverse, prop, from, to)
}

func rangeIntervalWant(path string, exclude identifier.Identifier, reverse string, prop identifier.Identifier, from, to int) string {
	return fmt.Sprintf(
		`{"bool":{"minimum_should_match":1,"must_not":[{"term":{"id":{"value":%q}}},%s],`+
			`"should":[{"constant_score":{"boost":2,"filter":{"nested":{"path":%q,"query":{"bool":{"must":[`+
			`{"term":{"%s.prop":{"value":%q}}},{"range":{"%s.range":{"gte":%d,"lte":%d}}}]}}}}}}]}}`,
		exclude.String(), reverse, path, path, prop.String(), path, from, to,
	)
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

// TestDuplicatesGetOrderIntegration verifies that candidates scoring the same are listed by id. Scores are
// sums of constant per-field weights, so two candidates matching the same fields of the document score
// exactly the same and nothing but the id can order them. Which of them the limit keeps would otherwise
// follow the index's internal document order, and two indexes holding the same documents do not have to
// agree on it.
func TestDuplicatesGetOrderIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	identProp := identifier.From("duplicateOrderIdentifier")
	const value = "Q42"

	lower := identifier.From("duplicateOrderCandidateOne")
	higher := identifier.From("duplicateOrderCandidateTwo")
	if lower.String() > higher.String() {
		lower, higher = higher, lower
	}

	// The candidate with the greater id is indexed first, so the document order of the index is the reverse
	// of the order the candidates are expected in and cannot be what puts them in it. Both carry the same
	// identifier value, which is the only thing the document below is matched on, so they score the same.
	for _, id := range []identifier.Identifier{higher, lower} {
		indexDocument(t, ctx, esClient, index, idClaimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
			Identifier: internalSearch.IdentifierClaims{{ //nolint:exhaustruct
				Prop:  identProp,
				Value: value,
			}},
		}))
	}
	refreshIndex(t, ctx, esClient, index)

	exclude := identifier.From("duplicateOrderDoc")
	doc := &document.D{
		CoreDocument: document.CoreDocument{ID: exclude, Base: []string{"x", "doc"}},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{{
				CoreClaim: document.CoreClaim{ID: identifier.New(), Confidence: document.HighConfidence},
				Prop:      document.Reference{ID: identProp},
				Value:     value,
			}},
		},
	}

	results, errE := search.DuplicatesGet(ctx, getSearchService, doc, exclude, []string{"en", "und"}, 5)
	require.NoError(t, errE, "% -+#.1v", errE)
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []string{lower.String(), higher.String()}, ids)

	// The limit therefore keeps the same candidate every time, rather than whichever one the index happened
	// to return first.
	limited, errE := search.DuplicatesGet(ctx, getSearchService, doc, exclude, []string{"en", "und"}, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	limitedIDs := make([]string, 0, len(limited))
	for _, r := range limited {
		limitedIDs = append(limitedIDs, r.ID)
	}
	assert.Equal(t, []string{lower.String()}, limitedIDs)
}
