package search

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

const (
	histogramBins = 100
)

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

func (s *Service) DocumentSearchAmountFilterAPIGet(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["s"]
	if !identifier.Valid(id) {
		s.badRequestWithError(w, req, errors.New(`"s" parameter is not a valid identifier`))
		return
	}

	prop := params["prop"]
	if !identifier.Valid(prop) {
		s.badRequestWithError(w, req, errors.New(`"prop" parameter is not a valid identifier`))
		return
	}

	unit := params["unit"]
	if !ValidAmountUnit(unit) {
		s.badRequestWithError(w, req, errors.New(`"unit" parameter is not a valid unit`))
		return
	}
	if unit == "@" {
		s.badRequestWithError(w, req, errors.New(`"unit" parameter cannot be "@"`))
		return
	}

	m := timing.NewMetric("s").Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		s.NotFound(w, req, nil)
		return
	}
	sh := ss.(*search) //nolint:errcheck

	query := s.getSearchQuery(sh)
	minMaxSearchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}

	minMaxAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop._id", prop),
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

	m = timing.NewMetric("es1").Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi1").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d1").Start()
	var minMax minMaxAmountAggregations
	err = json.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	var min, interval float64
	if minMax.Filter.Count == 0 {
		s.writeJSON(w, req, contentEncoding, make([]histogramAmountResult, 0), http.Header{
			"Total": {"0"},
		})
		return
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

	histogramSearchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}
	histogramAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop._id", prop),
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

	m = timing.NewMetric("es2").Start()
	res, err = histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi2").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d2").Start()
	var histogram histogramAmountAggregations
	err = json.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
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

	metadata := http.Header{
		"Total": {total},
		"Min":   {minString},
		"Max":   {maxString},
	}

	if minMax.Filter.Min.Value != minMax.Filter.Max.Value {
		metadata["Interval"] = []string{intervalString}
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
