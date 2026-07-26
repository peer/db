package search

import (
	"context"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// Get retrieves time filter data for a top-level property facet: the histogram (or single bucket),
// the identity counts (exists, specials, missing, universe, other value types), and the range
// metadata.
func (f *TimeFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filter := propTerm(timePath, prop)
	contexts := []histContext{{
		Name:         "time",
		Parent:       "",
		Path:         timePath,
		ParentFilter: nil,
		Filter:       filter,
	}}
	existsQuery := esdsl.NewNestedQuery(filter).Path(timePath)
	return histogramGet(
		ctx, getSearchService, query, contexts,
		f.Gte, f.Lte,
		timeStepDown,
		topHistogramCounts(prop, "time", existsQuery), histogramCountsOrder,
	)
}

// GetSubTime retrieves time filter data for a sub facet (parentProp > prop): the histogram merged
// across the parent collections the context allows, the identity counts, and the range metadata.
// parentCtx scopes every aggregation to qualifying parent claims.
func (f *TimeFilter) GetSubTime(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, parentCtx *ParentContext,
) ([]HistogramResult, map[string]any, errors.E) {
	var contexts []histContext
	for _, parent := range parentCtx.Collections() {
		pf, ok := parentCtx.CollectionFilter(parent)
		if !ok {
			continue
		}
		path := subPath(parent, "time")
		contexts = append(contexts, histContext{
			Name:         "time:" + parent,
			Parent:       parent,
			Path:         path,
			ParentFilter: pf,
			Filter:       propTerm(path, prop),
		})
	}
	existsQuery := subValueExistsQuery(parentCtx, "time", prop, nil)
	return histogramGet(
		ctx, getSearchService, query, contexts,
		f.Gte, f.Lte,
		timeStepDown,
		subHistogramCounts(parentCtx, prop, "time", existsQuery), histogramCountsOrder,
	)
}
