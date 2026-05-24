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
	missingNestedQuery := esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String()))
	return histogramFilterGet(
		ctx, getSearchService, query,
		missingNestedQuery, "claims.time", filter,
		"claims.time.from", "claims.time.to", "claims.time.range",
		f.Gte, f.Lte,
	)
}

// GetSubTime retrieves sub-time filter data for search results. It aggregates
// claims.subTime values for a given (parentProp, prop) combination,
// optionally restricted to listed parentTo values for cross-filtering with a
// sibling parent ref filter.
func (f *TimeFilter) GetSubTime(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64),
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filterMusts := []types.QueryVariant{
		esdsl.NewTermQuery("claims.subTime.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subTime.prop", esdsl.NewFieldValue().String(prop.String())),
	}
	if len(parentToRestrictions) > 0 {
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subTime.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}
	filter := esdsl.NewBoolQuery().Must(filterMusts...)
	return histogramSubFilterGet(
		ctx, getSearchService, query,
		parentProp, prop, "claims.subTime", filter,
		"claims.subTime.from", "claims.subTime.to", "claims.subTime.range",
		f.Gte, f.Lte,
	)
}
