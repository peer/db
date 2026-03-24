package search

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// FilterResult describes an available filter as an union of possible fields for each supported filter type.
type FilterResult struct {
	ID    string `json:"id"`
	Count int64  `json:"count"`
	Type  string `json:"type"`
	Unit  string `json:"unit,omitempty"`
}

// parseStringTermsBuckets converts string terms buckets with reverse-nested doc counts into FilterResult slices.
func parseStringTermsBuckets(buckets []types.StringTermsBucket, filterType string) ([]FilterResult, errors.E) {
	results := make([]FilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return nil, errE
		}
		results = append(results, FilterResult{
			ID:    key,
			Count: bucketDocs.DocCount,
			Type:  filterType,
			Unit:  "",
		})
	}
	return results, nil
}

// parseMultiTermsBuckets converts multi-terms buckets into FilterResult slices.
func parseMultiTermsBuckets(buckets []types.MultiTermsBucket) ([]FilterResult, errors.E) {
	results := make([]FilterResult, 0, len(buckets))
	for _, bucket := range buckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		if len(bucket.Key) < 2 { //nolint:mnd
			return nil, errors.New("unexpected key length for amount bucket")
		}
		propKey, ok := bucket.Key[0].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket prop")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[0])
			return nil, errE
		}
		unitKey, ok := bucket.Key[1].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket unit")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[1])
			return nil, errE
		}
		unit := unitKey
		if unit == "__missing__" {
			unit = ""
		}
		results = append(results, FilterResult{
			ID:    propKey,
			Count: bucketDocs.DocCount,
			Type:  "amount",
			Unit:  unit,
		})
	}
	return results, nil
}

// FiltersGet retrieves all available filters for the current search.
func FiltersGet(
	ctx context.Context, getSearchService func() (*search.Search, int64, int64), searchSession *Session,
) ([]FilterResult, map[string]interface{}, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchSession.ToQuery()

	searchService, propertiesTotal, unitsTotal := getSearchService()
	relAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.rel")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.rel.prop").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.rel.prop").PrecisionThreshold(int(2*propertiesTotal)))) //nolint:mnd
	amountAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.amount")).
		AddAggregation("props", esdsl.NewAggregations().
			MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
				esdsl.NewMultiTermLookup().Field("claims.amount.prop"),
				// Units are document IDs, so valid units can never be string "__missing__".
				esdsl.NewMultiTermLookup().Field("claims.amount.unit").Missing(esdsl.NewMissing().String("__missing__")),
			).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal*unitsTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			// TODO: Use a runtime field.
			//       See: https://www.elastic.co/guide/en/elasticsearch/reference/7.17/search-aggregations-metrics-cardinality-aggregation.html#_script_4
			Cardinality(esdsl.NewCardinalityAggregation().Script(
				// We use "|" as separator because this is used by ElasticSearch in "key_as_string" as well.
				// When unit is missing, "__missing__" is used as placeholder.
				esdsl.NewScript().Source(esdsl.NewScriptSource().String(
					`return doc['claims.amount.prop'].value + '|' + (doc['claims.amount.unit'].size() > 0 ? doc['claims.amount.unit'].value : '__missing__')`,
				)),
			).PrecisionThreshold(int(2*propertiesTotal*unitsTotal))))
	timeAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.time")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.time.prop").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.time.prop").PrecisionThreshold(int(2*propertiesTotal)))) //nolint:mnd
	searchService = searchService.Size(0).Query(query).
		AddAggregation("rel", relAggregation).
		AddAggregation("amount", amountAggregation).
		AddAggregation("time", timeAggregation)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	// Parse rel aggregation.
	relNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "rel")
	if errE != nil {
		return nil, nil, errE
	}
	relTerms, errE := aggAs[types.StringTermsAggregate](relNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	relBuckets, ok := relTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for rel")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", relTerms.Buckets)
		return nil, nil, errE
	}
	relTotal, errE := aggAs[types.CardinalityAggregate](relNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse amount aggregation.
	amountNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "amount")
	if errE != nil {
		return nil, nil, errE
	}
	amountTerms, errE := aggAs[types.MultiTermsAggregate](amountNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	amountBuckets, ok := amountTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for amount")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", amountTerms.Buckets)
		return nil, nil, errE
	}
	amountTotal, errE := aggAs[types.CardinalityAggregate](amountNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	// Parse time aggregation.
	timeNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "time")
	if errE != nil {
		return nil, nil, errE
	}
	timeTerms, errE := aggAs[types.StringTermsAggregate](timeNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	timeBuckets, ok := timeTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for time")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", timeTerms.Buckets)
		return nil, nil, errE
	}
	timeTotal, errE := aggAs[types.CardinalityAggregate](timeNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	relResults, errE := parseStringTermsBuckets(relBuckets, "rel")
	if errE != nil {
		return nil, nil, errE
	}
	amountResults, errE := parseMultiTermsBuckets(amountBuckets)
	if errE != nil {
		return nil, nil, errE
	}
	timeResults, errE := parseStringTermsBuckets(timeBuckets, "time")
	if errE != nil {
		return nil, nil, errE
	}

	results := make([]FilterResult, 0, len(relResults)+len(amountResults)+len(timeResults))
	results = append(results, relResults...)
	results = append(results, amountResults...)
	results = append(results, timeResults...)

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
	relTotalValue := relTotal.Value
	amountTotalValue := amountTotal.Value
	timeTotalValue := timeTotal.Value
	if int64(len(relBuckets)) > relTotalValue {
		relTotalValue = int64(len(relBuckets))
	}
	if int64(len(amountBuckets)) > amountTotalValue {
		amountTotalValue = int64(len(amountBuckets))
	}
	if int64(len(timeBuckets)) > timeTotalValue {
		timeTotalValue = int64(len(timeBuckets))
	}
	total := strconv.FormatInt(relTotalValue+amountTotalValue+timeTotalValue, 10)

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
