package search_test

import (
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

// TestFiltersGetTotalSaturationIntegration verifies that the available-filters total is marked as a
// lower bound ("<n>+") when a discovery terms aggregation drops distinct facet keys past its cap: a
// document with more than MaxResultsCount distinct ref properties yields more facets than are seen.
func TestFiltersGetTotalSaturationIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// One document with MaxResultsCount+1 distinct ref properties: the rel discovery terms returns the
	// top MaxResultsCount and drops the rest, so totalFacets is a lower bound.
	records := make(internalSearch.RelClaims, 0, search.MaxResultsCount+1)
	for i := range search.MaxResultsCount + 1 {
		records = append(records, refRecord(identifier.From("satProp", strconv.Itoa(i)), identifier.From("satTarget", strconv.Itoa(i)), nil))
	}
	indexDocument(t, ctx, esClient, index, relDoc("satDoc", records))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	// The total is the seen facet count marked as a lower bound; the returned facet slice is itself
	// capped at MaxResultsCount.
	assert.Equal(t, strconv.Itoa(search.MaxResultsCount)+"+", metadata["total"])
	assert.Len(t, results, search.MaxResultsCount)
}

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
	eleven := 11.0
	twenty := 20.0
	twentyOne := 21.0
	t1000 := float64(1000)
	t1001 := float64(1001)
	t2000 := float64(2000)
	t2001 := float64(2001)

	indexDocument(t, ctx, esClient, index, claimsDoc("filterDoc1", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel:    internalSearch.RelClaims{refRecord(refProp, refTarget, nil)},
		Amount: internalSearch.AmountClaims{amountRecord(amountProp, &unitID, &ten, &eleven, nil)},
		Time:   internalSearch.TimeClaims{timeRecord(timeProp, &t1000, &t1001, nil)},
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("filterDoc2", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel:    internalSearch.RelClaims{refRecord(refProp, refTarget, nil)},
		Amount: internalSearch.AmountClaims{amountRecord(amountProp, &unitID, &twenty, &twentyOne, nil)},
		Time:   internalSearch.TimeClaims{timeRecord(timeProp, &t2000, &t2001, nil)},
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	filterResults, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	// We should have 3 filters: ref, amount, and time.
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
		if len(fr.Props) > 0 {
			ids[fr.Type] = fr.Props[0]
		}
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

	queryDoc := func(id, text string) internalSearch.Document {
		doc := relDoc(id, internalSearch.RelClaims{refRecord(refProp, refTarget, nil)})
		doc.Text = map[string][]string{"en": {text}}
		return doc
	}

	indexDocument(t, ctx, esClient, index, queryDoc("queryDoc1", "searchable text"))
	indexDocument(t, ctx, esClient, index, queryDoc("queryDoc2", "other content"))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Query: "searchable",
	})

	filterResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	// With query "searchable", only 1 doc matches, so ref filter should have count 1.
	for _, fr := range filterResults {
		if fr.Type == "ref" && len(fr.Props) > 0 && fr.Props[0] == refProp.String() {
			assert.Equal(t, int64(1), fr.Count)
		}
	}
}

func TestFiltersGetAmountTimeValueDisplayQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	unitID := identifier.From("unitID")
	timeProp := identifier.From("timeProp")

	amountVal := float64(1500)
	amountValTo := float64(1501)
	timeVal := float64(1577836800)
	timeValTo := float64(1577836801)

	// The amount and time value bounds carry a formatted display label (from/toDisplay). The property names
	// are left unset, so a query can only match through a value-bound display.
	amountClaim := amountRecord(amountProp, &unitID, &amountVal, &amountValTo, nil)
	amountClaim.FromDisplay = "1500"
	amountClaim.ToDisplay = "1500"
	timeClaim := timeRecord(timeProp, &timeVal, &timeValTo, nil)
	timeClaim.FromDisplay = "2020-01-01 00:00:00"
	timeClaim.ToDisplay = "2020-01-01 00:00:00"
	indexDocument(t, ctx, esClient, index, claimsDoc("amountTimeDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountClaim},
		Time:   internalSearch.TimeClaims{timeClaim},
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// hasFacet reports whether a facet of the given type for the given prop is in the results.
	hasFacet := func(results []search.FilterResult, facetType, prop string) bool {
		for _, fr := range results {
			if fr.Type == facetType && len(fr.Props) > 0 && fr.Props[0] == prop {
				return true
			}
		}
		return false
	}

	// A query matching the amount value-bound display surfaces the amount facet but not the time facet.
	amountResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "1500*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasFacet(amountResults, "amount", amountProp.String()), "amount facet should match its value display")
	assert.False(t, hasFacet(amountResults, "time", timeProp.String()), "time facet should not match the amount value")

	// A query matching the time value-bound display surfaces the time facet but not the amount facet.
	timeResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "2020*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasFacet(timeResults, "time", timeProp.String()), "time facet should match its value display")
	assert.False(t, hasFacet(timeResults, "amount", amountProp.String()), "amount facet should not match the time value")
}

func TestFiltersGetSubAmountTimeValueDisplayQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentTo")
	subAmountProp := identifier.From("subAmountProp")
	subTimeProp := identifier.From("subTimeProp")
	unitID := identifier.From("unitID")

	amountVal := float64(1500)
	amountValTo := float64(1501)
	timeVal := float64(1577836800)
	timeValTo := float64(1577836801)

	// Sub-amount and sub-time value bounds carry the same flat from/toDisplay labels as their top-level
	// counterparts. The property names are left unset, so a query can only match through a value-bound
	// display. Both sub-claims live in the same parent claim's Sub container.
	subAmount := amountRecord(subAmountProp, &unitID, &amountVal, &amountValTo, nil)
	subAmount.FromDisplay = "1500"
	subAmount.ToDisplay = "1500"
	subTime := timeRecord(subTimeProp, &timeVal, &timeValTo, nil)
	subTime.FromDisplay = "2020-01-01 00:00:00"
	subTime.ToDisplay = "2020-01-01 00:00:00"
	indexDocument(t, ctx, esClient, index, relDoc("subAmountTimeDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, &internalSearch.ClaimTypes{ //nolint:exhaustruct
			Amount: internalSearch.AmountClaims{subAmount},
			Time:   internalSearch.TimeClaims{subTime},
		}),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// hasSubFacet reports whether a sub-facet (keyed by parentProp + prop) of the given type is in the results.
	// Sub-amount and sub-time facets are returned with type "amount"/"time" and a two-element Props slice.
	hasSubFacet := func(results []search.FilterResult, facetType, parent, prop string) bool {
		for _, fr := range results {
			if fr.Type == facetType && len(fr.Props) == 2 && fr.Props[0] == parent && fr.Props[1] == prop {
				return true
			}
		}
		return false
	}

	// A query matching the sub-amount value-bound display surfaces the sub-amount facet but not sub-time.
	amountResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "1500*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasSubFacet(amountResults, "amount", parentProp.String(), subAmountProp.String()), "sub-amount facet should match its value display")
	assert.False(t, hasSubFacet(amountResults, "time", parentProp.String(), subTimeProp.String()), "sub-time facet should not match the amount value")

	// A query matching the sub-time value-bound display surfaces the sub-time facet but not sub-amount.
	timeResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "2020*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasSubFacet(timeResults, "time", parentProp.String(), subTimeProp.String()), "sub-time facet should match its value display")
	assert.False(t, hasSubFacet(timeResults, "amount", parentProp.String(), subAmountProp.String()), "sub-amount facet should not match the time value")
}

func TestFiltersGetAmountMissingUnitIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("amountProp")
	ten := 10.0
	eleven := 11.0

	indexDocument(t, ctx, esClient, index, claimsDoc("noUnitDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(amountProp, nil, &ten, &eleven, nil)},
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	filterResults, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	// Should have exactly one amount filter with empty unit and count 1.
	assert.Len(t, filterResults, 1)
	assert.Equal(t, search.FilterResult{
		Props:    []string{amountProp.String()},
		Type:     "amount",
		Unit:     "",
		FilterID: "",
		Count:    int64(1),
	}, filterResults[0])
}

func TestFiltersGetValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	germany := identifier.From("germany")
	height := identifier.From("height")
	unitID := identifier.From("unitID")
	ten := 10.0
	eleven := 11.0

	// A reference facet "instance of" with a value "Germany".
	instanceOfRecord := namedRefRecord(instanceOf, germany, "Germany", nil)
	instanceOfRecord.PropDisplay = map[string]string{"en": "instance of"}
	indexDocument(t, ctx, esClient, index, relDoc("facetDoc1", internalSearch.RelClaims{instanceOfRecord}))
	// An amount facet "Height"; amounts have no value label, so this facet is reachable only by its name.
	heightClaim := amountRecord(height, &unitID, &ten, &eleven, nil)
	heightClaim.PropDisplay = map[string]string{"en": "Height"}
	indexDocument(t, ctx, esClient, index, claimsDoc("facetDoc2", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{heightClaim},
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	has := func(results []search.FilterResult, typ string, prop identifier.Identifier) bool {
		for _, r := range results {
			if r.Type == typ && len(r.Props) > 0 && r.Props[0] == prop.String() {
				return true
			}
		}
		return false
	}

	// Without a query both facets are available.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, "ref", instanceOf))
	assert.True(t, has(results, "amount", height))
	// The available-filters total is the count of all facets and must not change as the box is typed in.
	assert.Equal(t, "2", metadata["total"])

	// Matching a facet by its own property name keeps only that facet, but the total stays the same.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "instance*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, "ref", instanceOf))
	assert.False(t, has(results, "amount", height))
	assert.Equal(t, "2", metadata["total"])

	// Matching a reference facet by one of its value names keeps that facet too.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "germ*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, "ref", instanceOf))
	assert.False(t, has(results, "amount", height))
	assert.Equal(t, "2", metadata["total"])

	// An amount facet is reachable by its name even though its values (numbers) cannot be searched.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "heig*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, has(results, "amount", height))
	assert.False(t, has(results, "ref", instanceOf))
	assert.Equal(t, "2", metadata["total"])

	// A query that matches no facet name or value returns no facets, yet the total still reports both.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "zzz*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, has(results, "ref", instanceOf))
	assert.False(t, has(results, "amount", height))
	assert.Equal(t, "2", metadata["total"])
}

func TestFiltersGetRefDiscoveryCountValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	instanceOf := identifier.From("instanceOf")
	color := identifier.From("color")
	germany := identifier.From("germany")
	france := identifier.From("france")
	spain := identifier.From("spain")
	red := identifier.From("red")

	// labeledRefDoc builds a document with a single ref record under prop with the given property and value labels.
	labeledRefDoc := func(id string, prop, to identifier.Identifier, propLabel, toLabel string) internalSearch.Document {
		rec := namedRefRecord(prop, to, toLabel, nil)
		rec.PropDisplay = map[string]string{"en": propLabel}
		return relDoc(id, internalSearch.RelClaims{rec})
	}

	// The "instance of" reference facet has three documents but only one value ("Germany") matches "germ*".
	indexDocument(t, ctx, esClient, index, labeledRefDoc("refDoc1", instanceOf, germany, "instance of", "Germany"))
	indexDocument(t, ctx, esClient, index, labeledRefDoc("refDoc2", instanceOf, france, "instance of", "France"))
	indexDocument(t, ctx, esClient, index, labeledRefDoc("refDoc3", instanceOf, spain, "instance of", "Spain"))
	// The "color" reference facet matches neither "germ*" by value nor by its own property name.
	indexDocument(t, ctx, esClient, index, labeledRefDoc("refDoc4", color, red, "color", "red"))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	// count returns the reported count of the ref facet for prop, and whether it is present.
	count := func(results []search.FilterResult, prop identifier.Identifier) (int64, bool) {
		for _, r := range results {
			if r.Type == "ref" && len(r.Props) == 1 && r.Props[0] == prop.String() {
				return r.Count, true
			}
		}
		return 0, false
	}

	// Without a value query both facets are available at their full document counts.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok := count(results, instanceOf)
	assert.True(t, ok, "instance of facet present without query")
	assert.Equal(t, int64(3), c)
	c, ok = count(results, color)
	assert.True(t, ok, "color facet present without query")
	assert.Equal(t, int64(1), c)
	assert.Equal(t, "2", metadata["total"])

	// A value query matching only one of the three values keeps the facet at its full count (3, not 1) and
	// drops the facet that matches nothing, while the available-filters total is unchanged.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "germ*")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok = count(results, instanceOf)
	assert.True(t, ok, "instance of facet present under matching query")
	assert.Equal(t, int64(3), c)
	_, ok = count(results, color)
	assert.False(t, ok, "color facet absent under non-matching query")
	assert.Equal(t, "2", metadata["total"])

	// A query that matches nothing drops both facets but still reports the full total.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "zzz*")
	require.NoError(t, errE, "% -+#.1v", errE)
	_, ok = count(results, instanceOf)
	assert.False(t, ok, "instance of facet absent under non-matching query")
	_, ok = count(results, color)
	assert.False(t, ok, "color facet absent under non-matching query")
	assert.Equal(t, "2", metadata["total"])
}

func TestFiltersGetHasDiscoveryCountValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	colorProp := identifier.From("colorProp")
	shapeProp := identifier.From("shapeProp")

	// Three documents have a has claim, but only one of them carries the "color" has-property. Both
	// properties are has-only, so they pool into the single has facet.
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc1", internalSearch.RelClaims{namedHasRecord(colorProp, "color", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc2", internalSearch.RelClaims{namedHasRecord(shapeProp, "shape", nil, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("hasDoc3", internalSearch.RelClaims{namedHasRecord(shapeProp, "shape", nil, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	// hasCount returns the pooled has facet's count (the facet carries no Props) and whether it is present.
	hasCount := func(results []search.FilterResult) (int64, bool) {
		for _, r := range results {
			if r.Type == "has" && len(r.Props) == 0 {
				return r.Count, true
			}
		}
		return 0, false
	}

	// Without a value query the has facet reports all documents with any has claim.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok := hasCount(results)
	assert.True(t, ok, "has facet present without query")
	assert.Equal(t, int64(3), c)
	assert.Equal(t, "1", metadata["total"])

	// A value query matching only one has-property keeps the facet at its full document count (3, not 1),
	// while the available-filters total is unchanged.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "color*")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok = hasCount(results)
	assert.True(t, ok, "has facet present under matching query")
	assert.Equal(t, int64(3), c)
	assert.Equal(t, "1", metadata["total"])

	// A query matching no has-property drops the facet but still reports it in the total.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "zzz*")
	require.NoError(t, errE, "% -+#.1v", errE)
	_, ok = hasCount(results)
	assert.False(t, ok, "has facet absent under non-matching query")
	assert.Equal(t, "1", metadata["total"])
}

// TestFiltersGetPooledHasDistinctCountIntegration verifies that the pooled has facet counts distinct
// documents: a document with several pooled has-properties (boolean flags) counts once, not once per
// property, at both the top level and the sub level.
func TestFiltersGetPooledHasDistinctCountIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	flagA := identifier.From("flagA")
	flagB := identifier.From("flagB")
	flagC := identifier.From("flagC")
	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentTarget")
	subFlagA := identifier.From("subFlagA")
	subFlagB := identifier.From("subFlagB")

	// One document carrying three top-level flags and, under one parent claim, two sub-flags.
	indexDocument(t, ctx, esClient, index, relDoc("multiFlagDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, flagA, nil),
		simpleRelRecord(internalSearch.ClaimTypeHas, flagB, nil),
		simpleRelRecord(internalSearch.ClaimTypeHas, flagC, nil),
		refRecord(parentProp, parentTarget, relSub(
			simpleRelRecord(internalSearch.ClaimTypeHas, subFlagA, nil),
			simpleRelRecord(internalSearch.ClaimTypeHas, subFlagB, nil),
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	find := func(typ string, props []string) (search.FilterResult, bool) {
		for _, r := range results {
			if r.Type != typ || len(r.Props) != len(props) {
				continue
			}
			match := true
			for i := range props {
				if r.Props[i] != props[i] {
					match = false
					break
				}
			}
			if match {
				return r, true
			}
		}
		return search.FilterResult{}, false
	}

	// The one document counts once in the top-level pooled has facet, not three times.
	pooled, ok := find("has", nil)
	require.True(t, ok, "pooled has facet present")
	assert.Equal(t, int64(1), pooled.Count)

	// And once in the parent property's pooled sub-has facet, not twice.
	subPooled, ok := find("has", []string{parentProp.String()})
	require.True(t, ok, "pooled sub-has facet present")
	assert.Equal(t, int64(1), subPooled.Count)
}

// TestFiltersGetHasPoolingMigrationIntegration verifies which facet a property with has claims surfaces
// through: a has-only property joins the pooled has facet (even when its has claim carries sub-claims); a
// property that also has ref records gets its own value-list facet; a property with only valueless statements
// (here has beside none) gets a specials-only value-list facet; and a property with has claims beside an
// amount facet surfaces through that facet, whose count adds the documents reachable only through the
// specials.
func TestFiltersGetHasPoolingMigrationIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	pooledProp := identifier.From("pooledProp")
	refMixProp := identifier.From("refMixProp")
	amountMixProp := identifier.From("amountMixProp")
	noneMixProp := identifier.From("noneMixProp")
	unitID := identifier.From("unitID")
	refTarget := identifier.From("refTarget")
	subProp := identifier.From("subProp")
	subTarget := identifier.From("subTarget")

	ten := 10.0
	eleven := 11.0

	// pooledProp is has-only; the sub-claims on its has claim do not affect pooling.
	indexDocument(t, ctx, esClient, index, relDoc("pooledDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, pooledProp, relSub(refRecord(subProp, subTarget, nil))),
	}))
	// refMixProp has a has claim and a ref claim: its own value-list facet, counting both documents.
	indexDocument(t, ctx, esClient, index, relDoc("refMixHasDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, refMixProp, nil),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("refMixRefDoc", internalSearch.RelClaims{refRecord(refMixProp, refTarget, nil)}))
	// amountMixProp has a has claim and an amount claim: the amount facet counts the amount document plus the
	// has document (reachable only through the shared specials).
	indexDocument(t, ctx, esClient, index, relDoc("amountMixHasDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, amountMixProp, nil),
	}))
	indexDocument(t, ctx, esClient, index, claimsDoc("amountMixAmountDoc", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(amountMixProp, &unitID, &ten, &eleven, nil)},
	}))
	// noneMixProp has a has claim and a none claim and no valued facet anywhere: a specials-only value-list
	// facet.
	indexDocument(t, ctx, esClient, index, relDoc("noneMixHasDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeHas, noneMixProp, nil),
	}))
	indexDocument(t, ctx, esClient, index, relDoc("noneMixNoneDoc", internalSearch.RelClaims{
		simpleRelRecord(internalSearch.ClaimTypeNone, noneMixProp, nil),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	find := func(typ string, props []string) (search.FilterResult, bool) {
		for _, r := range results {
			if r.Type != typ || len(r.Props) != len(props) {
				continue
			}
			match := true
			for i := range props {
				if r.Props[i] != props[i] {
					match = false
					break
				}
			}
			if match {
				return r, true
			}
		}
		return search.FilterResult{}, false
	}

	// The pooled has facet's count is distinct documents with any has claim: pooledDoc plus the has
	// documents of the migrated and mixed properties (refMixHasDoc, amountMixHasDoc, noneMixHasDoc),
	// which are shown in their own facets. This is the documented upper bound (it matches the active
	// pooled-has availability count), not the count of documents with the pooled member property alone.
	pooled, ok := find("has", nil)
	require.True(t, ok, "pooled has facet present")
	assert.Equal(t, int64(4), pooled.Count)

	// refMixProp gets its own value-list facet counting both its documents; it must not be pooled.
	refMix, ok := find("ref", []string{refMixProp.String()})
	require.True(t, ok, "refMixProp value-list facet present")
	assert.Equal(t, int64(2), refMix.Count)

	// noneMixProp gets a specials-only value-list facet counting both its documents.
	noneMix, ok := find("ref", []string{noneMixProp.String()})
	require.True(t, ok, "noneMixProp specials-only facet present")
	assert.Equal(t, int64(2), noneMix.Count)

	// amountMixProp surfaces only through its amount facet, whose count adds the has-only document.
	amountMix, ok := find("amount", []string{amountMixProp.String()})
	require.True(t, ok, "amountMixProp amount facet present")
	assert.Equal(t, unitID.String(), amountMix.Unit)
	assert.Equal(t, int64(2), amountMix.Count)
	_, ok = find("ref", []string{amountMixProp.String()})
	assert.False(t, ok, "amountMixProp has no value-list facet")

	// The sub-claims of pooledProp's has claim are discovered as a sub facet under it, so the pooled
	// property still acts as a parent property even though it has no facet of its own at the top level.
	subFacet, ok := find("ref", []string{pooledProp.String(), subProp.String()})
	require.True(t, ok, "sub facet under the pooled has claim present")
	assert.Equal(t, int64(1), subFacet.Count)

	// No pooled property surfaces as its own top-level facet and no migrated property is pooled, so the
	// discovered facets are exactly the five above.
	_, ok = find("ref", []string{pooledProp.String()})
	assert.False(t, ok, "pooledProp has no top-level value-list facet")
	assert.Len(t, results, 5)
	assert.Equal(t, "5", metadata["total"])
}

// TestFiltersGetSubHasPooledIntegration verifies sub-level facet discovery of has claims: a has-only
// sub-property joins the parent property's pooled sub-has facet (Props carries just the parent property),
// while a sub-property that also has ref sub records gets its own sub value-list facet (Props carries the
// parent property and the sub-property).
func TestFiltersGetSubHasPooledIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentTo")
	colorProp := identifier.From("colorProp")
	mixedProp := identifier.From("mixedProp")
	mixedTarget := identifier.From("mixedTarget")

	// colorProp is a has-only sub-property: pooled into the parent property's sub-has facet.
	indexDocument(t, ctx, esClient, index, relDoc("subPooledDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(simpleRelRecord(internalSearch.ClaimTypeHas, colorProp, nil))),
	}))
	// mixedProp has both a has and a ref sub record: its own sub value-list facet.
	indexDocument(t, ctx, esClient, index, relDoc("subMixedDoc", internalSearch.RelClaims{
		refRecord(parentProp, parentTarget, relSub(
			simpleRelRecord(internalSearch.ClaimTypeHas, mixedProp, nil),
			refRecord(mixedProp, mixedTarget, nil),
		)),
	}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	find := func(typ string, props []string) (search.FilterResult, bool) {
		for _, r := range results {
			if r.Type != typ || len(r.Props) != len(props) {
				continue
			}
			match := true
			for i := range props {
				if r.Props[i] != props[i] {
					match = false
					break
				}
			}
			if match {
				return r, true
			}
		}
		return search.FilterResult{}, false
	}

	// The parent property itself is a top-level value-list facet over both documents.
	parentFacet, ok := find("ref", []string{parentProp.String()})
	require.True(t, ok, "parent property value-list facet present")
	assert.Equal(t, int64(2), parentFacet.Count)

	// The pooled sub-has facet's count is distinct documents with any sub-has under parentProp:
	// subPooledDoc (via colorProp) and subMixedDoc (via the mixed mixedProp, shown in its own sub
	// facet). This is the documented upper bound, not the count of documents with the pooled member
	// sub-property alone.
	subPooled, ok := find("has", []string{parentProp.String()})
	require.True(t, ok, "pooled sub-has facet present")
	assert.Equal(t, int64(2), subPooled.Count)

	// mixedProp migrated to its own sub value-list facet.
	subMixed, ok := find("ref", []string{parentProp.String(), mixedProp.String()})
	require.True(t, ok, "mixedProp sub value-list facet present")
	assert.Equal(t, int64(1), subMixed.Count)

	// colorProp has no sub value-list facet of its own.
	_, ok = find("ref", []string{parentProp.String(), colorProp.String()})
	assert.False(t, ok, "colorProp has no sub value-list facet")

	assert.Len(t, results, 3)
	assert.Equal(t, "3", metadata["total"])
}

// TestFiltersGetActiveFilterIntegration verifies that active filters are appended to the results with their
// FilterID set and an availability count, ahead of the inactive discovery entries.
func TestFiltersGetActiveFilterIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	refProp := identifier.From("refProp")
	target1 := identifier.From("target1")
	target2 := identifier.From("target2")

	indexDocument(t, ctx, esClient, index, relDoc("activeDoc1", internalSearch.RelClaims{refRecord(refProp, target1, nil)}))
	indexDocument(t, ctx, esClient, index, relDoc("activeDoc2", internalSearch.RelClaims{refRecord(refProp, target2, nil)}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{ //nolint:exhaustruct
		Filters: []search.Filter{{ //nolint:exhaustruct
			Prop: []identifier.Identifier{refProp},
			Ref:  &search.RefFilter{To: []search.ToValue{{ID: target1}}, Direct: nil},
		}},
	})

	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)

	// The active entry comes first with the filter's ID; its count is the facet's availability in
	// the rest-of-search scope (excluding the filter's own selection), so both documents count.
	// The discovery entry for the same facet follows without an ID, computed within the filtered
	// search scope (one matching document).
	assert.Equal(t, []search.FilterResult{
		{Props: []string{refProp.String()}, Type: "ref", Unit: "", FilterID: session.Filters[0].ID.String(), Count: 2},
		{Props: []string{refProp.String()}, Type: "ref", Unit: "", FilterID: "", Count: 1},
	}, results)
	// The total counts only the discovered facets, not the active-filter entries.
	assert.Equal(t, "1", metadata["total"])
}

func TestFiltersGetSubRefDiscoveryCountValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	parentProp := identifier.From("parentProp")
	parentTarget := identifier.From("parentTo")
	instanceOf := identifier.From("instanceOf")
	color := identifier.From("color")
	germany := identifier.From("germany")
	france := identifier.From("france")
	spain := identifier.From("spain")
	red := identifier.From("red")

	// subRefDoc builds a document with a single sub ref record under (parentProp, prop) with the value label,
	// nested in a parent claim's Sub container.
	subRefDoc := func(id string, prop, to identifier.Identifier, toLabel string) internalSearch.Document {
		return relDoc(id, internalSearch.RelClaims{
			refRecord(parentProp, parentTarget, relSub(namedRefRecord(prop, to, toLabel, nil))),
		})
	}

	// The (parentProp, "instance of") sub-reference facet has three documents but only one value matches "germ*".
	indexDocument(t, ctx, esClient, index, subRefDoc("subRefDoc1", instanceOf, germany, "Germany"))
	indexDocument(t, ctx, esClient, index, subRefDoc("subRefDoc2", instanceOf, france, "France"))
	indexDocument(t, ctx, esClient, index, subRefDoc("subRefDoc3", instanceOf, spain, "Spain"))
	// The (parentProp, "color") sub-reference facet matches nothing under "germ*".
	indexDocument(t, ctx, esClient, index, subRefDoc("subRefDoc4", color, red, "red"))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})
	enabledLanguages := internalSearch.EnabledLanguages(nil)

	// count returns the reported count of the sub-reference facet for (parentProp, prop), and whether present.
	count := func(results []search.FilterResult, prop identifier.Identifier) (int64, bool) {
		for _, r := range results {
			if r.Type == "ref" && len(r.Props) == 2 && r.Props[0] == parentProp.String() && r.Props[1] == prop.String() {
				return r.Count, true
			}
		}
		return 0, false
	}

	// Without a value query both sub-facets are available at their full document counts. The parent claims
	// also surface as a top-level value-list facet, so the total counts three facets.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok := count(results, instanceOf)
	assert.True(t, ok, "instance of sub-facet present without query")
	assert.Equal(t, int64(3), c)
	c, ok = count(results, color)
	assert.True(t, ok, "color sub-facet present without query")
	assert.Equal(t, int64(1), c)
	assert.Equal(t, "3", metadata["total"])

	// A value query matching only one value keeps the sub-facet at its full count (3, not 1) and drops the
	// sub-facet that matches nothing, while the available-filters total is unchanged.
	results, metadata, errE = search.FiltersGet(ctx, getSearchService, session, searchLangs(enabledLanguages), "germ*")
	require.NoError(t, errE, "% -+#.1v", errE)
	c, ok = count(results, instanceOf)
	assert.True(t, ok, "instance of sub-facet present under matching query")
	assert.Equal(t, int64(3), c)
	_, ok = count(results, color)
	assert.False(t, ok, "color sub-facet absent under non-matching query")
	assert.Equal(t, "3", metadata["total"])
}

// TestFiltersGetSpecialsActiveValuedFacetIntegration verifies that an active specials filter
// renders its path's real valued facet with correct availability counts: selecting the unknown
// special on an amount property keeps the amount facet (with its unit and full reachable-through
// count) instead of degrading to a specials-only value-list facet, and active entry counts are
// computed excluding the path's own selections.
func TestFiltersGetSpecialsActiveValuedFacetIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	amountProp := identifier.From("priceProp")
	unitID := identifier.From("unitEur")

	v10 := float64(10)
	v20 := float64(20)
	v30 := float64(30)
	indexAmountDoc(t, ctx, esClient, index, "priceDoc1", amountProp, unitID, &v10)
	indexAmountDoc(t, ctx, esClient, index, "priceDoc2", amountProp, unitID, &v20)
	indexAmountDoc(t, ctx, esClient, index, "priceDoc3", amountProp, unitID, &v30)
	indexDocument(t, ctx, esClient, index, claimsDoc("priceDocUnknown", internalSearch.ClaimTypes{ //nolint:exhaustruct
		Rel: internalSearch.RelClaims{simpleRelRecord(internalSearch.ClaimTypeUnknown, amountProp, nil)},
	}))
	refreshIndex(t, ctx, esClient, index)

	// With nothing selected, discovery offers the amount facet with the reachable-through count:
	// the three value documents plus the valueless one.
	emptySession := createSession(t, ctx, search.SessionData{})
	results, _, errE := search.FiltersGet(ctx, getSearchService, emptySession, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.FilterResult{
		{Props: []string{amountProp.String()}, Type: "amount", Unit: unitID.String(), FilterID: "", Count: 4},
	}, results)

	unknownFilter := search.Filter{ //nolint:exhaustruct
		Prop:     []identifier.Identifier{amountProp},
		Specials: &search.SpecialsFilter{Missing: false, None: false, Unknown: true, HasProperty: false},
	}
	gte := float64(9)
	lte := float64(31)
	rangeFilter := search.Filter{ //nolint:exhaustruct
		Prop:   []identifier.Identifier{amountProp},
		Amount: &search.AmountFilter{Unit: &unitID, Gte: &gte, Lte: &lte, Exists: false},
	}

	// With only the unknown special selected, the active entry still renders the property's real
	// facet (the amount facet with its unit) with the full reachable-through count, and no
	// specials-only value-list facet is offered beside it.
	unknownSession := createSession(t, ctx, search.SessionData{Filters: []search.Filter{unknownFilter}}) //nolint:exhaustruct
	results, _, errE = search.FiltersGet(ctx, getSearchService, unknownSession, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, []search.FilterResult{
		{Props: []string{amountProp.String()}, Type: "amount", Unit: unitID.String(), FilterID: unknownSession.Filters[0].ID.String(), Count: 4},
	}, results)

	// With only the range selected, the active amount entry's count stays the facet's full
	// reachable-through count, not the count within its own selection.
	rangeSession := createSession(t, ctx, search.SessionData{Filters: []search.Filter{rangeFilter}}) //nolint:exhaustruct
	results, _, errE = search.FiltersGet(ctx, getSearchService, rangeSession, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, results)
	assert.Equal(t, search.FilterResult{
		Props: []string{amountProp.String()}, Type: "amount", Unit: unitID.String(),
		FilterID: rangeSession.Filters[0].ID.String(), Count: 4,
	}, results[0])

	// With the range and the unknown special selected together, documents match through either
	// (OR within the path) and both active entries render the amount facet with the same count.
	bothSession := createSession(t, ctx, search.SessionData{Filters: []search.Filter{rangeFilter, unknownFilter}}) //nolint:exhaustruct
	searchResults, _, errE := search.ResultsGet(ctx, getSearchService, &bothSession.SessionData, nil, 0)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, searchResults, 4)
	results, _, errE = search.FiltersGet(ctx, getSearchService, bothSession, nil, "")
	require.NoError(t, errE, "% -+#.1v", errE)
	activeEntries := 0
	for _, r := range results {
		if r.FilterID == "" {
			continue
		}
		activeEntries++
		assert.Equal(t, "amount", r.Type, "entry %+v", r)
		assert.Equal(t, unitID.String(), r.Unit, "entry %+v", r)
		assert.Equal(t, int64(4), r.Count, "entry %+v", r)
	}
	assert.Equal(t, 2, activeEntries)
}

// TestFiltersGetDiscoveryBeyondCapValueQueryIntegration verifies that the filtered value-query
// discovery pass surfaces a facet that matches the typed text but ranks beyond the unfiltered
// discovery's Size cap by document count. A low-document-count property whose name matches is found
// even though a full cap's worth of higher-count filler properties rank ahead of it, and it does
// not grow the value-query-independent total (which stays a saturated lower bound).
func TestFiltersGetDiscoveryBeyondCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// MaxResultsCount filler ref properties, each present in two documents (document count 2), so they
	// all rank ahead of the target property (document count 1) and fill the unfiltered discovery cap.
	fillers := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		fillers = append(fillers, refRecord(identifier.From("filler", strconv.Itoa(i)), identifier.From("fillerTarget", strconv.Itoa(i)), nil))
	}
	indexDocument(t, ctx, esClient, index, relDoc("fillerDoc1", fillers))
	indexDocument(t, ctx, esClient, index, relDoc("fillerDoc2", fillers))

	// The target property: one document, and a distinctive property name to match by.
	findMeProp := identifier.From("findMeProp")
	findMe := refRecord(findMeProp, identifier.From("findMeTarget"), nil)
	findMe.PropNaming = map[string][]string{"en": {"findmeuniquename"}}
	findMe.PropDisplay = map[string]string{"en": "findmeuniquename"}
	indexDocument(t, ctx, esClient, index, relDoc("findMeDoc", internalSearch.RelClaims{findMe}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// Searching the target property's name surfaces its facet, even though it ranks beyond the cap.
	results, metadata, errE := search.FiltersGet(ctx, getSearchService, session, nil, "findmeuniquename*")
	require.NoError(t, errE, "% -+#.1v", errE)

	found := false
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) == 1 && r.Props[0] == findMeProp.String() {
			found = true
			assert.Equal(t, int64(1), r.Count)
		}
	}
	assert.True(t, found, "the beyond-cap target facet was surfaced by its name")

	// The total stays the value-query-independent saturated lower bound: the cap's worth of filler
	// facets, marked "+"; the beyond-cap surfaced facet does not grow it.
	assert.Equal(t, strconv.Itoa(search.MaxResultsCount)+"+", metadata["total"])
}

// TestFiltersGetQuotedPhraseNoStopWordIntegration verifies that a quoted phrase in the filter-pane
// value query keeps its stop words: "in language" as a phrase matches a property named "in language"
// but not a facet whose only possible match is a value named "language". Previously the English
// stop-word filter dropped "in" from the phrase on the stemmed naming field, collapsing it to
// "language"; the quoted phrase now routes to the unstemmed (no-stop) naming sub-field.
func TestFiltersGetQuotedPhraseNoStopWordIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	inLangProp := identifier.From("inLangProp")
	instOfProp := identifier.From("instOfProp")
	someVal := identifier.From("someVal")
	langVal := identifier.From("langVal")

	// A property named "in language".
	inLangRec := refRecord(inLangProp, someVal, nil)
	inLangRec.PropNaming = map[string][]string{"en": {"in language"}}
	inLangRec.PropDisplay = map[string]string{"en": "in language"}
	indexDocument(t, ctx, esClient, index, relDoc("inLangDoc", internalSearch.RelClaims{inLangRec}))

	// A property "instance of" whose value is named "language".
	instRec := refRecord(instOfProp, langVal, nil)
	instRec.PropNaming = map[string][]string{"en": {"instance of"}}
	instRec.PropDisplay = map[string]string{"en": "instance of"}
	instRec.ToNaming = map[string][]string{"en": {"language"}}
	instRec.ToDisplay = map[string]string{"en": "language"}
	indexDocument(t, ctx, esClient, index, relDoc("instOfDoc", internalSearch.RelClaims{instRec}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	hasRefFacet := func(results []search.FilterResult, prop string) bool {
		for _, r := range results {
			if r.Type == "ref" && len(r.Props) == 1 && r.Props[0] == prop {
				return true
			}
		}
		return false
	}

	// Quoted phrase (the frontend appends no wildcard when the query ends with a quote): the
	// "in language" property matches by its name; the "instance of" property, whose only possible
	// match is its value named "language", does not, because the phrase keeps "in".
	quoted, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, `"in language"`)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasRefFacet(quoted, inLangProp.String()), "the in-language property matches the quoted phrase")
	assert.False(t, hasRefFacet(quoted, instOfProp.String()), "a value named language must not match the quoted phrase \"in language\"")

	// Unquoted, "in" is a required AND term (matched via the non-stop und bucket), so the value named
	// "language" is excluded there too; the in-language property still matches.
	unquoted, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "in language*")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.True(t, hasRefFacet(unquoted, inLangProp.String()), "the in-language property matches the unquoted query")
	assert.False(t, hasRefFacet(unquoted, instOfProp.String()), "a value named language must not match the unquoted in language")
}

// TestFiltersGetSubDiscoveryBeyondCapValueQueryIntegration verifies that the filtered sub-discovery
// pass surfaces a sub facet (parentProp, subProp) matching the typed text but ranking beyond the
// unfiltered sub-discovery's Size cap by document count within its parent property.
func TestFiltersGetSubDiscoveryBeyondCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	locationProp := identifier.From("locationProp")
	venue := identifier.From("venue")

	// MaxResultsCount filler sub-properties under one parent claim, present in two documents (sub
	// document count 2), filling the unfiltered sub-discovery cap ahead of the target (document count 1).
	fillerSubs := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		fillerSubs = append(fillerSubs, refRecord(identifier.From("subFiller", strconv.Itoa(i)), identifier.From("subFillerTarget", strconv.Itoa(i)), nil))
	}
	fillerParent := refRecord(locationProp, venue, relSub(fillerSubs...))
	indexDocument(t, ctx, esClient, index, relDoc("subFillerDoc1", internalSearch.RelClaims{fillerParent}))
	indexDocument(t, ctx, esClient, index, relDoc("subFillerDoc2", internalSearch.RelClaims{fillerParent}))

	// The target sub-property: one document, with a distinctive property name to match by.
	targetSubProp := identifier.From("targetSubProp")
	targetSub := refRecord(targetSubProp, identifier.From("targetSubTarget"), nil)
	targetSub.PropNaming = map[string][]string{"en": {"findmesubuniquename"}}
	targetSub.PropDisplay = map[string]string{"en": "findmesubuniquename"}
	indexDocument(t, ctx, esClient, index, relDoc("subTargetDoc", internalSearch.RelClaims{refRecord(locationProp, venue, relSub(targetSub))}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// Searching the target sub-property's name surfaces its (parentProp, subProp) sub facet, even
	// though it ranks beyond the sub cap within the parent property.
	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "findmesubuniquename*")
	require.NoError(t, errE, "% -+#.1v", errE)

	found := false
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) == 2 && r.Props[0] == locationProp.String() && r.Props[1] == targetSubProp.String() {
			found = true
			assert.Equal(t, int64(1), r.Count)
		}
	}
	assert.True(t, found, "the beyond-cap target sub facet was surfaced by its name")
}

// TestFiltersGetBeyondParentCapValueQueryIntegration verifies that the filtered parent enumeration
// surfaces a sub facet under a parent property that itself ranks beyond the parent-property Size cap
// by document count: the parent has no bucket in the unfiltered sub discovery, yet its matching sub
// facet is still found.
func TestFiltersGetBeyondParentCapValueQueryIntegration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	esClient, getSearchService, index := initES(t)

	// MaxResultsCount filler parent properties, each with a sub-claim, present in two documents
	// (parent document count 2), filling the parent-property cap ahead of the target (document count 1).
	fillerParents := make(internalSearch.RelClaims, 0, search.MaxResultsCount)
	for i := range search.MaxResultsCount {
		sub := refRecord(identifier.From("pFillerSub", strconv.Itoa(i)), identifier.From("pFillerSubTarget", strconv.Itoa(i)), nil)
		fillerParents = append(fillerParents, refRecord(identifier.From("pFiller", strconv.Itoa(i)), identifier.From("pFillerTarget", strconv.Itoa(i)), relSub(sub)))
	}
	indexDocument(t, ctx, esClient, index, relDoc("pFillerDoc1", fillerParents))
	indexDocument(t, ctx, esClient, index, relDoc("pFillerDoc2", fillerParents))

	// The target parent property: one document (so it ranks beyond the parent cap), with a sub-claim
	// whose distinctive name we search by.
	targetParentProp := identifier.From("targetParentProp")
	targetSubProp := identifier.From("targetSubProp")
	targetSub := refRecord(targetSubProp, identifier.From("targetSubTarget"), nil)
	targetSub.PropNaming = map[string][]string{"en": {"findmeparentbeyonduniquename"}}
	targetSub.PropDisplay = map[string]string{"en": "findmeparentbeyonduniquename"}
	indexDocument(t, ctx, esClient, index, relDoc("targetParentDoc", internalSearch.RelClaims{refRecord(targetParentProp, identifier.From("targetParentTarget"), relSub(targetSub))}))
	refreshIndex(t, ctx, esClient, index)

	session := createSession(t, ctx, search.SessionData{})

	// Searching the target sub-property's name surfaces its (parentProp, subProp) sub facet, even
	// though the parent property ranks beyond the parent-property cap.
	results, _, errE := search.FiltersGet(ctx, getSearchService, session, nil, "findmeparentbeyonduniquename*")
	require.NoError(t, errE, "% -+#.1v", errE)

	found := false
	for _, r := range results {
		if r.Type == "ref" && len(r.Props) == 2 && r.Props[0] == targetParentProp.String() && r.Props[1] == targetSubProp.String() {
			found = true
			assert.Equal(t, int64(1), r.Count)
		}
	}
	assert.True(t, found, "the sub facet under the beyond-parent-cap property was surfaced by its name")
}
