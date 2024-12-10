package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

//nolint:tagliatelle
type minMaxTimeAggregations struct {
	Filter struct {
		Count int64 `json:"doc_count"`
		Min   struct {
			Value document.Timestamp `json:"value_as_string"`
		} `json:"min"`
		Max struct {
			Value document.Timestamp `json:"value_as_string"`
		} `json:"max"`
	} `json:"filter"`
}

//nolint:tagliatelle
type histogramTimeAggregations struct {
	Filter struct {
		Hist struct {
			Buckets []struct {
				Key  document.Timestamp `json:"key_as_string"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
			} `json:"buckets"`
		} `json:"hist"`
	} `json:"filter"`
}

type histogramTimeResult struct {
	Min   document.Timestamp `json:"min"`
	Count int64              `json:"count"`
}

func TimeFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id, prop identifier.Identifier,
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
	minMaxAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.time.prop.id", prop),
		).SubAggregation(
			"min",
			elastic.NewMinAggregation().Field("claims.time.timestamp"),
		).SubAggregation(
			"max",
			elastic.NewMaxAggregation().Field("claims.time.timestamp"),
		),
	)
	minMaxSearchService = minMaxSearchService.Size(0).Query(query).Aggregation("minMax", minMaxAggregation)

	m = metrics.Duration(internal.MetricElasticSearch1).Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal1).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal1).Start()
	var minMax minMaxTimeAggregations
	errE := x.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	// We use int64 and not time.Duration because it cannot hold durations we need.
	// time.Duration stores durations as nanosecond, but we want seconds here.
	// See: https://github.com/elastic/elasticsearch/issues/83101
	var minValue, interval int64
	if minMax.Filter.Count == 0 {
		return make([]histogramTimeResult, 0), map[string]interface{}{
			"total": 0,
		}, nil
	} else if minMax.Filter.Min.Value == minMax.Filter.Max.Value {
		minValue = time.Time(minMax.Filter.Min.Value).Unix()
		interval = 1
	} else {
		minValue = time.Time(minMax.Filter.Min.Value).Unix()
		maxValue := time.Time(minMax.Filter.Max.Value).Unix() + 1
		interval = (maxValue - minValue) / histogramBins
		interval2 := (time.Time(minMax.Filter.Max.Value).Unix() - minValue) / histogramBins
		if interval == interval2 {
			interval = interval2 + 1
		}
	}

	offsetString := fmt.Sprintf("%ds", minValue)
	intervalString := fmt.Sprintf("%ds", interval)
	histogramSearchService, _ := getSearchService()
	histogramAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.time.prop.id", prop),
		).SubAggregation(
			"hist",
			elastic.NewDateHistogramAggregation().Field("claims.time.timestamp").Offset(offsetString).FixedInterval(intervalString).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		),
	)
	histogramSearchService = histogramSearchService.Size(0).Query(query).Aggregation("histogram", histogramAggregation)

	m = metrics.Duration(internal.MetricElasticSearch2).Start()
	res, err = histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal2).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal2).Start()
	var histogram histogramTimeAggregations
	errE = x.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]histogramTimeResult, len(histogram.Filter.Hist.Buckets))
	for i, bucket := range histogram.Filter.Hist.Buckets {
		results[i] = histogramTimeResult{
			Min:   bucket.Key,
			Count: bucket.Docs.Count,
		}
	}

	total := strconv.Itoa(len(results))

	metadata := map[string]interface{}{
		"total": total,
		"min":   minMax.Filter.Min.Value.String(),
		"max":   minMax.Filter.Max.Value.String(),
	}

	if minMax.Filter.Min.Value != minMax.Filter.Max.Value {
		metadata["interval"] = intervalString
	}

	return results, metadata, nil
}
