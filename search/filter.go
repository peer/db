// Package search provides search functionality including filters and result handling.
package search

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	histogramBins = 100
)

// minMaxAggregations is the response structure for min/max aggregations.
// ES min/max always returns float64 values even for long fields.
//
//nolint:tagliatelle
type minMaxAggregations struct {
	Filter struct {
		Min struct {
			Value float64 `json:"value"`
		} `json:"min"`
		Max struct {
			Value float64 `json:"value"`
		} `json:"max"`
		Docs struct {
			Count int64 `json:"doc_count"`
		} `json:"docs"`
	} `json:"filter"`
}

// histogramAggregations is the response structure for histogram aggregations.
// Histogram bucket keys are always float64 in JSON.
//
//nolint:tagliatelle
type histogramAggregations struct {
	Filter struct {
		Hist struct {
			Buckets []struct {
				Key  float64 `json:"key"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
			} `json:"buckets"`
		} `json:"hist"`
	} `json:"filter"`
}

// HistogramResult represents count for a single bucket in a filter histogram.
type HistogramResult struct {
	From  float64 `json:"from"`
	Count int64   `json:"count"`
}

// histogramFilterGet retrieves histogram filter data for search results.
// It runs a min/max aggregation followed by a histogram aggregation on the specified nested path.
// If extractBounds returns non-nil bounds from the search session's filters, those bounds are used
// for the histogram range instead of (or to override) the min/max from the data. This provides
// "hard bounds" (session range narrower than data) and "extended bounds" (session range wider than data).
func histogramFilterGet(
	ctx context.Context,
	getSearchService func() (*elastic.SearchService, int64, int64),
	id identifier.Identifier,
	nestedPath string,
	filter elastic.Query,
	fromField, toField string,
	formatValue func(float64) string,
	computeInterval func(from, to float64) (float64, float64, string),
	extractBounds func(session *Session) (from, to *float64),
) ([]HistogramResult, map[string]interface{}, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := GetSession(ctx, id)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	query := searchSession.ToQuery()

	// Extract optional bounds from the search session's filters.
	// We use float64 bounds even if we could sometimes use int64 bounds so that we stay
	// compatible with the minMax aggregation which uses only float64.
	sessionFrom, sessionTo := extractBounds(searchSession)

	var docCount int64
	var minValue, maxValue float64

	// If bounds come from the session, we can skip the min/max aggregation (but we still need a doc count).
	if sessionFrom != nil && sessionTo != nil {
		// We still need to know if there are any matching documents.
		// Run a count-only aggregation.
		countSearchService, _, _ := getSearchService()
		countAggregation := elastic.NewNestedAggregation().Path(nestedPath).SubAggregation(
			"filter",
			elastic.NewFilterAggregation().Filter(filter).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		)
		countSearchService = countSearchService.Size(0).Query(query).Aggregation("count", countAggregation)

		m = metrics.Duration(internalStore.MetricElasticSearch1).Start()
		res, err := countSearchService.Do(ctx)
		m.Stop()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.TookInMillis) * time.Millisecond

		var countResult minMaxAggregations
		m = metrics.Duration(internalStore.MetricJSONUnmarshal1).Start()
		errE = x.Unmarshal(res.Aggregations["count"], &countResult)
		m.Stop()
		if errE != nil {
			return nil, nil, errE
		}

		docCount = countResult.Filter.Docs.Count
		// Use session bounds directly.
		minValue = *sessionFrom
		maxValue = *sessionTo
	} else {
		// Run min/max aggregation to determine data range and doc count.
		minMaxSearchService, _, _ := getSearchService()
		minMaxAggregation := elastic.NewNestedAggregation().Path(nestedPath).SubAggregation(
			"filter",
			elastic.NewFilterAggregation().Filter(filter).SubAggregation(
				"min",
				elastic.NewMinAggregation().Field(fromField),
			).SubAggregation(
				"max",
				elastic.NewMaxAggregation().Field(toField),
			).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		)
		minMaxSearchService = minMaxSearchService.Size(0).Query(query).Aggregation("minMax", minMaxAggregation)

		m = metrics.Duration(internalStore.MetricElasticSearch1).Start()
		res, err := minMaxSearchService.Do(ctx)
		m.Stop()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.TookInMillis) * time.Millisecond

		m = metrics.Duration(internalStore.MetricJSONUnmarshal1).Start()
		var minMax minMaxAggregations
		errE = x.Unmarshal(res.Aggregations["minMax"], &minMax)
		m.Stop()
		if errE != nil {
			return nil, nil, errE
		}

		docCount = minMax.Filter.Docs.Count
		minValue = minMax.Filter.Min.Value
		maxValue = minMax.Filter.Max.Value
	}

	if docCount == 0 {
		return []HistogramResult{}, map[string]interface{}{
			"total": 0,
		}, nil
	}

	// Bounds are the same, return a single bucket.
	if minValue == maxValue {
		valString := formatValue(minValue)
		return []HistogramResult{{From: minValue, Count: docCount}}, map[string]interface{}{
			"total": "1",
			"from":  valString,
			"to":    valString,
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

	// TODO: Set "hard bounds".
	histAgg := elastic.NewHistogramAggregation().
		Field(fromField).
		Interval(interval).
		Offset(offset).
		ExtendedBounds(minValue, upperBound).
		SubAggregation("docs", elastic.NewReverseNestedAggregation())

	// Second query: histogram.
	histogramSearchService, _, _ := getSearchService()
	histogramAggregation := elastic.NewNestedAggregation().Path(nestedPath).SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(filter).SubAggregation("hist", histAgg),
	)
	histogramSearchService = histogramSearchService.Size(0).Query(query).Aggregation("histogram", histogramAggregation)

	m = metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err := histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal2).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internalStore.MetricJSONUnmarshal2).Start()
	var histogram histogramAggregations
	errE = x.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]HistogramResult, len(histogram.Filter.Hist.Buckets))
	for i, bucket := range histogram.Filter.Hist.Buckets {
		results[i] = HistogramResult{
			From:  bucket.Key,
			Count: bucket.Docs.Count,
		}
	}

	total := strconv.Itoa(len(results))

	metadata := map[string]interface{}{
		"total":    total,
		"from":     formatValue(minValue),
		"to":       formatValue(maxValue),
		"interval": intervalString,
	}

	return results, metadata, nil
}
