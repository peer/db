package search

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
)

type floatValueAggregation struct {
	Value *float64 `json:"value"`
}

type histogramSizeAggregations struct {
	Buckets []struct {
		Key   float64 `json:"key"`
		Count int64   `json:"doc_count"`
	} `json:"buckets"`
}

func (s *Service) DocumentSearchSizeFilterGet(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id, errE := identifier.FromString(params["s"])
	if errE != nil {
		s.BadRequestWithError(w, req, errors.WithMessage(errE, `"s" parameter is not a valid identifier`))
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

	minAggregation := elastic.NewMinAggregation().Field("_size")
	maxAggregation := elastic.NewMaxAggregation().Field("_size")
	minMaxSearchService = minMaxSearchService.Size(0).Query(query).Aggregation("min", minAggregation).Aggregation("max", maxAggregation)

	m = timing.NewMetric("es1").Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi1").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d1").Start()
	var minSize floatValueAggregation
	err = json.Unmarshal(res.Aggregations["min"], &minSize)
	if err != nil {
		m.Stop()
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var maxSize floatValueAggregation
	err = json.Unmarshal(res.Aggregations["max"], &maxSize)
	if err != nil {
		m.Stop()
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	m.Stop()

	var min, interval float64
	if res.Hits.TotalHits.Value == 0 || minSize.Value == nil || maxSize.Value == nil {
		s.WriteJSON(w, req, make([]histogramAmountResult, 0), map[string]interface{}{
			"total": 0,
		})
		return
	} else if *minSize.Value == *maxSize.Value {
		min = *minSize.Value
		interval = math.Nextafter(*minSize.Value, *minSize.Value+1)
	} else if *maxSize.Value-*minSize.Value < histogramBins {
		// A special case when there is less than histogramBins of discrete values. In this case we do
		// not want to sample empty bins between values (but prefer to draw wider lines in a histogram).
		min = *minSize.Value
		interval = 1
	} else {
		min = *minSize.Value
		max := math.Nextafter(*maxSize.Value, *maxSize.Value+1)
		interval = (max - min) / histogramBins
		interval2 := (*maxSize.Value - *minSize.Value) / float64(histogramBins)
		if interval == interval2 {
			interval = math.Nextafter(interval2, interval2+1)
		}
	}

	histogramSearchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.NotFoundWithError(w, req, errE)
		return
	}
	histogramAggregation := elastic.NewHistogramAggregation().Field("_size").Offset(min).Interval(interval)
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
	var histogram histogramSizeAggregations
	err = json.Unmarshal(res.Aggregations["histogram"], &histogram)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
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

	s.WriteJSON(w, req, results, metadata)
}
