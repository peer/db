package search

import (
	"context"
	"math"
	"strconv"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
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

// findAmountBounds walks the Filters tree looking for an AmountFilter matching the given prop and unit.
// It returns the Gte and Lte bounds if found. It searches And, Or, and Not recursively.
func findAmountBounds(filters *Filters, prop identifier.Identifier, unit *identifier.Identifier) (*float64, *float64) {
	if filters == nil {
		return nil, nil
	}

	if filters.Amount != nil && !filters.Amount.None && filters.Amount.Prop == prop && matchUnit(filters.Amount.Unit, unit) {
		return filters.Amount.Gte, filters.Amount.Lte
	}

	// TODO: This is not really correct. We should do intersection of bounds here.
	for i := range filters.And {
		f, t := findAmountBounds(&filters.And[i], prop, unit)
		if f != nil || t != nil {
			return f, t
		}
	}
	// TODO: This is not really correct. We should do union of bounds here.
	for i := range filters.Or {
		f, t := findAmountBounds(&filters.Or[i], prop, unit)
		if f != nil || t != nil {
			return f, t
		}
	}
	// TODO: This is not really correct. We should do negation of bounds here.
	if filters.Not != nil {
		f, t := findAmountBounds(filters.Not, prop, unit)
		if f != nil || t != nil {
			return f, t
		}
	}

	return nil, nil
}

// matchUnit returns true if the two unit pointers represent the same unit (both nil or both equal).
func matchUnit(a, b *identifier.Identifier) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// amountFormatValue formats a float64 amount value as a string.
func amountFormatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// amountComputeInterval computes the histogram interval for amount values.
// It ensures that exactly histogramBins buckets are produced by slightly widening
// the interval so that the max value falls inside the last bucket, not in a 101st bucket.
func amountComputeInterval(from, to float64) (float64, float64, string) {
	// Bins are intervals [from, to). So for upperBound we want the next value after "to".
	upperBound := math.Nextafter(to, to+1)
	interval := (upperBound - from) / float64(histogramBins)
	interval2 := (to - from) / float64(histogramBins)
	if interval == interval2 {
		// The difference between upperBound and "to" was too small so the interval does not represent it.
		// Let's increase the interval to the next value to make sure "to" falls inside the last bin
		// and is not moved into its own bin.
		interval = math.Nextafter(interval, interval+1)
	}
	// Extended bounds include both endpoints, interval [min, max], so we return "to" as the upper bound
	// (to not include the upperBound which we used to compute the interval).
	return interval, to, strconv.FormatFloat(interval, 'f', -1, 64)
}

// AmountFilterGet retrieves amount filter data for search results.
func AmountFilterGet(
	ctx context.Context, getSearchService func() (*search.Search, int64, int64), id, prop identifier.Identifier, unit *identifier.Identifier,
) ([]HistogramResult, map[string]interface{}, errors.E) {
	filter := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
		amountUnitFilter(unit),
	)
	return histogramFilterGet(
		ctx, getSearchService, id,
		"claims.amount", filter,
		"claims.amount.from", "claims.amount.to",
		amountFormatValue,
		amountComputeInterval,
		func(session *Session) (*float64, *float64) {
			return findAmountBounds(session.Filters, prop, unit)
		},
	)
}
