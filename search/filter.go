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

// aggAs extracts a typed aggregation from a map of aggregations.
//
// TODO: Contribute upstream. See: https://github.com/elastic/go-elasticsearch/issues/1367
func aggAs[T any](aggs map[string]types.Aggregate, key string) (*T, errors.E) {
	raw, ok := aggs[key]
	if !ok {
		errE := errors.New("aggregation not found")
		errors.Details(errE)["key"] = key
		return nil, errE
	}
	typed, ok := raw.(*T)
	if !ok {
		errE := errors.New("unexpected aggregation type")
		errors.Details(errE)["key"] = key
		errors.Details(errE)["type"] = fmt.Sprintf("%T", raw)
		return nil, errE
	}
	return typed, nil
}

// parseMinMax extracts doc count, min, and max values from a nested->filter aggregation result.
func parseMinMax(aggs map[string]types.Aggregate, key string) (int64, float64, float64, errors.E) {
	nested, errE := aggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return 0, 0, 0, errE
	}
	filter, errE := aggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return 0, 0, 0, errE
	}
	docs, errE := aggAs[types.ReverseNestedAggregate](filter.Aggregations, "docs")
	if errE != nil {
		return 0, 0, 0, errE
	}
	minAgg, errE := aggAs[types.MinAggregate](filter.Aggregations, "min")
	if errE != nil {
		return 0, 0, 0, errE
	}
	maxAgg, errE := aggAs[types.MaxAggregate](filter.Aggregations, "max")
	if errE != nil {
		return 0, 0, 0, errE
	}
	var minVal, maxVal float64
	if minAgg.Value != nil {
		minVal = float64(*minAgg.Value)
	}
	if maxAgg.Value != nil {
		maxVal = float64(*maxAgg.Value)
	}
	return docs.DocCount, minVal, maxVal, nil
}

// parseCountOnly extracts doc count from a nested->filter->docs aggregation result.
func parseCountOnly(aggs map[string]types.Aggregate, key string) (int64, errors.E) {
	nested, errE := aggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return 0, errE
	}
	filter, errE := aggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return 0, errE
	}
	docs, errE := aggAs[types.ReverseNestedAggregate](filter.Aggregations, "docs")
	if errE != nil {
		return 0, errE
	}
	return docs.DocCount, nil
}

// parseHistogramBuckets extracts histogram bucket results from a nested->filter->hist aggregation.
func parseHistogramBuckets(aggs map[string]types.Aggregate, key string) ([]HistogramResult, errors.E) {
	nested, errE := aggAs[types.NestedAggregate](aggs, key)
	if errE != nil {
		return nil, errE
	}
	filter, errE := aggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	histAgg, errE := aggAs[types.HistogramAggregate](filter.Aggregations, "hist")
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
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
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
// wider than data).
//
// missingNestedQuery is the nested-path inner query that identifies an entry
// matching the filter's identity (e.g., prop term for top-level filters, or
// parentProp+prop combination for sub-claim filters). Documents are counted
// as missing when no nested entry under nestedPath satisfies this query.
func histogramFilterGet(
	ctx context.Context,
	getSearchService func() *esSearch.Search,
	query types.QueryVariant,
	missingNestedQuery types.QueryVariant,
	nestedPath string,
	filter types.QueryVariant,
	fromField, toField, rangeField string,
	sessionFrom, sessionTo *float64,
) ([]HistogramResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	// Aggregation for documents missing the property.
	missingAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(missingNestedQuery).Path(nestedPath),
		))

	var docCount int64
	var missingCount int64
	var minValue, maxValue float64

	// If bounds come from the session, we can skip the min/max aggregation (but we still need a doc count).
	if sessionFrom != nil && sessionTo != nil {
		// We still need to know if there are any matching documents.
		// Run a count-only aggregation.
		countSearchService := getSearchService()
		countAggregation := esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(filter).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())))
		countSearchService = countSearchService.Size(0).Query(query).
			AddAggregation("count", countAggregation).
			AddAggregation(missingKey, missingAggregation)

		m := metrics.Duration(internalStore.MetricElasticSearch1).Start()
		res, err := countSearchService.Do(ctx)
		m.Stop()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.Took) * time.Millisecond

		var errE errors.E
		docCount, errE = parseCountOnly(res.Aggregations, "count")
		if errE != nil {
			return nil, nil, errE
		}
		missingFilter, errE := aggAs[types.FilterAggregate](res.Aggregations, missingKey)
		if errE != nil {
			return nil, nil, errE
		}
		missingCount = missingFilter.DocCount
		// Use session bounds directly.
		minValue = *sessionFrom
		maxValue = *sessionTo
	} else {
		// Run min/max aggregation to determine data range and doc count.
		minMaxSearchService := getSearchService()
		minMaxAggregation := esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(filter).
				AddAggregation("min", esdsl.NewAggregations().
					Min(esdsl.NewMinAggregation().Field(fromField))).
				AddAggregation("max", esdsl.NewAggregations().
					Max(esdsl.NewMaxAggregation().Field(toField))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())))
		minMaxSearchService = minMaxSearchService.Size(0).Query(query).
			AddAggregation("minMax", minMaxAggregation).
			AddAggregation(missingKey, missingAggregation)

		m := metrics.Duration(internalStore.MetricElasticSearch1).Start()
		res, err := minMaxSearchService.Do(ctx)
		m.Stop()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.Took) * time.Millisecond

		var errE errors.E
		docCount, minValue, maxValue, errE = parseMinMax(res.Aggregations, "minMax")
		if errE != nil {
			return nil, nil, errE
		}
		missingFilter, errE := aggAs[types.FilterAggregate](res.Aggregations, missingKey)
		if errE != nil {
			return nil, nil, errE
		}
		missingCount = missingFilter.DocCount
	}

	if docCount == 0 {
		return []HistogramResult{}, map[string]any{
			"total":    0,
			missingKey: missingCount,
		}, nil
	}

	// Bounds are the same, return a single bucket.
	if minValue == maxValue {
		valString := strconv.FormatFloat(minValue, 'f', -1, 64)
		return []HistogramResult{{From: minValue, Count: docCount}}, map[string]any{
			"total":    "1",
			"from":     valString,
			"to":       valString,
			missingKey: missingCount,
		}, nil
	}

	// Compute interval and upper bound for the histogram. The upper bound may be
	// adjusted (e.g., rounded up for integer intervals) so the range is evenly divisible.
	interval, upperBound, intervalString := computeInterval(minValue, maxValue)

	// Compute offset so that bucket boundaries align with minValue.
	offset := math.Mod(minValue, interval)
	if offset < 0 {
		offset += interval
	}

	histAgg := esdsl.NewAggregations().
		Histogram(esdsl.NewHistogramAggregation().
			Field(rangeField).
			Interval(types.Float64(interval)).
			Offset(types.Float64(offset)).
			ExtendedBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(minValue)).Max(types.Float64(upperBound))).
			HardBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(minValue)).Max(types.Float64(upperBound)))).
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

	m := metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err := histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal2).Duration = time.Duration(res.Took) * time.Millisecond

	results, errE := parseHistogramBuckets(res.Aggregations, "histogram")
	if errE != nil {
		return nil, nil, errE
	}

	total := strconv.Itoa(len(results))

	metadata := map[string]any{
		"total":    total,
		"from":     strconv.FormatFloat(minValue, 'f', -1, 64),
		"to":       strconv.FormatFloat(maxValue, 'f', -1, 64),
		"interval": intervalString,
		missingKey: missingCount,
	}

	return results, metadata, nil
}
