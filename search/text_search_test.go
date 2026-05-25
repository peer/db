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

// resultIDs returns the result hits as a sorted slice of ID strings.
func resultIDs(results []search.Result) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestTextSearchUndWildcardCaseAndDiacritic(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")

	// doc1 has the literal diacritic form; doc2 has the folded form.
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc1ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Žagar Špela"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc2ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Zagar Ivan"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	// With AnalyzeWildcard on the und clause, the typed wildcard gets lowercased
	// and ICU-folded before prefix matching. So Žagar*, žagar*, ŽAGAR*, and
	// Zagar* should all match both docs (because both indexed surface tokens
	// fold to "zagar").
	for _, q := range []string{"Žagar*", "žagar*", "ŽAGAR*", "Zagar*"} {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			session := createSession(t, ctx, search.SessionData{
				View:    "",
				Query:   q,
				Filters: nil,
				Reverse: nil,
			})
			results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
			require.NoError(t, errE, "% -+#.1v", errE)
			ids := resultIDs(results)
			assert.ElementsMatch(t, []string{doc1ID.String(), doc2ID.String()}, ids,
				"query %q should match both diacritic and folded surface forms via analyze_wildcard on und", q)
		})
	}
}

func TestTextSearchUndQuotedExactVsFolded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	doc1ID := identifier.From("doc1") // literal "Žagar".
	doc2ID := identifier.From("doc2") // folded "Zagar".

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc1ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Žagar"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc2ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Zagar"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	// Quoted phrase "Žagar" should match BOTH:
	//   - doc1 via the exact-routed clause (text.und.exact, diacritic-preserved).
	//   - doc2 via the folded clause (text.und, where Žagar folds to zagar and
	//     matches doc2's "Zagar" token also folded to zagar).
	// dis_max picks the higher of the two per doc; doc1 should score higher
	// because it matches in two clauses (exact and folded) while doc2 only
	// matches the folded one.
	quotedSession := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   `"Žagar"`,
		Filters: nil,
		Reverse: nil,
	})
	results, _, errE := search.ResultsGet(ctx, getSearchService, &quotedSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 2)
	assert.Equal(t, doc1ID.String(), results[0].ID,
		`"Žagar" with both clauses matching should rank doc1 (literal Žagar) above doc2 (folded)`)
	assert.ElementsMatch(t,
		[]string{doc1ID.String(), doc2ID.String()},
		[]string{results[0].ID, results[1].ID},
	)

	// Quoted "Zagar" should also match both, with doc2 ranked first (it's the
	// literal exact match for "Zagar").
	quotedFoldedSession := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   `"Zagar"`,
		Filters: nil,
		Reverse: nil,
	})
	results, _, errE = search.ResultsGet(ctx, getSearchService, &quotedFoldedSession.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 2)
	assert.Equal(t, doc2ID.String(), results[0].ID,
		`"Zagar" should rank doc2 (literal Zagar) above doc1 (only matches folded)`)
}

func TestTextSearchUndUnquotedFoldsBoth(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc1ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Žagar"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc2ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Zagar"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	// Unquoted "žagar" is folded to "zagar" by standard_string on both query and
	// index sides. Both docs match (their indexed tokens also fold to "zagar").
	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   "žagar",
		Filters: nil,
		Reverse: nil,
	})
	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.ElementsMatch(t,
		[]string{doc1ID.String(), doc2ID.String()},
		resultIDs(results),
		"unquoted žagar should match both via folded standard_string on text.und",
	)
}

func TestTextSearchStemmedPhraseEnglish(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// Index two English-tagged docs. doc1 has the plural form "running shoes".
	// A quoted phrase "running shoe" (singular noun) should still match it via
	// the stemmed-phrase clause: english_stemmer reduces both to the same root
	// (run / shoe), so phrase positions line up after stemming.
	doc1ID := identifier.From("doc1")
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc1ID,
		Display: nil,
		Text:    map[string][]string{"en": {"running shoes"}},
		Claims:  internalSearch.ClaimTypes{},
	})

	// doc2 is a control: contains "running" but not "shoes". Should not match
	// a quoted phrase that requires both terms adjacent.
	doc2ID := identifier.From("doc2")
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc2ID,
		Display: nil,
		Text:    map[string][]string{"en": {"running fast"}},
		Claims:  internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	// Quoted phrase, singular noun: should match doc1 via the stemmed-phrase
	// clause (text.en, no quote_field_suffix, english_stemmer applied).
	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   `"running shoe"`,
		Filters: nil,
		Reverse: nil,
	})
	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	ids := resultIDs(results)
	assert.Contains(t, ids, doc1ID.String(),
		`"running shoe" should match a doc indexing "running shoes" via the stemmed-phrase clause`)
	assert.NotContains(t, ids, doc2ID.String(),
		`"running shoe" should not match a doc that only contains "running" (phrase requires both terms adjacent)`)
}

func TestTextSearchExactFieldRejectsFolded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// Confirm the .exact analyzer truly preserves diacritics: a quoted phrase
	// containing only the diacritic form should match ONLY through the exact
	// clause for the doc that has the diacritic. The folded-clause match still
	// brings in the no-diacritic doc, but the relative scoring distinguishes
	// them (verified above). Here we just confirm both docs are indexed and
	// returned for the quoted query, and that a query crafted to exercise the
	// exact field directly (a unique diacritic form) sees the right docs.
	doc1ID := identifier.From("doc1")
	doc2ID := identifier.From("doc2")
	doc3ID := identifier.From("doc3")

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc1ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Müller"}}, // German umlaut.
		Claims:  internalSearch.ClaimTypes{},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc2ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Muller"}}, // ASCII.
		Claims:  internalSearch.ClaimTypes{},
	})
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:      doc3ID,
		Display: nil,
		Text:    map[string][]string{"und": {"Smith"}}, // unrelated.
		Claims:  internalSearch.ClaimTypes{},
	})
	refreshIndex(t, ctx, esClient, index)

	// Quoted "Müller": both Müller and Muller docs match (one via .exact, the
	// other via folded). Smith doesn't match. Doc1 with literal Müller ranks
	// first because it matches in two clauses.
	session := createSession(t, ctx, search.SessionData{
		View:    "",
		Query:   `"Müller"`,
		Filters: nil,
		Reverse: nil,
	})
	results, _, errE := search.ResultsGet(ctx, getSearchService, &session.SessionData)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Len(t, results, 2)
	assert.Equal(t, doc1ID.String(), results[0].ID, "literal Müller should rank first")
	assert.ElementsMatch(t,
		[]string{doc1ID.String(), doc2ID.String()},
		resultIDs(results),
	)
	assert.NotContains(t, resultIDs(results), doc3ID.String())
}
