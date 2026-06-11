package search_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v9"
	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

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
	testutils.RequireNoESError(t, err)
}

// refreshIndex forces an ES index refresh so documents are searchable.
func refreshIndex(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string) { //nolint:revive
	t.Helper()

	_, err := esClient.Indices.Refresh().Index(index).Do(ctx)
	testutils.RequireNoESError(t, err)
}

// indexAmountDoc indexes a document carrying a single point-amount claim equal to
// value under amountProp with unitID. It seeds amount-filter tests.
func indexAmountDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, amountProp, unitID identifier.Identifier, value *float64) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          identifier.From(id),
		Display:     nil,
		Text:        nil,
		Time:        nil,
		LastUpdated: nil,
		Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: value, LessThan: nil, LessThanOrEqual: value,
				},
				From: value, FromDisplay: "", To: value, ToDisplay: "",
			}},
			Time:      nil,
			Link:      nil,
			Reference: nil,
			Has:       nil,
			None:      nil,
			Unknown:   nil,
			SubRef:    nil,
			SubAmount: nil,
			SubTime:   nil,
			SubHas:    nil,
		},
	})
}

// indexScoreDoc indexes a document carrying the given English text and counts.score.
// It seeds counts.score ranking-boost tests.
func indexScoreDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string, id identifier.Identifier, text string, score *int) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		DisplaySort: nil,
		ID:          id,
		Display:     nil,
		Text:        map[string][]string{"en": {text}},
		Time:        nil,
		LastUpdated: nil,
		Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: score},
		Claims: internalSearch.ClaimTypes{
			Identifier: nil,
			String:     nil,
			HTML:       nil,
			Amount:     nil,
			Time:       nil,
			Link:       nil,
			Reference:  nil,
			Has:        nil,
			None:       nil,
			Unknown:    nil,
			SubRef:     nil,
			SubAmount:  nil,
			SubTime:    nil,
			SubHas:     nil,
		},
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

	//nolint:dupl
	for _, tc := range []struct {
		id    string
		value *float64
	}{
		{"timeDoc1", &t1000},
		{"timeDoc2", &t5000},
		{"timeDoc3", &t9000},
	} {
		indexDocument(t, ctx, esClient, index, internalSearch.Document{
			DisplaySort: nil,
			ID:          identifier.From(tc.id),
			Display:     nil,
			Text:        nil,
			Time:        nil,
			LastUpdated: nil,
			Counts:      internalSearch.Counts{References: nil, Claims: nil, Score: nil},
			Claims: internalSearch.ClaimTypes{
				Identifier: nil,
				String:     nil,
				HTML:       nil,
				Amount:     nil,
				Time: internalSearch.TimeClaims{{
					Prop: timeProp, PropDisplay: nil, PropNaming: nil,
					Range: internalSearch.RangeFloat{
						GreaterThan: nil, GreaterThanOrEqual: tc.value, LessThan: nil, LessThanOrEqual: tc.value,
					},
					From: tc.value, FromDisplay: "", To: tc.value, ToDisplay: "",
				}},
				Link:      nil,
				Reference: nil,
				Has:       nil,
				None:      nil,
				Unknown:   nil,
				SubRef:    nil,
				SubAmount: nil,
				SubTime:   nil,
				SubHas:    nil,
			},
		})
	}
	refreshIndex(t, ctx, esClient, index)
}

// createSession is a test helper that creates a search session from SessionData.
// It generates Base/ID for the session and any filters that lack them.
func createSession(t *testing.T, ctx context.Context, data search.SessionData) *search.Session { //nolint:revive
	t.Helper()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}

	// Generate Base/ID for filters that don't have them.
	for i := range data.Filters {
		if len(data.Filters[i].Base) == 0 {
			filterBase := append(base, "FILTER", identifier.New().String()) //nolint:gocritic
			data.Filters[i].Base = filterBase
			filterID := identifier.From(filterBase...)
			data.Filters[i].ID = &filterID
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
