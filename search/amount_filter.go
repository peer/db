package search

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	histogramBins = 100
)

//nolint:tagliatelle
type minMaxAmountAggregations struct {
	Filter struct {
		Count int64 `json:"doc_count"`
		Min   struct {
			Value float64 `json:"value"`
		} `json:"min"`
		Max struct {
			Value float64 `json:"value"`
		} `json:"max"`
		Discrete struct {
			Value float64 `json:"value"`
		} `json:"discrete"`
	} `json:"filter"`
}

//nolint:tagliatelle
type histogramAmountAggregations struct {
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

type histogramAmountResult struct {
	Min   float64 `json:"min"`
	Count int64   `json:"count"`
}

func AmountFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64), id, prop identifier.Identifier, unit string,
) (interface{}, map[string]interface{}, errors.E) {
	metrics := waf.MustGetMetrics(ctx)

	if !document.ValidAmountUnit(unit) {
		return nil, nil, errors.Errorf(`%w: "unit" is not a valid unit`, ErrInvalidArgument)
	}
	if unit == "@" {
		return nil, nil, errors.Errorf(`%w: "unit" cannot be "@"`, ErrInvalidArgument)
	}

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
	minMaxAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop.id", prop),
			).Must(
				elastic.NewTermQuery("claims.amount.unit", unit),
			),
		).SubAggregation(
			"min",
			elastic.NewMinAggregation().Field("claims.amount.amount"),
		).SubAggregation(
			"max",
			elastic.NewMaxAggregation().Field("claims.amount.amount"),
		).SubAggregation(
			"discrete",
			// We want to know if all values are discrete (integers). They are if the sum is zero.
			elastic.NewSumAggregation().Script(
				// TODO: Use a runtime field.
				//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
				elastic.NewScript("return Math.abs(doc['claims.amount.amount'].value - Math.floor(doc['claims.amount.amount'].value))"),
			),
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
	var minMax minMaxAmountAggregations
	err = json.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	var min, interval float64
	if minMax.Filter.Count == 0 {
		return make([]histogramAmountResult, 0), map[string]interface{}{
			"total": 0,
		}, nil
	} else if minMax.Filter.Min.Value == minMax.Filter.Max.Value {
		min = minMax.Filter.Min.Value
		interval = math.Nextafter(minMax.Filter.Min.Value, minMax.Filter.Min.Value+1)
	} else if minMax.Filter.Discrete.Value == 0 && minMax.Filter.Max.Value-minMax.Filter.Min.Value < histogramBins {
		// A special case when there is less than histogramBins of discrete values. In this case we do
		// not want to sample empty bins between values (but prefer to draw wider lines in a histogram).
		min = minMax.Filter.Min.Value
		interval = 1
	} else {
		min = minMax.Filter.Min.Value
		max := math.Nextafter(minMax.Filter.Max.Value, minMax.Filter.Max.Value+1)
		interval = (max - min) / histogramBins
		interval2 := (minMax.Filter.Max.Value - minMax.Filter.Min.Value) / float64(histogramBins)
		if interval == interval2 {
			interval = math.Nextafter(interval2, interval2+1)
		}
	}

	histogramSearchService, _ := getSearchService()
	histogramAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop.id", prop),
			).Must(
				elastic.NewTermQuery("claims.amount.unit", unit),
			),
		).SubAggregation(
			"hist",
			elastic.NewHistogramAggregation().Field("claims.amount.amount").Offset(min).Interval(interval).SubAggregation(
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
	var histogram histogramAmountAggregations
	err = json.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	results := make([]histogramAmountResult, len(histogram.Filter.Hist.Buckets))
	for i, bucket := range histogram.Filter.Hist.Buckets {
		results[i] = histogramAmountResult{
			Min:   bucket.Key,
			Count: bucket.Docs.Count,
		}
	}

	total := strconv.Itoa(len(results))
	intervalString := strconv.FormatFloat(interval, 'f', -1, 64)
	minString := strconv.FormatFloat(minMax.Filter.Min.Value, 'f', -1, 64)
	maxString := strconv.FormatFloat(minMax.Filter.Max.Value, 'f', -1, 64)

	metadata := map[string]interface{}{
		"total": total,
		"min":   minString,
		"max":   maxString,
	}

	if minMax.Filter.Min.Value != minMax.Filter.Max.Value {
		metadata["interval"] = intervalString
	}

	return results, metadata, nil
}
