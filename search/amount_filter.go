package search

import (
	"context"
	"math"
	"strconv"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// amountUnitFilter returns a query that matches the unit field.
// If unit is provided, it matches the exact value. If nil, it matches documents where unit does not exist.
func amountUnitFilter(unit *identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if unit != nil {
		return esdsl.NewTermQuery("claims.amount.unit", esdsl.NewFieldValue().String(unit.String()))
	}
	return esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field("claims.amount.unit"))
}

// computeInterval computes the histogram interval.
//
// It ensures that exactly histogramBins buckets are produced by slightly widening
// the interval so that the max value falls inside the last bucket, not in a 101st bucket.
func computeInterval(from, to float64) (float64, float64, string) {
	// Bins are intervals [from, to). So for upperBound we want the next value after "to".
	upperBound := math.Nextafter(to, math.Inf(1))
	interval := (upperBound - from) / float64(histogramBins)
	interval2 := (to - from) / float64(histogramBins)
	if interval == interval2 {
		// The difference between upperBound and "to" was too small so the interval does not represent it.
		// Let's increase the interval to the next value to make sure "to" falls inside the last bin
		// and is not moved into its own bin.
		interval = math.Nextafter(interval, math.Inf(1))
	}
	// Extended bounds include both endpoints, interval [min, max], so we return "to" as the upper bound
	// (to not include the upperBound which we used to compute the interval).
	return interval, to, strconv.FormatFloat(interval, 'f', -1, 64)
}

// Get retrieves amount filter data for search results.
func (f *AmountFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filter := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
		amountUnitFilter(f.Unit),
	)
	missingNestedQuery := esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String()))
	return histogramFilterGet(
		ctx, getSearchService, query,
		missingNestedQuery, "claims.amount", filter,
		"claims.amount.from", "claims.amount.to", "claims.amount.range",
		f.Gte, f.Lte,
		amountStepDown,
	)
}

// subAmountUnitFilter returns a query that matches the unit field of a
// sub-amount entry.
func subAmountUnitFilter(unit *identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if unit != nil {
		return esdsl.NewTermQuery("claims.subAmount.unit", esdsl.NewFieldValue().String(unit.String()))
	}
	return esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field("claims.subAmount.unit"))
}

// GetSubAmount retrieves sub-amount filter data for search results. It
// aggregates claims.subAmount values for a given (parentProp, prop)
// combination, optionally restricted to listed parentTo values for
// cross-filtering with a sibling parent ref filter.
func (f *AmountFilter) GetSubAmount(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, parentProp, prop identifier.Identifier,
	parentToRestrictions []identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filterMusts := []types.QueryVariant{
		esdsl.NewTermQuery("claims.subAmount.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subAmount.prop", esdsl.NewFieldValue().String(prop.String())),
		subAmountUnitFilter(f.Unit),
	}
	if len(parentToRestrictions) > 0 {
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subAmount.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		filterMusts = append(filterMusts, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}
	filter := esdsl.NewBoolQuery().Must(filterMusts...)
	return histogramSubFilterGet(
		ctx, getSearchService, query,
		parentProp, prop, "claims.subAmount", filter,
		"claims.subAmount.from", "claims.subAmount.to", "claims.subAmount.range",
		f.Gte, f.Lte,
		amountStepDown,
	)
}
