package search_test

import (
	"bytes"
	"context"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/search"
)

// siteContext returns ctx with a minimal site stored in it so that site-aware code (such as
// SessionData.Validate, which calls waf.MustGetSite) works in tests. The site has no
// LanguagePriority, so the session language resolves to the package default language.
func siteContext(ctx context.Context) context.Context {
	return waf.WithSite[*internalSite.Site](ctx, &internalSite.Site{})
}

// searchLangs builds the *search.Languages a filter-search entry point takes: the given enabled
// languages for the indexed-text queries, and the same set minus the undetermined language (which has no
// special-value labels) for the special-value label search. For these default-language test sites that
// makes the special set [en], matching what a default-language request produces, so the special search
// scopes to English.
func searchLangs(enabledLanguages []string) *search.Languages {
	special := slices.DeleteFunc(slices.Clone(enabledLanguages), func(lang string) bool {
		return lang == document.UndeterminedLanguage
	})
	return &search.Languages{Enabled: enabledLanguages, Special: special}
}

// initES creates and configures an ES client and a test index.
// It returns the client, a search service factory, and the index name.
func initES(t *testing.T) (*elasticsearch.TypedClient, func() *esSearch.Search, string) {
	t.Helper()

	if os.Getenv("ELASTIC") == "" {
		t.Skip("ELASTIC is not available")
	}

	ctx := t.Context()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	esClient, errE := internalSearch.GetClient(cleanhttp.DefaultPooledClient(), logger, os.Getenv("ELASTIC"))
	require.NoError(t, errE, "% -+#.1v", errE)

	index := "s" + strings.ToLower(identifier.New().String())

	t.Cleanup(func() {
		// We do not use t.Context() because we want an active context, not a canceled one.
		errE := internalSearch.DeleteIndex(context.Background(), esClient, index)
		assert.NoError(t, errE, "% -+#.1v", errE)
	})

	errE = internalSearch.EnsureIndex(ctx, esClient, index, 1, nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	getSearchService := func() *esSearch.Search {
		return esClient.Search().Index(index)
	}

	return esClient, getSearchService, index
}

// indexDocument indexes a document into ES using the internal search.Document struct.
func indexDocument(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string, doc internalSearch.Document) { //nolint:revive
	t.Helper()

	data, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)
	_, err := esClient.Index(index).Id(doc.ID.String()).Raw(bytes.NewReader(data)).Do(ctx)
	testutils.RequireNoESError(ctx, t, err)
}

// refreshIndex forces an ES index refresh so documents are searchable.
func refreshIndex(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string) { //nolint:revive
	t.Helper()

	_, err := esClient.Indices.Refresh().Index(index).Do(ctx)
	testutils.RequireNoESError(ctx, t, err)
}

// claimsDoc builds a Document with the given ID and claims and nothing else.
func claimsDoc(id string, claims internalSearch.ClaimTypes) internalSearch.Document {
	return internalSearch.Document{
		ID:              identifier.From(id),
		Display:         nil,
		DisplaySort:     nil,
		Text:            nil,
		Time:            nil,
		LastUpdated:     nil,
		Counts:          internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims:          claims,
		ReadableByRoles: nil,
		ReadableByUsers: nil,
	}
}

// indexSortDoc indexes a document which carries no claims, only the fields the default sort order uses:
// the given earliest time (nil for none) and the given English display-sort label. An empty displaySort
// leaves the document without any display-sort label at all, which is what documents without a rendered
// display label look like.
func indexSortDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, tm *float64, displaySort string) { //nolint:revive
	t.Helper()

	var ds map[string]string
	if displaySort != "" {
		ds = map[string]string{"en": displaySort}
	}
	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:              identifier.From(id),
		Display:         nil,
		DisplaySort:     ds,
		Text:            nil,
		Time:            tm,
		LastUpdated:     nil,
		Counts:          internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims:          internalSearch.ClaimTypes{Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil},
		ReadableByRoles: nil,
		ReadableByUsers: nil,
	})
}

// refRecord builds a ref rel record for prop pointing at to, with a self hierarchy path and
// IsLeaf true, matching what the converter produces for a flat (no hierarchy) target. sub is the
// record's Sub container (nil for none).
func refRecord(prop, to identifier.Identifier, sub *internalSearch.ClaimTypes) internalSearch.RelClaim {
	target := to
	return internalSearch.RelClaim{
		ClaimType:     internalSearch.ClaimTypeRef,
		Prop:          prop,
		PropDisplay:   nil,
		PropNaming:    nil,
		PropSortKey:   nil,
		To:            &target,
		ToDisplay:     nil,
		ToNaming:      nil,
		ToSortKey:     nil,
		ToPath:        []string{internalSearch.SelfHierarchyPathPrefix + to.String()},
		ToParent:      nil,
		ToDisplayPath: nil,
		ToPathSortKey: nil,
		IsLeaf:        true,
		Sub:           sub,
	}
}

// simpleRelRecord builds a has, none, or unknown rel record for prop, with the given Sub container.
func simpleRelRecord(claimType string, prop identifier.Identifier, sub *internalSearch.ClaimTypes) internalSearch.RelClaim {
	return internalSearch.RelClaim{
		ClaimType:     claimType,
		Prop:          prop,
		PropDisplay:   nil,
		PropNaming:    nil,
		PropSortKey:   nil,
		To:            nil,
		ToDisplay:     nil,
		ToNaming:      nil,
		ToSortKey:     nil,
		ToPath:        nil,
		ToParent:      nil,
		ToDisplayPath: nil,
		ToPathSortKey: nil,
		IsLeaf:        false,
		Sub:           sub,
	}
}

// amountRecord builds an amount record for prop with the given unit (possibly nil) and window
// edges, mirroring the converter's point and interval shapes: a known from is the inclusive range
// lower bound and a known to the exclusive range upper bound, while a nil endpoint leaves its
// field absent and uses a MaxFloat64 sentinel range bound instead.
func amountRecord(prop identifier.Identifier, unit *identifier.Identifier, from, to *float64, sub *internalSearch.ClaimTypes) internalSearch.AmountClaim {
	rangeFloat := internalSearch.RangeFloat{
		GreaterThan: nil, GreaterThanOrEqual: from, LessThan: to, LessThanOrEqual: nil,
	}
	if from == nil {
		lower := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &lower
	}
	if to == nil {
		upper := math.MaxFloat64
		rangeFloat.LessThan = nil
		rangeFloat.LessThanOrEqual = &upper
	}
	return internalSearch.AmountClaim{
		Prop: prop, PropDisplay: nil, PropNaming: nil, PropSortKey: nil, Unit: unit,
		Range: rangeFloat,
		From:  from, FromDisplay: "", To: to, ToDisplay: "",
		Sub: sub,
	}
}

// timeRecord builds a time record for prop with the given window edges, mirroring the converter's
// point and interval shapes the same way amountRecord does.
func timeRecord(prop identifier.Identifier, from, to *float64, sub *internalSearch.ClaimTypes) internalSearch.TimeClaim {
	rangeFloat := internalSearch.RangeFloat{
		GreaterThan: nil, GreaterThanOrEqual: from, LessThan: to, LessThanOrEqual: nil,
	}
	if from == nil {
		lower := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &lower
	}
	if to == nil {
		upper := math.MaxFloat64
		rangeFloat.LessThan = nil
		rangeFloat.LessThanOrEqual = &upper
	}
	return internalSearch.TimeClaim{
		Prop: prop, PropDisplay: nil, PropNaming: nil, PropSortKey: nil,
		Range: rangeFloat,
		From:  from, FromDisplay: "", To: to, ToDisplay: "",
		Sub: sub,
	}
}

// indexAmountDoc indexes a document carrying a single point-amount claim of the given
// value with precision 1 under amountProp with unitID. It mirrors what convertAmount
// produces: the endpoints are the edges of the symmetric precision window
// [value-0.5, value+0.5) and the indexed range upper bound is exclusive.
func indexAmountDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, amountProp, unitID identifier.Identifier, value *float64) { //nolint:revive,lll
	t.Helper()

	from := *value - 0.5
	to := *value + 0.5
	indexDocument(t, ctx, esClient, index, claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(amountProp, &unitID, &from, &to, nil)},
	}))
}

// indexAmountIntervalDoc indexes a document carrying a single interval amount claim under
// amountProp with the given unit (possibly nil), with optionally open endpoints, mirroring
// how the converter indexes them: from and to are window edges with from indexed as the
// inclusive range lower bound and to as the exclusive range upper bound, while an open
// (none) endpoint leaves its field absent and uses a MaxFloat64 sentinel range bound instead.
func indexAmountIntervalDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, amountProp identifier.Identifier, unit *identifier.Identifier, from, to *float64) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
		Amount: internalSearch.AmountClaims{amountRecord(amountProp, unit, from, to, nil)},
	}))
}

// indexTimePointDoc indexes a document carrying a single point-time claim of the given
// value at second precision under timeProp. It mirrors what convertTime produces: the
// endpoints are the edges of the precision window [value, value+1) and the indexed range
// upper bound is exclusive. This is also the shape the converter produces for an interval
// claim with one unknown endpoint, which it collapses to a point at the known endpoint.
func indexTimePointDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, timeProp identifier.Identifier, value *float64) { //nolint:revive,lll
	t.Helper()

	from := *value
	to := *value + 1
	indexDocument(t, ctx, esClient, index, claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
		Time: internalSearch.TimeClaims{timeRecord(timeProp, &from, &to, nil)},
	}))
}

// indexScoreDoc indexes a document carrying the given English text and counts.score.
// It seeds counts.score ranking-boost tests.
func indexScoreDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string, id identifier.Identifier, text string, score *int) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:              id,
		Display:         nil,
		DisplaySort:     nil,
		Text:            map[string][]string{"en": {text}},
		Time:            nil,
		LastUpdated:     nil,
		Counts:          internalSearch.Counts{References: nil, Claims: nil, Score: score},
		Claims:          internalSearch.ClaimTypes{Rel: nil, Amount: nil, Time: nil, Identifier: nil, String: nil, HTML: nil, Link: nil},
		ReadableByRoles: nil,
		ReadableByUsers: nil,
	})
}

// seedTimeFilterDocs indexes three documents each carrying a single point-time
// claim under timeProp (at 1000, 5000 and 9000) and refreshes the index. It seeds
// the time-filter integration tests.
func seedTimeFilterDocs(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string, timeProp identifier.Identifier) { //nolint:revive
	t.Helper()

	t1000 := float64(1000)
	t5000 := float64(5000)
	t9000 := float64(9000)

	indexTimePointDoc(t, ctx, esClient, index, "timeDoc1", timeProp, &t1000)
	indexTimePointDoc(t, ctx, esClient, index, "timeDoc2", timeProp, &t5000)
	indexTimePointDoc(t, ctx, esClient, index, "timeDoc3", timeProp, &t9000)
	refreshIndex(t, ctx, esClient, index)
}

// indexTimeIntervalDoc indexes a document carrying a single interval time claim under
// timeProp, with optionally open endpoints, mirroring how convertTimeInterval indexes
// them: a known from is stored in the from field and as the range lower bound, a known
// to is stored in the to field and as the strict range upper bound, while an open (none)
// endpoint leaves its field absent and uses a MaxFloat64 sentinel range bound instead.
func indexTimeIntervalDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, timeProp identifier.Identifier, from, to *float64) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, claimsDoc(id, internalSearch.ClaimTypes{ //nolint:exhaustruct
		Time: internalSearch.TimeClaims{timeRecord(timeProp, from, to, nil)},
	}))
}

// newPathResolver builds a search.HierarchyPathsResolver backed by a map from value id to its indexed
// hierarchy path strings ("<hierProp>:<root>/.../<self>"), so tests can inject a selected value's ancestors
// without an Elasticsearch round-trip. An id not in the map resolves to no paths.
func newPathResolver(paths map[identifier.Identifier][]string) search.HierarchyPathsResolver {
	return func(_ context.Context, id identifier.Identifier) ([]string, errors.E) {
		return paths[id], nil
	}
}

// createSession is a test helper that creates a search session from SessionData.
// It generates Base/ID for the session and any filters or prefilters that lack them.
func createSession(t *testing.T, ctx context.Context, data search.SessionData) *search.Session { //nolint:revive
	t.Helper()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}

	// Generate Base/ID for filters that don't have them.
	for _, filters := range [][]search.Filter{data.Filters, data.Prefilters} {
		for i := range filters {
			if len(filters[i].Base) == 0 {
				filterBase := append(base, "FILTER", identifier.New().String()) //nolint:gocritic
				filters[i].Base = filterBase
				filterID := identifier.From(filterBase...)
				filters[i].ID = &filterID
			}
		}
	}

	session := &search.Session{
		SessionData: data,
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}

	errE := search.CreateSession(siteContext(ctx), session)
	require.NoError(t, errE, "% -+#.1v", errE)

	return session
}
