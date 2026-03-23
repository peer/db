// Package search provides search functionality including filters and result handling.
package search

import (
	"context"
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

//nolint:tagliatelle
type minMaxAggregations[T float64 | int64] struct {
	Filter struct {
		Min struct {
			Value T `json:"value"`
		} `json:"min"`
		Max struct {
			Value T `json:"value"`
		} `json:"max"`
		Docs struct {
			Count int64 `json:"doc_count"`
		} `json:"docs"`
	} `json:"filter"`
}

//nolint:tagliatelle
type histogramAggregations struct {
	Filter struct {
		Hist struct {
			Buckets []struct {
				Key  string `json:"key"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
			} `json:"buckets"`
		} `json:"hist"`
	} `json:"filter"`
}

// histogramRange represents the from and to bounds of a histogram bucket.
type histogramRange[T float64 | int64] struct {
	From T
	To   T
}

// HistogramResult represents count for a single bucket in a filter histogram.
type HistogramResult[T float64 | int64] struct {
	From  T     `json:"from"`
	Count int64 `json:"count"`
}

// histogramFilterGet is a generic helper that retrieves histogram filter data for search results.
// It runs a min/max aggregation followed by a range histogram aggregation on the specified nested path.
func histogramFilterGet[T float64 | int64](
	ctx context.Context,
	getSearchService func() (*elastic.SearchService, int64, int64),
	id identifier.Identifier,
	nestedPath string,
	filter elastic.Query,
	fromField, toField, rangeField string,
	formatValue func(T) string,
	computeRanges func(from, to T) (ranges map[string]histogramRange[T], intervalString string),
) ([]HistogramResult[T], map[string]interface{}, errors.E) {
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internalStore.MetricSearchSession).Start()
	searchSession, errE := GetSession(ctx, id)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	query := searchSession.ToQuery()

	// First query: get min/max values and document count.
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
	var minMax minMaxAggregations[T]
	errE = x.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	if minMax.Filter.Docs.Count == 0 {
		return []HistogramResult[T]{}, map[string]interface{}{
			"total": 0,
		}, nil
	}

	minValue := minMax.Filter.Min.Value
	maxValue := minMax.Filter.Max.Value

	// All values are the same, return a single bucket.
	if minValue == maxValue {
		valString := formatValue(minValue)
		return []HistogramResult[T]{{From: minValue, Count: minMax.Filter.Docs.Count}}, map[string]interface{}{
			"total": "1",
			"min":   valString,
			"max":   valString,
		}, nil
	}

	// Compute range buckets and build range aggregation on the range field.
	ranges, intervalString := computeRanges(minValue, maxValue)
	rangeAgg := elastic.NewRangeAggregation().Field(rangeField)
	for key, r := range ranges {
		rangeAgg = rangeAgg.AddRangeWithKey(key, r.From, r.To)
	}
	rangeAgg = rangeAgg.SubAggregation("docs", elastic.NewReverseNestedAggregation())

	// Second query: histogram.
	histogramSearchService, _, _ := getSearchService()
	histogramAggregation := elastic.NewNestedAggregation().Path(nestedPath).SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(filter).SubAggregation("hist", rangeAgg),
	)
	histogramSearchService = histogramSearchService.Size(0).Query(query).Aggregation("histogram", histogramAggregation)

	m = metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err = histogramSearchService.Do(ctx)
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

	results := make([]HistogramResult[T], len(histogram.Filter.Hist.Buckets))
	for i, bucket := range histogram.Filter.Hist.Buckets {
		results[i] = HistogramResult[T]{
			From:  ranges[bucket.Key].From,
			Count: bucket.Docs.Count,
		}
	}

	total := strconv.Itoa(len(results))

	metadata := map[string]interface{}{
		"total":    total,
		"min":      formatValue(minMax.Filter.Min.Value),
		"max":      formatValue(minMax.Filter.Max.Value),
		"interval": intervalString,
	}

	return results, metadata, nil
}
