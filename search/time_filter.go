package search

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// findTimeBounds walks the Filters tree looking for a TimeFilter matching the given prop.
// It returns the Gte and Lte bounds if found.
func findTimeBounds(filters *Filters, prop identifier.Identifier) (*float64, *float64) {
	if filters == nil {
		return nil, nil
	}

	if filters.Time != nil && !filters.Time.None && filters.Time.Prop == prop {
		return filters.Time.Gte, filters.Time.Lte
	}

	// TODO: This is not really correct. We should do intersection of bounds here.
	for i := range filters.And {
		f, t := findTimeBounds(&filters.And[i], prop)
		if f != nil || t != nil {
			return f, t
		}
	}
	// TODO: This is not really correct. We should do union of bounds here.
	for i := range filters.Or {
		f, t := findTimeBounds(&filters.Or[i], prop)
		if f != nil || t != nil {
			return f, t
		}
	}
	// TODO: This is not really correct. We should do negation of bounds here.
	if filters.Not != nil {
		f, t := findTimeBounds(filters.Not, prop)
		if f != nil || t != nil {
			return f, t
		}
	}

	return nil, nil
}

// TimeFilterGet retrieves time filter data for search results.
func TimeFilterGet(
	ctx context.Context, getSearchService func() (*search.Search, int64, int64), id, prop identifier.Identifier,
) ([]HistogramResult, map[string]interface{}, errors.E) {
	filter := esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String()))
	return histogramFilterGet(
		ctx, getSearchService, id,
		"claims.time", filter,
		"claims.time.from", "claims.time.to", "claims.time.range",
		func(session *Session) (*float64, *float64) {
			return findTimeBounds(session.Filters, prop)
		},
	)
}
