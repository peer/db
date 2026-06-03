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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	"gitlab.com/peerdb/peerdb/search"
)

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
		_, err := esClient.Indices.Delete(index).IgnoreUnavailable(true).Do(context.Background())
		assert.NoError(t, err)
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
	require.NoError(t, err)
}

// refreshIndex forces an ES index refresh so documents are searchable.
func refreshIndex(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index string) { //nolint:revive
	t.Helper()

	_, err := esClient.Indices.Refresh().Index(index).Do(ctx)
	require.NoError(t, err)
}

// indexAmountDoc indexes a document carrying a single point-amount claim equal to
// value under amountProp with unitID. It seeds amount-filter tests.
func indexAmountDoc(t *testing.T, ctx context.Context, esClient *elasticsearch.TypedClient, index, id string, amountProp, unitID identifier.Identifier, value *float64) { //nolint:revive,lll
	t.Helper()

	indexDocument(t, ctx, esClient, index, internalSearch.Document{
		ID:              identifier.From(id),
		Display:         nil,
		Text:            nil,
		Time:            nil,
		ReferencesCount: nil,
		ClaimsCount:     nil,
		Claims: internalSearch.ClaimTypes{
			Amount: internalSearch.AmountClaims{{
				Prop: amountProp, PropDisplay: nil, PropNaming: nil, Unit: &unitID,
				Range: internalSearch.RangeFloat{
					GreaterThan: nil, GreaterThanOrEqual: value, LessThan: nil, LessThanOrEqual: value,
				},
				From: value, FromDisplay: "", To: value, ToDisplay: "",
			}},
			Time: nil, Reference: nil, Has: nil, None: nil, Unknown: nil, SubRef: nil, SubAmount: nil, SubTime: nil, SubHas: nil,
		},
	})
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

	errE := search.CreateSession(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)

	return session
}
