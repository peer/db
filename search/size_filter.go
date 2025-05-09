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

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

type floatValueAggregation struct {
	Value *float64 `json:"value"`
}

//nolint:tagliatelle
type histogramSizeAggregations struct {
	Buckets []struct {
		Key   float64 `json:"key"`
		Count int64   `json:"doc_count"`
	} `json:"buckets"`
}

func SizeFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id identifier.Identifier,
) (interface{}, map[string]interface{}, errors.E) {
	metrics := waf.MustGetMetrics(ctx)

	m := metrics.Duration(internal.MetricSearchState).Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		return nil, nil, errors.WithStack(ErrNotFound)
	}
	sh := ss.(*State) //nolint:errcheck,forcetypeassert

	query := sh.Query()

	minMaxSearchService, _ := getSearchService()
	minAggregation := elastic.NewMinAggregation().Field("_size")
	maxAggregation := elastic.NewMaxAggregation().Field("_size")
	minMaxSearchService = minMaxSearchService.Size(0).Query(query).Aggregation("min", minAggregation).Aggregation("max", maxAggregation)

	m = metrics.Duration(internal.MetricElasticSearch1).Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal1).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal1).Start()
	var minSize floatValueAggregation
	errE := x.Unmarshal(res.Aggregations["min"], &minSize)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var maxSize floatValueAggregation
	errE = x.Unmarshal(res.Aggregations["max"], &maxSize)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	m.Stop()

	var minValue, interval float64
	if res.Hits.TotalHits.Value == 0 || minSize.Value == nil || maxSize.Value == nil {
		return make([]histogramAmountResult, 0), map[string]interface{}{
			"total": 0,
		}, nil
	} else if *minSize.Value == *maxSize.Value {
		minValue = *minSize.Value
		interval = math.Nextafter(*minSize.Value, *minSize.Value+1)
	} else if *maxSize.Value-*minSize.Value < histogramBins {
		// A special case when there is less than histogramBins of discrete values. In this case we do
		// not want to sample empty bins between values (but prefer to draw wider lines in a histogram).
		minValue = *minSize.Value
		interval = 1
	} else {
		minValue = *minSize.Value
		maxValue := math.Nextafter(*maxSize.Value, *maxSize.Value+1)
		interval = (maxValue - minValue) / histogramBins
		interval2 := (*maxSize.Value - *minSize.Value) / float64(histogramBins)
		if interval == interval2 {
			interval = math.Nextafter(interval2, interval2+1)
		}
	}

	histogramSearchService, _ := getSearchService()
	histogramAggregation := elastic.NewHistogramAggregation().Field("_size").Offset(minValue).Interval(interval)
	histogramSearchService = histogramSearchService.Size(0).Query(query).Aggregation("histogram", histogramAggregation)

	m = metrics.Duration(internal.MetricElasticSearch2).Start()
	res, err = histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal2).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal2).Start()
	var histogram histogramSizeAggregations
	errE = x.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]histogramAmountResult, len(histogram.Buckets))
	for i, bucket := range histogram.Buckets {
		results[i] = histogramAmountResult{
			Min:   bucket.Key,
			Count: bucket.Count,
		}
	}

	total := strconv.Itoa(len(results))
	intervalString := strconv.FormatFloat(interval, 'f', -1, 64)
	minString := strconv.FormatInt(int64(*minSize.Value), 10)
	maxString := strconv.FormatInt(int64(*maxSize.Value), 10)

	metadata := map[string]interface{}{
		"total": total,
		"min":   minString,
		"max":   maxString,
	}

	if *minSize.Value != *maxSize.Value {
		metadata["interval"] = intervalString
	}

	return results, metadata, nil
}
