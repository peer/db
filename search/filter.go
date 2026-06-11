// Package search provides search functionality including filters and result handling.
package search

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	histogramBins = 100

	// missingKey is the aggregation/metadata key used for the count of documents missing the filtered property.
	missingKey = "missing"
)

// HistogramResult represents count for a single bucket in a filter histogram.
type HistogramResult struct {
	From  float64 `json:"from"`
	Count int64   `json:"count"`
}

// parseMinMax extracts doc count and combined min/max endpoint values from a nested->filter aggregation result.
// Min is the smallest and max the largest known endpoint value. Claims with both endpoints always have from
// smaller than to (endpoints are precision window edges), so min(from) and max(to) over all claims cover them.
// Open (none) start claims index only their to field and open end claims index only their from field, so they
// have dedicated aggregations (openStart and openEnd) which alone can extend the combined range beyond min(from)
// and max(to). Min and max are nil when no matching claim has any known endpoint. minIsToEnd reports whether
// the min is determined by an open start claim's to value (also on a tie with a from value): known to endpoints
// are indexed as exclusive range upper bounds and an open start claim has no other endpoint to overlap with,
// so such a claim does not overlap a histogram bucket starting at the min and the histogram start has to be
// lowered to catch it.
func parseMinMax(aggs map[string]types.Aggregate, key string) (int64, *float64, *float64, bool, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	docs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](filter.Aggregations, "docs")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	minFromAgg, errE := internalSearch.AggAs[types.MinAggregate](filter.Aggregations, "minFrom")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	maxToAgg, errE := internalSearch.AggAs[types.MaxAggregate](filter.Aggregations, "maxTo")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	openStart, errE := internalSearch.AggAs[types.FilterAggregate](filter.Aggregations, "openStart")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	minToAgg, errE := internalSearch.AggAs[types.MinAggregate](openStart.Aggregations, "minTo")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	openEnd, errE := internalSearch.AggAs[types.FilterAggregate](filter.Aggregations, "openEnd")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	maxFromAgg, errE := internalSearch.AggAs[types.MaxAggregate](openEnd.Aggregations, "maxFrom")
	if errE != nil {
		return 0, nil, nil, false, errE
	}
	var minVal, maxVal *float64
	var minIsToEnd bool
	if minFromAgg.Value != nil {
		v := float64(*minFromAgg.Value)
		minVal = &v
	}
	if minToAgg.Value != nil && (minVal == nil || float64(*minToAgg.Value) <= *minVal) {
		v := float64(*minToAgg.Value)
		minVal = &v
		minIsToEnd = true
	}
	if maxToAgg.Value != nil {
		v := float64(*maxToAgg.Value)
		maxVal = &v
	}
	if maxFromAgg.Value != nil && (maxVal == nil || float64(*maxFromAgg.Value) > *maxVal) {
		v := float64(*maxFromAgg.Value)
		maxVal = &v
	}
	return docs.DocCount, minVal, maxVal, minIsToEnd, nil
}

// parseHistogramBuckets extracts histogram bucket results from a nested->filter->hist aggregation.
func parseHistogramBuckets(aggs map[string]types.Aggregate, key string) ([]HistogramResult, errors.E) {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return nil, errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	histAgg, errE := internalSearch.AggAs[types.HistogramAggregate](filter.Aggregations, "hist")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := histAgg.Buckets.([]types.HistogramBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for histogram")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", histAgg.Buckets)
		return nil, errE
	}
	results := make([]HistogramResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		results = append(results, HistogramResult{
			From:  float64(bucket.Key),
			Count: bucketDocs.DocCount,
		})
	}
	return results, nil
}

// histogramSubFilterGet retrieves histogram filter data for sub-claim
// (amount or time) filters. It mirrors histogramFilterGet but its missing
// query identifies entries by parentProp and prop on the given nestedPath
// (the sub-claim ES nested path, e.g., "claims.subAmount" or "claims.subTime").
func histogramSubFilterGet(
	ctx context.Context,
	getSearchService func() *esSearch.Search,
	query types.QueryVariant,
	parentProp, prop identifier.Identifier,
	nestedPath string,
	filter types.QueryVariant,
	fromField, toField, rangeField string,
	sessionFrom, sessionTo *float64,
	stepDown func(v, span float64) float64,
) ([]HistogramResult, map[string]any, errors.E) {
	missingNestedQuery := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery(nestedPath+".parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery(nestedPath+".prop", esdsl.NewFieldValue().String(prop.String())),
	)
	return histogramFilterGet(
		ctx, getSearchService, query,
		missingNestedQuery, nestedPath, filter,
		fromField, toField, rangeField,
		sessionFrom, sessionTo,
		stepDown,
	)
}

// histogramFilterGet retrieves histogram filter data for search results.
// It runs a min/max aggregation followed by a histogram aggregation on the
// specified nested path. The parent filter is excluded from the session query
// so that the histogram shows values available under the other filters, not
// restricted by the current filter's own values. If sessionFrom and sessionTo
// are non-nil, those bounds are used for the histogram range instead of (or
// to override) the min/max from the data. This provides "hard bounds"
// (session range narrower than data) and "extended bounds" (session range
// wider than data). When the data has a single known endpoint value, a single
// bucket is returned even when session bounds are set, so that selecting the
// single value round-trips to the same response.
//
// missingNestedQuery is the nested-path inner query that identifies an entry
// matching the filter's identity (e.g., prop term for top-level filters, or
// parentProp+prop combination for sub-claim filters). Documents are counted
// as missing when no nested entry under nestedPath satisfies this query.
//
// stepDown lowers the histogram start when the min known endpoint value is determined by a
// to value: to values are indexed as exclusive range upper bounds, so a claim ending exactly
// at the min would not overlap a first bucket starting there. It is given the value and the
// histogram span and returns a value below it, by one step of the value's apparent precision
// (time or amount specific).
func histogramFilterGet(
	ctx context.Context,
	getSearchService func() *esSearch.Search,
	query types.QueryVariant,
	missingNestedQuery types.QueryVariant,
	nestedPath string,
	filter types.QueryVariant,
	fromField, toField, rangeField string,
	sessionFrom, sessionTo *float64,
	stepDown func(v, span float64) float64,
) ([]HistogramResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	// Aggregation for documents missing the property.
	missingAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(missingNestedQuery).Path(nestedPath),
		))

	// Run min/max aggregation to determine data range and doc count. Claims with an open
	// (none) end index only the endpoint they have, so aggregating just min over from and
	// max over to could miss known endpoints or return no value at all. Their known
	// endpoints are aggregated separately (openStart and openEnd), both to extend the
	// combined range and because an open start claim determining the min requires
	// lowering the histogram start (its to is an exclusive range upper bound).
	minMaxSearchService := getSearchService()
	minMaxAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(filter).
			AddAggregation("minFrom", esdsl.NewAggregations().
				Min(esdsl.NewMinAggregation().Field(fromField))).
			AddAggregation("maxTo", esdsl.NewAggregations().
				Max(esdsl.NewMaxAggregation().Field(toField))).
			AddAggregation("openStart", esdsl.NewAggregations().
				Filter(esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field(fromField))).
				AddAggregation("minTo", esdsl.NewAggregations().
					Min(esdsl.NewMinAggregation().Field(toField)))).
			AddAggregation("openEnd", esdsl.NewAggregations().
				Filter(esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field(toField))).
				AddAggregation("maxFrom", esdsl.NewAggregations().
					Max(esdsl.NewMaxAggregation().Field(fromField)))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation())))
	minMaxSearchService = minMaxSearchService.Size(0).Query(query).
		AddAggregation("minMax", minMaxAggregation).
		AddAggregation(missingKey, missingAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch1).Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.Took) * time.Millisecond

	docCount, minValue, maxValue, minIsToEnd, errE := parseMinMax(res.Aggregations, "minMax")
	if errE != nil {
		return nil, nil, errE
	}
	missingFilter, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, missingKey)
	if errE != nil {
		return nil, nil, errE
	}
	missingCount := missingFilter.DocCount

	if docCount == 0 {
		return []HistogramResult{}, map[string]any{
			"total":    "0",
			missingKey: missingCount,
		}, nil
	}

	// No known endpoint values, so there is nothing to span a histogram with. This happens when all
	// matching claims have both endpoints open (none): such claims index only sentinel range bounds.
	// Claims with unknown endpoints never appear here because the converter collapses an interval
	// with one unknown endpoint to a point claim and converts a fully unknown interval to an
	// unknown claim, which is not indexed under the nested path at all.
	if minValue == nil || maxValue == nil {
		return []HistogramResult{}, map[string]any{
			"total":    "0",
			missingKey: missingCount,
		}, nil
	}

	// The data has a single known endpoint value, return a single bucket, even when session
	// bounds are set, so that selecting the single value round-trips to the same response.
	// The from and to metadata bounds can be used by the client to filter to the single
	// value. When the value is a known to endpoint of an open start claim, the from bound is
	// lowered by one precision step like for the histogram, because such a claim does not
	// contain the value itself (to endpoints are indexed as exclusive range upper bounds).
	// There is no histogram span to refine the step against, so the step is unrefined.
	if *minValue == *maxValue {
		fromValue := *minValue
		if minIsToEnd {
			fromValue = stepDown(fromValue, math.Inf(1))
		}
		return []HistogramResult{{From: *minValue, Count: docCount}}, map[string]any{
			"total":    "1",
			"from":     strconv.FormatFloat(fromValue, 'f', -1, 64),
			"to":       strconv.FormatFloat(*maxValue, 'f', -1, 64),
			missingKey: missingCount,
		}, nil
	}

	var histogramFrom, histogramTo float64
	if sessionFrom != nil && sessionTo != nil {
		// Use session bounds directly.
		histogramFrom = *sessionFrom
		histogramTo = *sessionTo
		// Equal session bounds cannot span a histogram, return a single bucket at the value.
		if histogramFrom == histogramTo {
			valString := strconv.FormatFloat(histogramFrom, 'f', -1, 64)
			return []HistogramResult{{From: histogramFrom, Count: docCount}}, map[string]any{
				"total":    "1",
				"from":     valString,
				"to":       valString,
				missingKey: missingCount,
			}, nil
		}
	} else {
		histogramFrom = *minValue
		histogramTo = *maxValue
		if minIsToEnd {
			// The min is a known to endpoint and those are indexed as exclusive range upper bounds,
			// so a claim ending exactly at the min would not overlap a first bucket starting there.
			// Lower the histogram start so that such claims are counted.
			histogramFrom = stepDown(histogramFrom, histogramTo-histogramFrom)
		}
	}

	// Compute interval and upper bound for the histogram. The upper bound may be
	// adjusted (e.g., rounded up for integer intervals) so the range is evenly divisible.
	interval, upperBound, intervalString := computeInterval(histogramFrom, histogramTo)

	// Compute offset so that bucket boundaries align with histogramFrom.
	offset := math.Mod(histogramFrom, interval)
	if offset < 0 {
		offset += interval
	}

	histAgg := esdsl.NewAggregations().
		Histogram(esdsl.NewHistogramAggregation().
			Field(rangeField).
			Interval(types.Float64(interval)).
			Offset(types.Float64(offset)).
			ExtendedBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(histogramFrom)).Max(types.Float64(upperBound))).
			HardBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(histogramFrom)).Max(types.Float64(upperBound)))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation()))

	// Second query: histogram.
	histogramSearchService := getSearchService()
	histogramAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(filter).
			AddAggregation("hist", histAgg))
	histogramSearchService = histogramSearchService.Size(0).Query(query).AddAggregation("histogram", histogramAggregation)

	m = metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err = histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal2).Duration = time.Duration(res.Took) * time.Millisecond

	results, errE := parseHistogramBuckets(res.Aggregations, "histogram")
	if errE != nil {
		return nil, nil, errE
	}

	total := strconv.Itoa(len(results))

	metadata := map[string]any{
		"total":    total,
		"from":     strconv.FormatFloat(histogramFrom, 'f', -1, 64),
		"to":       strconv.FormatFloat(histogramTo, 'f', -1, 64),
		"interval": intervalString,
		missingKey: missingCount,
	}

	return results, metadata, nil
}
