package search

import (
	"context"
	"slices"
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

//nolint:tagliatelle
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
	ID    string `json:"id,omitempty"`
	Count int64  `json:"count,omitempty"`
	Type  string `json:"type,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

func FiltersGet( //nolint:maintidx
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

	if !sh.Ready() {
		return nil, nil, errors.WithStack(ErrNotReady)
	}

	query := sh.Query()

	searchService, propertiesTotal := getSearchService()
	relAggregation := elastic.NewNestedAggregation().Path("claims.rel").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("claims.rel.prop.id").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("claims.rel.prop.id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
	)
	amountAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"filter",
		elastic.NewFilterAggregation().Filter(
			elastic.NewBoolQuery().MustNot(elastic.NewTermQuery("claims.amount.unit", "@")),
		).SubAggregation(
			"props",
			elastic.NewMultiTermsAggregation().Terms("claims.amount.prop.id", "claims.amount.unit").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
				"docs",
				elastic.NewReverseNestedAggregation(),
			),
		).SubAggregation(
			"total",
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal*AmountUnitsTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			// TODO: Use a runtime field.
			//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
			elastic.NewCardinalityAggregation().Script(
				// We use "|" as separator because this is used by ElasticSearch in "key_as_string" as well.
				elastic.NewScript("return doc['claims.amount.prop.id'].value + '|' + doc['claims.amount.unit'].value"),
			).PrecisionThreshold(2*propertiesTotal*int64(document.AmountUnitsTotal)),
		),
	)
	timeAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("claims.time.prop.id").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("claims.time.prop.id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
	)
	stringAggregation := elastic.NewNestedAggregation().Path("claims.string").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("claims.string.prop.id").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("claims.string.prop.id").PrecisionThreshold(2*propertiesTotal), //nolint:gomnd
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

	m = metrics.Duration(internal.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internal.MetricJSONUnmarshal).Start()
	var rel termAggregations
	errE := x.Unmarshal(res.Aggregations["rel"], &rel)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var amount filteredMultiTermAggregations
	errE = x.Unmarshal(res.Aggregations["amount"], &amount)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var timeA termAggregations
	errE = x.Unmarshal(res.Aggregations["time"], &timeA)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var str termAggregations
	errE = x.Unmarshal(res.Aggregations["string"], &str)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var index intValueAggregation
	errE = x.Unmarshal(res.Aggregations["index"], &index)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var size intValueAggregation
	errE = x.Unmarshal(res.Aggregations["size"], &size)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	m.Stop()

	indexFilter := 0
	if index.Value > 1 {
		indexFilter++
	}

	sizeFilter := 0
	if size.Value > 0 {
		sizeFilter++
	}

	results := make([]searchFiltersResult, len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(timeA.Props.Buckets)+len(str.Props.Buckets)+indexFilter+sizeFilter)
	for i, bucket := range rel.Props.Buckets {
		results[i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "rel",
			Unit:  "",
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
	for i, bucket := range timeA.Props.Buckets {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "time",
			Unit:  "",
		}
	}
	for i, bucket := range str.Props.Buckets {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(timeA.Props.Buckets)+i] = searchFiltersResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "string",
			Unit:  "",
		}
	}
	if indexFilter != 0 {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(timeA.Props.Buckets)+len(str.Props.Buckets)] = searchFiltersResult{
			ID: "",
			// This depends on TrackTotalHits being set to true.
			Count: res.Hits.TotalHits.Value,
			Type:  "index",
			Unit:  "",
		}
	}
	if sizeFilter != 0 {
		results[len(rel.Props.Buckets)+len(amount.Filter.Props.Buckets)+len(timeA.Props.Buckets)+len(str.Props.Buckets)+indexFilter] = searchFiltersResult{
			ID:    "",
			Count: size.Value,
			Type:  "size",
			Unit:  "",
		}
	}

	// Because we combine multiple aggregations of MaxResultsCount each, we have to
	// re-sort results and limit them ourselves.
	slices.SortStableFunc(results, func(a searchFiltersResult, b searchFiltersResult) int {
		if a.Count > b.Count {
			return -1
		} else if a.Count < b.Count {
			return 1
		}
		return 0
	})
	if len(results) > MaxResultsCount {
		results = results[:MaxResultsCount]
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	if int64(len(rel.Props.Buckets)) > rel.Total.Value {
		rel.Total.Value = int64(len(rel.Props.Buckets))
	}
	if int64(len(amount.Filter.Props.Buckets)) > amount.Filter.Total.Value {
		amount.Filter.Total.Value = int64(len(amount.Filter.Props.Buckets))
	}
	if int64(len(timeA.Props.Buckets)) > timeA.Total.Value {
		timeA.Total.Value = int64(len(timeA.Props.Buckets))
	}
	if int64(len(str.Props.Buckets)) > str.Total.Value {
		str.Total.Value = int64(len(str.Props.Buckets))
	}
	total := strconv.FormatInt(rel.Total.Value+amount.Filter.Total.Value+timeA.Total.Value+str.Total.Value+int64(indexFilter)+int64(sizeFilter), 10)

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
