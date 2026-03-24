package search

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
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
type multiTermAggregations struct {
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
}

// FilterResult describes an available filter as an union of possible fields for each supported filter type.
type FilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
	Type  string `json:"type"`
	Unit  string `json:"unit,omitempty"`
}

// FiltersGet retrieves all available filters for the current search.
func FiltersGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64, int64), searchSession *Session,
) ([]FilterResult, map[string]interface{}, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchSession.ToQuery()

	searchService, propertiesTotal, unitsTotal := getSearchService()
	relAggregation := elastic.NewNestedAggregation().Path("claims.rel").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("claims.rel.prop").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("claims.rel.prop").PrecisionThreshold(2*propertiesTotal), //nolint:mnd
	)
	amountAggregation := elastic.NewNestedAggregation().Path("claims.amount").SubAggregation(
		"props",
		elastic.NewMultiTermsAggregation().MultiTerms(
			elastic.MultiTerm{Field: "claims.amount.prop", Missing: nil},
			// Units are document IDs, so valid units can never be string "__missing__".
			elastic.MultiTerm{Field: "claims.amount.unit", Missing: "__missing__"},
		).Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal*unitsTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		// TODO: Use a runtime field.
		//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
		elastic.NewCardinalityAggregation().Script(
			// We use "|" as separator because this is used by ElasticSearch in "key_as_string" as well.
			// When unit is missing, "__missing__" is used as placeholder.
			elastic.NewScript(
				`return doc['claims.amount.prop'].value + '|' + (doc['claims.amount.unit'].size() > 0 ? doc['claims.amount.unit'].value : '__missing__')`,
			),
		).PrecisionThreshold(2*propertiesTotal*unitsTotal),
	)
	timeAggregation := elastic.NewNestedAggregation().Path("claims.time").SubAggregation(
		"props",
		elastic.NewTermsAggregation().Field("claims.time.prop").Size(MaxResultsCount).OrderByAggregation("docs", false).SubAggregation(
			"docs",
			elastic.NewReverseNestedAggregation(),
		),
	).SubAggregation(
		"total",
		// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
		// so we set precision threshold to twice as much to try to always get precise counts.
		elastic.NewCardinalityAggregation().Field("claims.time.prop").PrecisionThreshold(2*propertiesTotal), //nolint:mnd
	)
	searchService = searchService.Size(0).Query(query).
		Aggregation("rel", relAggregation).
		Aggregation("amount", amountAggregation).
		Aggregation("time", timeAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	m = metrics.Duration(internalStore.MetricJSONUnmarshal).Start()
	var rel termAggregations
	errE := x.Unmarshal(res.Aggregations["rel"], &rel)
	if errE != nil {
		m.Stop()
		return nil, nil, errE
	}
	var amount multiTermAggregations
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
	m.Stop()

	results := make([]FilterResult, len(rel.Props.Buckets)+len(amount.Props.Buckets)+len(timeA.Props.Buckets))
	for i, bucket := range rel.Props.Buckets {
		results[i] = FilterResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "rel",
			Unit:  "",
		}
	}
	for i, bucket := range amount.Props.Buckets {
		unit := bucket.Key[1]
		if unit == "__missing__" {
			unit = ""
		}
		results[len(rel.Props.Buckets)+i] = FilterResult{
			ID:    bucket.Key[0],
			Count: bucket.Docs.Count,
			Type:  "amount",
			Unit:  unit,
		}
	}
	for i, bucket := range timeA.Props.Buckets {
		results[len(rel.Props.Buckets)+len(amount.Props.Buckets)+i] = FilterResult{
			ID:    bucket.Key,
			Count: bucket.Docs.Count,
			Type:  "time",
			Unit:  "",
		}
	}

	// Because we combine multiple aggregations of MaxResultsCount each, we have to
	// re-sort results and limit them ourselves.
	slices.SortStableFunc(results, func(a FilterResult, b FilterResult) int {
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
	if int64(len(amount.Props.Buckets)) > amount.Total.Value {
		amount.Total.Value = int64(len(amount.Props.Buckets))
	}
	if int64(len(timeA.Props.Buckets)) > timeA.Total.Value {
		timeA.Total.Value = int64(len(timeA.Props.Buckets))
	}
	total := strconv.FormatInt(rel.Total.Value+amount.Total.Value+timeA.Total.Value, 10)

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
