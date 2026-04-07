package search

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// FilterResult describes an available filter as an union of possible fields for each supported filter type.
type FilterResult struct {
	Props    []string `json:"props,omitempty"`
	Type     string   `json:"type"`
	Unit     string   `json:"unit,omitempty"`
	FilterID string   `json:"filterId,omitempty"`
	Count    int64    `json:"count"`
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
			Props:    []string{key},
			Type:     filterType,
			Unit:     "",
			FilterID: "",
			Count:    bucketDocs.DocCount,
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
			Props:    []string{propKey},
			Type:     "amount",
			Unit:     unit,
			FilterID: "",
			Count:    bucketDocs.DocCount,
		})
	}
	return results, nil
}

// FiltersGet retrieves all available filters for the current search.
func FiltersGet( //nolint:maintidx
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64), searchSession *Session,
) ([]FilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchSession.ToQuery()

	searchService, propertiesTotal, unitsTotal := getSearchService()
	refAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("props", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field("claims.ref.prop").Size(MaxResultsCount).
				Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			// Cardinality aggregation returns the count of all buckets. It can be at most propertiesTotal,
			// so we set precision threshold to twice as much to try to always get precise counts.
			Cardinality(esdsl.NewCardinalityAggregation().Field("claims.ref.prop").PrecisionThreshold(int(2*propertiesTotal)))) //nolint:mnd
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
	// Has aggregation counts documents that have at least one has claim.
	// Only simple has claims (without sub-claims) are indexed in claims.has, so no
	// additional filtering is needed. Unlike other filter types, has produces a single
	// filter rather than one per property.
	hasAggregation := esdsl.NewAggregations().
		Filter(esdsl.NewNestedQuery(
			esdsl.NewMatchAllQuery(),
		).Path("claims.has"))
	// SubRef aggregation discovers available (parentProp, prop) combinations across all sub-references.
	subRefAggregation := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.sub")).
		AddAggregation("props", esdsl.NewAggregations().
			MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
				esdsl.NewMultiTermLookup().Field("claims.sub.parentProp"),
				esdsl.NewMultiTermLookup().Field("claims.sub.prop"),
			).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("total", esdsl.NewAggregations().
			Cardinality(esdsl.NewCardinalityAggregation().Script(
				esdsl.NewScript().Source(esdsl.NewScriptSource().String(
					`return doc['claims.sub.parentProp'].value + '|' + doc['claims.sub.prop'].value`,
				)),
			).PrecisionThreshold(int(2*propertiesTotal*propertiesTotal))))
	searchService = searchService.Size(0).Query(query).
		AddAggregation("ref", refAggregation).
		AddAggregation("amount", amountAggregation).
		AddAggregation("time", timeAggregation).
		AddAggregation("has", hasAggregation).
		AddAggregation("subRef", subRefAggregation)

	// For each active filter, add an aggregation that computes the property count
	// excluding that filter's own restriction. This ensures active filters always
	// appear in results with correct counts.
	for i, f := range searchSession.Filters {
		if f.ID == nil {
			// This should not be possible.
			continue
		}
		if f.Has != nil {
			// Has filter uses a different aggregation structure since it is global (no specific prop).
			// Only simple has claims (without sub-claims) are indexed in claims.has.
			activeAgg := esdsl.NewAggregations().
				Filter(searchSession.ToQueryExcluding(*f.ID)).
				AddAggregation("count", esdsl.NewAggregations().
					Filter(esdsl.NewNestedQuery(
						esdsl.NewMatchAllQuery(),
					).Path("claims.has")))
			searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
			continue
		}
		if f.Ref != nil && len(f.Prop) == 2 {
			// SubRef filter: aggregate on claims.sub with parentProp + prop filter.
			activeAgg := esdsl.NewAggregations().
				Filter(searchSession.ToQueryExcluding(*f.ID)).
				AddAggregation("count", esdsl.NewAggregations().
					Filter(esdsl.NewNestedQuery(
						esdsl.NewBoolQuery().Must(
							esdsl.NewTermQuery("claims.sub.parentProp", esdsl.NewFieldValue().String(f.Prop[0].String())),
							esdsl.NewTermQuery("claims.sub.prop", esdsl.NewFieldValue().String(f.Prop[1].String())),
						),
					).Path("claims.sub")))
			searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
			continue
		}
		prop := f.Prop[0]
		var nestedPath string
		switch {
		case f.Ref != nil:
			nestedPath = "claims.ref"
		case f.Amount != nil:
			nestedPath = "claims.amount"
		case f.Time != nil:
			nestedPath = "claims.time"
		default:
			// This should not be possible.
			continue
		}
		activeAgg := esdsl.NewAggregations().
			Filter(searchSession.ToQueryExcluding(*f.ID)).
			AddAggregation("nested", esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(nestedPath)).
				AddAggregation("filter", esdsl.NewAggregations().
					Filter(esdsl.NewTermQuery(nestedPath+".prop", esdsl.NewFieldValue().String(prop.String()))).
					AddAggregation("docs", esdsl.NewAggregations().
						ReverseNested(esdsl.NewReverseNestedAggregation()))))
		searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	// Parse ref aggregation.
	refNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "ref")
	if errE != nil {
		return nil, nil, errE
	}
	refTerms, errE := aggAs[types.StringTermsAggregate](refNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	refBuckets, ok := refTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for ref")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", refTerms.Buckets)
		return nil, nil, errE
	}
	refTotal, errE := aggAs[types.CardinalityAggregate](refNested.Aggregations, "total")
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

	// Parse has aggregation.
	hasFilterAgg, errE := aggAs[types.FilterAggregate](res.Aggregations, "has")
	if errE != nil {
		return nil, nil, errE
	}
	hasDocCount := hasFilterAgg.DocCount

	// Parse subRef aggregation.
	subRefNested, errE := aggAs[types.NestedAggregate](res.Aggregations, "subRef")
	if errE != nil {
		return nil, nil, errE
	}
	subRefTerms, errE := aggAs[types.MultiTermsAggregate](subRefNested.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	subRefBuckets, ok := subRefTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for subRef")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", subRefTerms.Buckets)
		return nil, nil, errE
	}
	subRefTotal, errE := aggAs[types.CardinalityAggregate](subRefNested.Aggregations, "total")
	if errE != nil {
		return nil, nil, errE
	}

	refResults, errE := parseStringTermsBuckets(refBuckets, "ref")
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

	// Parse subRef multi-terms buckets into FilterResult entries with 2-element Props.
	subRefResults := make([]FilterResult, 0, len(subRefBuckets))
	for _, bucket := range subRefBuckets {
		bucketDocs, errE := aggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, nil, errE
		}
		if len(bucket.Key) < 2 { //nolint:mnd
			return nil, nil, errors.New("unexpected key length for subRef bucket")
		}
		parentPropKey, ok := bucket.Key[0].(string)
		if !ok {
			errE := errors.New("unexpected key type for subRef bucket parentProp")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[0])
			return nil, nil, errE
		}
		propKey, ok := bucket.Key[1].(string)
		if !ok {
			errE := errors.New("unexpected key type for subRef bucket prop")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[1])
			return nil, nil, errE
		}
		subRefResults = append(subRefResults, FilterResult{
			Props:    []string{parentPropKey, propKey},
			Type:     "ref",
			Unit:     "",
			FilterID: "",
			Count:    bucketDocs.DocCount,
		})
	}

	results := make([]FilterResult, 0, len(refResults)+len(amountResults)+len(timeResults)+len(subRefResults)+1)
	results = append(results, refResults...)
	results = append(results, amountResults...)
	results = append(results, timeResults...)
	results = append(results, subRefResults...)

	// Add has filter result if any documents have has claims.
	if hasDocCount > 0 {
		results = append(results, FilterResult{
			Props:    nil,
			Type:     "has",
			Unit:     "",
			FilterID: "",
			Count:    hasDocCount,
		})
	}

	// Parse per-active-filter aggregation results and append them with FilterID set.
	// Main results (without FilterID) remain as inactive filter options.
	for i, f := range searchSession.Filters {
		if f.ID == nil {
			// This should not be possible.
			continue
		}

		var result FilterResult
		if f.Has != nil {
			result = FilterResult{Props: nil, Type: "has", Unit: "", FilterID: f.ID.String(), Count: 0}
		} else {
			props := make([]string, 0, len(f.Prop))
			for _, p := range f.Prop {
				props = append(props, p.String())
			}
			switch {
			case f.Ref != nil:
				result = FilterResult{Props: props, Type: "ref", Unit: "", FilterID: f.ID.String(), Count: 0}
			case f.Amount != nil:
				result = FilterResult{Props: props, Type: "amount", Unit: "", FilterID: f.ID.String(), Count: 0}
				if f.Amount.Unit != nil {
					result.Unit = f.Amount.Unit.String()
				}
			case f.Time != nil:
				result = FilterResult{Props: props, Type: "time", Unit: "", FilterID: f.ID.String(), Count: 0}
			default:
				// This should not be possible.
				continue
			}
		}

		aggName := fmt.Sprintf("active_%d", i)
		activeFilter, errE := aggAs[types.FilterAggregate](res.Aggregations, aggName)
		if errE != nil {
			return nil, nil, errE
		}

		if f.Has != nil || (f.Ref != nil && len(f.Prop) == 2) {
			// Has and subRef filters use a "count" sub-aggregation structure.
			countFilter, errE := aggAs[types.FilterAggregate](activeFilter.Aggregations, "count")
			if errE != nil {
				return nil, nil, errE
			}
			result.Count = countFilter.DocCount
		} else {
			activeNested, errE := aggAs[types.NestedAggregate](activeFilter.Aggregations, "nested")
			if errE != nil {
				return nil, nil, errE
			}
			propFilter, errE := aggAs[types.FilterAggregate](activeNested.Aggregations, "filter")
			if errE != nil {
				return nil, nil, errE
			}
			activeDocs, errE := aggAs[types.ReverseNestedAggregate](propFilter.Aggregations, "docs")
			if errE != nil {
				return nil, nil, errE
			}
			result.Count = activeDocs.DocCount
		}

		results = append(results, result)
	}

	// Sort: active filters first, then inactive, each group by count descending.
	slices.SortStableFunc(results, func(a FilterResult, b FilterResult) int {
		aActive := a.FilterID != ""
		bActive := b.FilterID != ""
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		return cmp.Compare(b.Count, a.Count)
	})
	if len(results) > MaxResultsCount {
		results = results[:MaxResultsCount]
	}

	// Cardinality count is approximate, so we make sure the total is sane.
	// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate
	refTotalValue := refTotal.Value
	amountTotalValue := amountTotal.Value
	timeTotalValue := timeTotal.Value
	if int64(len(refBuckets)) > refTotalValue {
		refTotalValue = int64(len(refBuckets))
	}
	if int64(len(amountBuckets)) > amountTotalValue {
		amountTotalValue = int64(len(amountBuckets))
	}
	if int64(len(timeBuckets)) > timeTotalValue {
		timeTotalValue = int64(len(timeBuckets))
	}
	// Has filter contributes at most 1 to the total (one filter for all has claims).
	var hasTotalValue int64
	if hasDocCount > 0 {
		hasTotalValue = 1
	}
	subRefTotalValue := max(int64(len(subRefBuckets)), subRefTotal.Value)
	total := strconv.FormatInt(refTotalValue+amountTotalValue+timeTotalValue+hasTotalValue+subRefTotalValue, 10)

	return results, map[string]any{
		"total": total,
	}, nil
}
