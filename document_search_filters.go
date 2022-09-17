package search

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	gddo "github.com/golang/gddo/httputil"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/exp/slices"

	"gitlab.com/peerdb/search/identifier"
)

// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?

func (s *Service) populateProperties(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("active.rel.prop._id", "2fjzZyP7rv8E4aHnBc6KAa"),
		elastic.NewTermQuery("active.rel.to._id", "HohteEmv2o7gPRnJ5wukVe"),
	)
	query := elastic.NewNestedQuery("active.rel", boolQuery)

	total, err := s.ESClient.Count(s.Index).Query(query).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	s.propertiesTotal = total

	return nil
}

type relAggregations struct {
	Props struct {
		Buckets []struct {
			Key  string `json:"key"`
			Docs struct {
				Count int64 `json:"doc_count"`
			} `json:"docs"`
		} `json:"buckets"`
	} `json:"props"`
	Total struct {
		Value int64 `json:"value"`
	} `json:"total"`
}

type amountAggregations struct {
	Filter struct {
		Props struct {
			Buckets []struct {
				Key  []string `json:"key"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
				Min struct {
					Value float64 `json:"value"`
				}
				Max struct {
					Value float64 `json:"value"`
				}
			} `json:"buckets"`
		} `json:"props"`
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
	} `json:"filter"`
}

func (s *Service) DocumentSearchFiltersGetJSON(w http.ResponseWriter, req *http.Request, params Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	id := params["s"]
	if !identifier.Valid(id) {
		s.BadRequest(w, req, nil)
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
	relAggregation := elastic.NewNestedAggregation().Path("active.rel").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("active.rel.prop._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most s.propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("active.rel.prop._id").PrecisionThreshold(2*s.propertiesTotal), //nolint:gomnd
	)
	amountAggregation := elastic.NewNestedAggregation().Path("active.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().MustNot(elastic.NewTermQuery("active.amount.unit", "@")),
		).SubAggregation(
			"props",
			elastic.NewMultiTermsAggregation().Terms("active.amount.prop._id", "active.amount.unit").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			).SubAggregation(
				"min",
				elastic.NewMinAggregation().Field("active.amount.amount"),
			).SubAggregation(
				"max",
				elastic.NewMaxAggregation().Field("active.amount.amount"),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. It can be at most s.propertiesTotal*amountUnitsTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			elastic.NewCardinalityAggregation().Script(
				elastic.NewScript("return [doc['active.amount.prop._id'], doc['active.amount.unit']]"),
			).PrecisionThreshold(2*s.propertiesTotal*int64(amountUnitsTotal)), //nolint:gomnd
		),
	)
	searchService := s.getSearchService(req).Size(0).Query(query).Aggregation("rel", relAggregation).Aggregation("amount", amountAggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var rel relAggregations
	err = json.Unmarshal(res.Aggregations["rel"], &rel)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var amount amountAggregations
	err = json.Unmarshal(res.Aggregations["amount"], &amount)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	m.Stop()

	results := make([]searchResult, len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets))
	for i, bucket := range rel.Props.Buckets {
		results[i] = searchResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "rel",
		}
	}
	for i, bucket := range amount.Filter.Props.Buckets {
		min := bucket.Min.Value
		max := bucket.Max.Value
		results[len(rel.Props.Buckets)+i] = searchResult{
			ID:    bucket.Key[0],
			Count: bucket.Docs.Count,
			Type:  "amount",
			Unit:  bucket.Key[1],
			Min:   &min,
			Max:   &max,
		}
	}

	// Because we combine multiple aggregations of maxResultsCount each, we have to
	// re-sort results and limit them ourselves.
	slices.SortStableFunc(results, func(a searchResult, b searchResult) bool {
		return a.Count > b.Count
	})
	if len(results) > maxResultsCount {
		results = results[:maxResultsCount]
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(rel.Props.Buckets))+int64(len(amount.Filter.Props.Buckets)) > rel.Total.Value {
		rel.Total.Value = int64(len(rel.Props.Buckets)) + int64(len(amount.Filter.Props.Buckets))
	}
	total := strconv.FormatInt(rel.Total.Value+amount.Filter.Total.Value, 10) //nolint:gomnd

	metadata := http.Header{
		"Total": {total},
	}

	s.writeJSON(w, req, contentEncoding, results[:maxResultsCount], metadata)
}
