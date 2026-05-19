package search

import (
	"context"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// Get retrieves time filter data for search results.
func (f *TimeFilter) Get(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant, prop identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filter := esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String()))
	return histogramFilterGet(
		ctx, getSearchService, query,
		prop, "claims.time", filter,
		"claims.time.from", "claims.time.to", "claims.time.range",
		f.Gte, f.Lte,
	)
}
