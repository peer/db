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

	for domain, site := range s.Sites {
		total, err := s.ESClient.Count(site.Index).Query(query).Do(ctx)
		if err != nil {
			return errors.Errorf(`site "%s": %w`, site.Index, err)
		}
		// Map cannot be modified directly, so we modify the copy
		// and store it back into the map.
		site.propertiesTotal = total
		s.Sites[domain] = site
	}

	return nil
}

type termAggregations struct {
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

type filteredMultiTermAggregations struct {
	Filter struct {
		Props struct {
			Buckets []struct {
				Key  []string `json:"key"`
				Docs struct {
					Count int64 `json:"doc_count"`
				} `json:"docs"`
			} `json:"buckets"`
		} `json:"props"`
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
	} `json:"filter"`
}

type intValueAggregation struct {
	Value int64 `json:"value"`
}

type searchFiltersResult struct {
	ID    string `json:"_id,omitempty"`
	Count int64  `json:"_count,omitempty"`
	Type  string `json:"_type,omitempty"`
	Unit  string `json:"_unit,omitempty"`
}

func (s *Service) DocumentSearchFiltersAPIGet(w http.ResponseWriter, req *http.Request, params Params) {
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
	searchService, propertiesTotal, errE := s.getSearchService(req)
	if errE != nil {
		s.notFoundWithError(w, req, errE)
		return
	}
	relAggregation := elastic.NewNestedAggregation().Path("active.rel").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("active.rel.prop._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("active.rel.prop._id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
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
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal*amountUnitsTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			// TODO: Use a runtime field.
			//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
			elastic.NewCardinalityAggregation().Script(
				// We use "|" as separator because this is used by ElasticSearch in "key_as_string" as well.
				elastic.NewScript("return doc['active.amount.prop._id'].value + '|' + doc['active.amount.unit'].value"),
			).PrecisionThreshold(2*propertiesTotal*int64(amountUnitsTotal)),
		),
	)
	timeAggregation := elastic.NewNestedAggregation().Path("active.time").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("active.time.prop._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("active.time.prop._id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
	)
	stringAggregation := elastic.NewNestedAggregation().Path("active.string").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("active.string.prop._id").Size(maxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("active.string.prop._id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
	)
	// Cardinality aggregation returns the count of all buckets. 40000 is the maximum precision threshold,
	// so we use it to get the most accurate approximation.
	indexAggregation := elastic.NewCardinalityAggregation().Field("_index").PrecisionThreshold(40000) //nolint:gomnd
	sizeAggregation := elastic.NewValueCountAggregation().Field("_size")
	searchService = searchService.Size(0).Query(query).
		Aggregation("rel", relAggregation).
		Aggregation("amount", amountAggregation).
		Aggregation("time", timeAggregation).
		Aggregation("string", stringAggregation).
		Aggregation("index", indexAggregation).
		Aggregation("size", sizeAggregation)

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = timing.NewMetric("d").Start()
	var rel termAggregations
	err = json.Unmarshal(res.Aggregations["rel"], &rel)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var amount filteredMultiTermAggregations
	err = json.Unmarshal(res.Aggregations["amount"], &amount)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var time termAggregations
	err = json.Unmarshal(res.Aggregations["time"], &time)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var str termAggregations
	err = json.Unmarshal(res.Aggregations["string"], &str)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var index intValueAggregation
	err = json.Unmarshal(res.Aggregations["index"], &index)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	var size intValueAggregation
	err = json.Unmarshal(res.Aggregations["size"], &size)
	if err != nil {
		m.Stop()
		s.internalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	m.Stop()

	indexFilter := 0
	if index.Value > 0 {
		indexFilter++
	}

	sizeFilter := 0
	if size.Value > 0 {
		sizeFilter++
	}

	results := make([]searchFiltersResult, len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(time.Props.Buckets)+len(str.Props.Buckets)+indexFilter+sizeFilter)
	for i, bucket := range rel.Props.Buckets {
		results[i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "rel",
		}
	}
	for i, bucket := range amount.Filter.Props.Buckets {
		results[len(rel.Props.Buckets)+i] = searchFiltersResult{
			ID:    bucket.Key[0],
			Count: bucket.Docs.Count,
			Type:  "amount",
			Unit:  bucket.Key[1],
		}
	}
	for i, bucket := range time.Props.Buckets {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "time",
		}
	}
	for i, bucket := range str.Props.Buckets {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(time.Props.Buckets)+i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "string",
		}
	}
	if indexFilter != 0 {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(time.Props.Buckets)+len(str.Props.Buckets)] = searchFiltersResult{
			// This depends on TrackTotalHits being set to true.
			Count: res.Hits.TotalHits.Value,
			Type:  "index",
		}
	}
	if sizeFilter != 0 {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(time.Props.Buckets)+len(str.Props.Buckets)+indexFilter] = searchFiltersResult{
			Count: size.Value,
			Type:  "size",
		}
	}

	// Because we combine multiple aggregations of maxResultsCount each, we have to
	// re-sort results and limit them ourselves.
	slices.SortStableFunc(results, func(a searchFiltersResult, b searchFiltersResult) bool {
		return a.Count > b.Count
	})
	if len(results) > maxResultsCount {
		results = results[:maxResultsCount]
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(rel.Props.Buckets)) > rel.Total.Value {
		rel.Total.Value = int64(len(rel.Props.Buckets))
	}
	if int64(len(amount.Filter.Props.Buckets)) > amount.Filter.Total.Value {
		amount.Filter.Total.Value = int64(len(amount.Filter.Props.Buckets))
	}
	if int64(len(time.Props.Buckets)) > time.Total.Value {
		time.Total.Value = int64(len(time.Props.Buckets))
	}
	if int64(len(str.Props.Buckets)) > str.Total.Value {
		str.Total.Value = int64(len(str.Props.Buckets))
	}
	total := strconv.FormatInt(rel.Total.Value+amount.Filter.Total.Value+time.Total.Value+str.Total.Value+int64(indexFilter)+int64(sizeFilter), 10)

	metadata := http.Header{
		"Total": {total},
	}

	s.writeJSON(w, req, contentEncoding, results, metadata)
}
