package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/identifier"
)

type minMaxTimeAggregations struct {
	Filter struct {
		Count int64 `json:"doc_count"`
		Min   struct {
			Value Timestamp `json:"value_as_string"`
		} `json:"min"`
		Max struct {
			Value Timestamp `json:"value_as_string"`
		} `json:"max"`
	} `json:"filter"`
}

type histogramTimeAggregations struct {
	Filter struct {
		Hist struct {
			Buckets []struct {
				Key  Timestamp `json:"key_as_string"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
			} `json:"buckets"`
		} `json:"hist"`
	} `json:"filter"`
}

type histogramTimeResult struct {
	Min   Timestamp `json:"min"`
	Count int64     `json:"count"`
}

func (s *Service) DocumentSearchTimeFilterAPIGet(w http.ResponseWriter, req *http.Request, params Params) {
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
	minMaxAggregation := elastic.NewNestedAggregation().Path("active.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("active.time.prop._id", prop),
		).SubAggregation(
			"min",
			elastic.NewMinAggregation().Field("active.time.timestamp"),
		).SubAggregation(
			"max",
			elastic.NewMaxAggregation().Field("active.time.timestamp"),
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
	var minMax minMaxTimeAggregations
	err = json.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	// We use int64 and not time.Duration because it cannot hold durations we need.
	// time.Duration stores durations as nanosecond, but we want seconds here.
	// See: https://github.com/elastic/elasticsearch/issues/83101
	var min, interval int64
	if minMax.Filter.Count == 0 {
		s.writeJSON(w, req, contentEncoding, make([]histogramTimeResult, 0), http.Header{
			"Total": {"0"},
		})
		return
	} else if minMax.Filter.Min.Value == minMax.Filter.Max.Value {
		min = time.Time(minMax.Filter.Min.Value).Unix()
		interval = 1
	} else {
		min = time.Time(minMax.Filter.Min.Value).Unix()
		max := time.Time(minMax.Filter.Max.Value).Unix() + 1
		interval = (max - min) / histogramBins
		interval2 := (time.Time(minMax.Filter.Max.Value).Unix() - min) / histogramBins
		if interval == interval2 {
			interval = interval2 + 1
		}
	}

	offsetString := fmt.Sprintf("%ds", min)
	intervalString := fmt.Sprintf("%ds", interval)
	histogramSearchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}
	histogramAggregation := elastic.NewNestedAggregation().Path("active.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("active.time.prop._id", prop),
		).SubAggregation(
			"hist",
			elastic.NewDateHistogramAggregation().Field("active.time.timestamp").Offset(offsetString).FixedInterval(intervalString).SubAggregation(
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
	var histogram histogramTimeAggregations
	err = json.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	results := make([]histogramTimeResult, len(histogram.Filter.Hist.Buckets))
	for i, bucket := range histogram.Filter.Hist.Buckets {
		results[i] = histogramTimeResult{
			Min:   bucket.Key,
			Count: bucket.Docs.Count,
		}
	}

	total := strconv.Itoa(len(results))

	metadata := http.Header{
		"Total": {total},
		"Min":   {minMax.Filter.Min.Value.String()},
		"Max":   {minMax.Filter.Max.Value.String()},
	}

	if minMax.Filter.Min.Value != minMax.Filter.Max.Value {
		metadata["Interval"] = []string{intervalString}
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
