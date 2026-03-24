package search

import (
	"context"
	"math"
	"strconv"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// findTimeBounds walks the Filters tree looking for a TimeFilter matching the given prop.
// It returns the Gte and Lte bounds (converted to float64) if found.
func findTimeBounds(filters *Filters, prop identifier.Identifier) (*float64, *float64) {
	if filters == nil {
		return nil, nil
	}

	if filters.Time != nil && !filters.Time.None && filters.Time.Prop == prop {
		f := float64(*filters.Time.Gte)
		t := float64(*filters.Time.Lte)
		return &f, &t
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

// timeFormatValue formats a float64 time value as an integer string.
func timeFormatValue(v float64) string {
	return strconv.FormatInt(int64(math.Round(v)), 10)
}

// timeComputeInterval computes the histogram interval for time (integer) values.
// It ensures that the max value falls inside the last bucket by widening the range
// by 1 (since values are integers) and using integer-sized intervals.
func timeComputeInterval(from, to float64) (float64, float64, string) {
	// Bins are intervals [from, to). So for upperBound we want the next value after "to".
	// Because "to" is really an integer, we do + 1 here.
	upperBound := to + 1
	// We want integer-sized intervals, so we round up to the next integer.
	interval := math.Ceil((upperBound - from) / float64(histogramBins))
	interval2 := math.Ceil((to - from) / float64(histogramBins))
	if interval == interval2 {
		// The difference between upperBound and "to" was too small so the interval does not represent it.
		// Let's increase the interval to the next value to make sure "to" falls inside the last bin
		// and is not moved into its own bin.
		// We want integer-sized intervals, so we + 1 here.
		interval++
	}
	// Extended bounds include both endpoints, interval [min, max], so we return "to" as the upper bound
	// (to not include the upperBound which we used to compute the interval).
	return interval, to, strconv.FormatInt(int64(math.Round(interval)), 10)
}

// TimeFilterGet retrieves time filter data for search results.
func TimeFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64, int64), id, prop identifier.Identifier,
) ([]HistogramResult, map[string]interface{}, errors.E) {
	filter := elastic.NewTermQuery("claims.time.prop", prop)
	return histogramFilterGet(
		ctx, getSearchService, id,
		"claims.time", filter,
		"claims.time.from", "claims.time.to",
		timeFormatValue,
		timeComputeInterval,
		func(session *Session) (*float64, *float64) {
			return findTimeBounds(session.Filters, prop)
		},
	)
}
