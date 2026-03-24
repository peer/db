package search

import (
	"context"
	"math"
	"math/big"
	"strconv"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// int64ToFloat64Floor converts an int64 to float64, rounding toward negative infinity
// if the value cannot be represented exactly. This is appropriate for lower bounds.
func int64ToFloat64Floor(v int64) float64 {
	f := float64(v)
	// Compare using big.Float to avoid int64 overflow when converting back.
	if new(big.Float).SetFloat64(f).Cmp(new(big.Float).SetInt64(v)) > 0 {
		f = math.Nextafter(f, math.Inf(-1))
	}
	return f
}

// int64ToFloat64Ceil converts an int64 to float64, rounding toward positive infinity
// if the value cannot be represented exactly. This is appropriate for upper bounds.
func int64ToFloat64Ceil(v int64) float64 {
	f := float64(v)
	// Compare using big.Float to avoid int64 overflow when converting back.
	if new(big.Float).SetFloat64(f).Cmp(new(big.Float).SetInt64(v)) < 0 {
		f = math.Nextafter(f, math.Inf(1))
	}
	return f
}

// findTimeBounds walks the Filters tree looking for a TimeFilter matching the given prop.
// It returns the Gte and Lte bounds (converted to float64) if found. The lower bound
// is converted rounding down and the upper bound rounding up to ensure the float64 range
// contains the original int64 range.
func findTimeBounds(filters *Filters, prop identifier.Identifier) (*float64, *float64) {
	if filters == nil {
		return nil, nil
	}

	if filters.Time != nil && !filters.Time.None && filters.Time.Prop == prop {
		f := int64ToFloat64Floor(*filters.Time.Gte)
		t := int64ToFloat64Ceil(*filters.Time.Lte)
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
// It picks the largest integer interval that still produces at least histogramBins buckets.
// For small ranges (< histogramBins integers), interval is 1 so each integer gets its own bin.
func timeComputeInterval(from, to float64) (float64, float64, string) {
	// Largest integer interval such that floor((to-from)/interval) + 1 >= histogramBins.
	// This ensures at least histogramBins buckets when possible.
	interval := math.Max(1, math.Floor((to-from)/float64(histogramBins-1)))
	return interval, to, strconv.FormatInt(int64(math.Round(interval)), 10)
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
		timeFormatValue,
		timeComputeInterval,
		func(session *Session) (*float64, *float64) {
			return findTimeBounds(session.Filters, prop)
		},
	)
}
