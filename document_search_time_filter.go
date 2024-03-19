package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
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

func (s *Service) DocumentSearchTimeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" parameter is not a valid identifier`))
		return
	}

	prop, errE := identifier.FromString(params["prop"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"prop" parameter is not a valid identifier`))
		return
	}

	m := timing.NewMetric("s").Start()
	ss, ok := searches.Load(id)
	m.Stop()
	if !ok {
		// Something was not OK, so we return not found.
		s.NotFound(w, req)
		return
	}
	sh := ss.(*searchState) //nolint:errcheck

	query := s.getSearchQuery(sh)
	minMaxSearchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.NotFoundWithError(w, req, errE)
		return
	}
	minMaxAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.time.prop._id", prop),
		).SubAggregation(
			"min",
			elastic.NewMinAggregation().Field("claims.time.timestamp"),
		).SubAggregation(
			"max",
			elastic.NewMaxAggregation().Field("claims.time.timestamp"),
		),
	)
	minMaxSearchService = minMaxSearchService.Size(0).Query(query).Aggregation("minMax", minMaxAggregation)

	m = timing.NewMetric("es1").Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi1").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d1").Start()
	var minMax minMaxTimeAggregations
	err = json.Unmarshal(res.Aggregations["minMax"], &minMax)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	// We use int64 and not time.Duration because it cannot hold durations we need.
	// time.Duration stores durations as nanosecond, but we want seconds here.
	// See: https://github.com/elastic/elasticsearch/issues/83101
	var min, interval int64
	if minMax.Filter.Count == 0 {
		s.WriteJSON(w, req, make([]histogramTimeResult, 0), map[string]interface{}{
			"total": 0,
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
		s.NotFoundWithError(w, req, errE)
		return
	}
	histogramAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewTermQuery("claims.time.prop._id", prop),
		).SubAggregation(
			"hist",
			elastic.NewDateHistogramAggregation().Field("claims.time.timestamp").Offset(offsetString).FixedInterval(intervalString).SubAggregation(
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
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi2").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d2").Start()
	var histogram histogramTimeAggregations
	err = json.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
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

	metadata := map[string]interface{}{
		"total": total,
		"min":   minMax.Filter.Min.Value.String(),
		"max":   minMax.Filter.Max.Value.String(),
	}

	if minMax.Filter.Min.Value != minMax.Filter.Max.Value {
		metadata["Interval"] = []string{intervalString}
	}

	s.WriteJSON(w, req, results, metadata)
}
