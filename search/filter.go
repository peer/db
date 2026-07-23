// Package search provides search functionality including filters and result handling.
//
// Counting exactness: document-level counts (universe sizes, missing and special-value counts,
// facet availability, active-filter counts) compile as root-level filter aggregations, which count
// documents and are therefore exact by construction. Top-level facets enumerate buckets in a
// single collection each (value lists over the rel collection, each histogram over its value
// collection), so their bucket counts are exact as well. Sub facets that span a single parent
// collection have counts exact as well.
//
// Sub facets that span multiple parent collections: their bucket-enumerated counts (value lists,
// direct counts, histogram buckets, discovery enumerations) run once per parent collection (exact
// individually) and merge in Go by bucket key, with document counts summed, so a document contributing
// the same bucket key through two parent collections (its parent property stated in two of the
// collections, for example as a ref claim and as a string claim, with the same sub-value under
// each) counts once per collection, which means that they overcount the number of documents, but
// this is just cosmetic (shown as numbers to users) because UI does not rely on the precise count.
// Claim types within the rel collection (ref, has, none, unknown) do not trigger this: they share
// one collection, so their sub records count in one context.
//
// The other additive counts are the headline reachable-through counts of amount and
// time facets, which add the property's valueless (has/none/unknown) document count to the valued
// one, so a document with both a valued and a valueless statement for the property counts twice,
// overcounting the number of documents shown in the facet headline. Also just cosmetic
// (shown as numbers to users) because UI does not rely on the precise count.
//
// childCount (distinct child values) and distinct-value totals have to be exact up to MaxResultsCount
// for the UI and this is why we use set counts for them. For set counts, undercounting is harmful,
// overcounting is merely cosmetic. At top level, set counts come from a single collection: childCount
// by a cardinality aggregation, the totals by the terms bucket count while unsaturated with a
// cardinality estimate past the cap. Sub facets union the per-collection terms keys in Go instead of
// summing cardinalities, exact while no per-collection terms aggregation saturated. Past the cap the
// sub distinct-value total falls back to the summed per-collection cardinalities (never reporting
// fewer values than the keys held). The sub childCount has no fallback and clamps at its key set
// size (safe: a clamped childCount still exceeds anything the value list can load, see the
// childCounts aggregation comment). The pooled sub-has total is the merged bucket count, a
// lower bound.
package search

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	histogramBins = 100

	// boundsExpansion is the fraction of the selected range added on each side when session
	// bounds are set, so the slider can be dragged outward to widen the current selection. The
	// expanded bounds are clamped to the unfiltered data range (see histogramGet).
	boundsExpansion = 0.1

	// Metadata keys of the identity and special-value counts facets report. They are also used as
	// the corresponding aggregation names. The Metadata response header holds them as structured
	// field dictionary keys, which must be lowercase.
	existsKey      = "exists"
	hasPropertyKey = "has_property"
	unknownKey     = "unknown"
	noneKey        = "none"
	missingKey     = "missing"
	universeKey    = "universe"
	otherTypesKey  = "other_types"
)

// HistogramResult represents count for a single bucket in a filter histogram.
type HistogramResult struct {
	From  float64 `json:"from"`
	Count int64   `json:"count"`
}

// histContext is one aggregation context a histogram facet spans: a top-level facet has a single
// context on its value collection, a sub facet one context per parent collection its parent
// context allows. Path is the value collection's nested path; parent and pf are set for sub
// contexts (the parent collection's path and the parent-level filter). Filter is the value-level
// filter (the property term, plus the unit condition for amounts).
type histContext struct {
	Name         string
	Parent       string
	Path         string
	ParentFilter types.QueryVariant
	Filter       types.QueryVariant
}

// Wrap nests the given inner aggregations into the context's aggregation chain: for a top-level
// context nested(path) -> filter; for a sub context nested(parent) -> pf -> nested(path) -> filter.
func (h *histContext) Wrap(inner *types.Aggregations) types.AggregationsVariant { //nolint:ireturn
	filtered := esdsl.NewAggregations().Filter(h.Filter)
	for name, agg := range inner.Aggregations {
		filtered = filtered.AddAggregation(name, &agg)
	}
	valueLevel := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(h.Path)).
		AddAggregation("filter", filtered)
	if h.Parent == "" {
		return valueLevel
	}
	return esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(parentPath(h.Parent))).
		AddAggregation("parentFilter", esdsl.NewAggregations().
			Filter(h.ParentFilter).
			AddAggregation("value", valueLevel))
}

// Unwrap walks the context's aggregation chain in a response back down to the "filter" level.
func (h *histContext) Unwrap(aggs map[string]types.Aggregate) (map[string]types.Aggregate, errors.E) {
	level := aggs
	if h.Parent != "" {
		parentNested, errE := internalSearch.AggAs[types.NestedAggregate](level, h.Name)
		if errE != nil {
			return nil, errE
		}
		pf, errE := internalSearch.AggAs[types.FilterAggregate](parentNested.Aggregations, "parentFilter")
		if errE != nil {
			return nil, errE
		}
		valueNested, errE := internalSearch.AggAs[types.NestedAggregate](pf.Aggregations, "value")
		if errE != nil {
			return nil, errE
		}
		level = valueNested.Aggregations
	} else {
		nested, errE := internalSearch.AggAs[types.NestedAggregate](level, h.Name)
		if errE != nil {
			return nil, errE
		}
		level = nested.Aggregations
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](level, "filter")
	if errE != nil {
		return nil, errE
	}
	return filter.Aggregations, nil
}

// MinMaxInner builds the endpoint aggregations of the min/max phase. Claims with an open (none)
// end index only the endpoint they have, so aggregating just min over from and max over to could
// miss known endpoints or return no value at all. Their known endpoints are aggregated separately
// (openStart and openEnd), both to extend the combined range and because an open start claim
// determining the min requires lowering the histogram start (its to is an exclusive range upper
// bound).
func (h *histContext) MinMaxInner() *types.Aggregations {
	return esdsl.NewAggregations().
		AddAggregation("minFrom", esdsl.NewAggregations().
			Min(esdsl.NewMinAggregation().Field(h.Path+".from"))).
		AddAggregation("maxTo", esdsl.NewAggregations().
			Max(esdsl.NewMaxAggregation().Field(h.Path+".to"))).
		AddAggregation("openStart", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field(h.Path+".from"))).
			AddAggregation("minTo", esdsl.NewAggregations().
				Min(esdsl.NewMinAggregation().Field(h.Path+".to")))).
		AddAggregation("openEnd", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field(h.Path+".to"))).
			AddAggregation("maxFrom", esdsl.NewAggregations().
				Max(esdsl.NewMaxAggregation().Field(h.Path+".from")))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		AggregationsCaster()
}

// mergedMinMax accumulates the min/max phase results across the facet's contexts.
type mergedMinMax struct {
	DocCount   int64
	MinValue   *float64
	MaxValue   *float64
	MinIsToEnd bool
}

// Fold merges one context's min/max endpoint aggregations. Min is the smallest and max the largest
// known endpoint value; minIsToEnd reports whether the min is determined by an open start claim's
// to value (also on a tie with a from value), which requires lowering the histogram start (known
// to endpoints are indexed as exclusive range upper bounds).
func (m *mergedMinMax) Fold(aggs map[string]types.Aggregate) errors.E {
	docs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](aggs, "docs")
	if errE != nil {
		return errE
	}
	m.DocCount += docs.DocCount
	minFromAgg, errE := internalSearch.AggAs[types.MinAggregate](aggs, "minFrom")
	if errE != nil {
		return errE
	}
	maxToAgg, errE := internalSearch.AggAs[types.MaxAggregate](aggs, "maxTo")
	if errE != nil {
		return errE
	}
	openStart, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "openStart")
	if errE != nil {
		return errE
	}
	minToAgg, errE := internalSearch.AggAs[types.MinAggregate](openStart.Aggregations, "minTo")
	if errE != nil {
		return errE
	}
	openEnd, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "openEnd")
	if errE != nil {
		return errE
	}
	maxFromAgg, errE := internalSearch.AggAs[types.MaxAggregate](openEnd.Aggregations, "maxFrom")
	if errE != nil {
		return errE
	}
	if minFromAgg.Value != nil {
		v := float64(*minFromAgg.Value)
		if m.MinValue == nil || v < *m.MinValue {
			m.MinValue = &v
			m.MinIsToEnd = false
		}
	}
	if minToAgg.Value != nil {
		v := float64(*minToAgg.Value)
		if m.MinValue == nil || v <= *m.MinValue {
			m.MinValue = &v
			m.MinIsToEnd = true
		}
	}
	if maxToAgg.Value != nil {
		v := float64(*maxToAgg.Value)
		if m.MaxValue == nil || v > *m.MaxValue {
			m.MaxValue = &v
		}
	}
	if maxFromAgg.Value != nil {
		v := float64(*maxFromAgg.Value)
		if m.MaxValue == nil || v > *m.MaxValue {
			m.MaxValue = &v
		}
	}
	return nil
}

// expandSessionBounds widens the selected [from, to] range by boundsExpansion of its span on each
// side so the slider can be dragged outward to expand the selection, clamped to the unfiltered data
// range [dataMin, dataMax] so it never extends past the data. A selection already wider than the
// data (extended bounds) is kept as is so the current selection always fits within the slider.
func expandSessionBounds(from, to, dataMin, dataMax float64) (float64, float64) {
	margin := (to - from) * boundsExpansion
	return min(from, max(from-margin, dataMin)), max(to, min(to+margin, dataMax))
}

// identityCounts adds the facet's identity and special-value count aggregations (each a root-level
// filter aggregation) and returns the order their counts fold into metadata.
func identityCounts(searchService *esSearch.Search, counts map[string]types.QueryVariant, order []string) *esSearch.Search {
	for _, name := range order {
		searchService = searchService.AddAggregation("count:"+name, esdsl.NewAggregations().Filter(counts[name]))
	}
	return searchService
}

// parseIdentityCounts reads the identity and special-value counts into metadata.
func parseIdentityCounts(aggs map[string]types.Aggregate, order []string, metadata map[string]any) errors.E {
	for _, name := range order {
		agg, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "count:"+name)
		if errE != nil {
			return errE
		}
		metadata[name] = agg.DocCount
	}
	return nil
}

// histogramGet retrieves histogram filter data for search results across the facet's aggregation
// contexts (one for a top-level facet, one per parent collection for a sub facet). It runs a
// min/max phase (merged across contexts) followed by a histogram phase with a shared interval and
// offset, buckets merged by key with document counts summed. The caller excludes the facet's own
// filters from the session query, so the histogram shows values available under the other filters.
//
// If sessionFrom and sessionTo are non-nil, the histogram range is the selected range widened by a
// margin (boundsExpansion of the selected span) on each side and clamped to the data min/max, so
// the slider can be dragged outward to expand the selection without ever extending past the data.
// A selection wider than the data ("extended bounds") is kept as is so the current selection always
// fits within the slider. When the data has a single known endpoint value, a single bucket is
// returned even when session bounds are set, so that selecting the single value round-trips to the
// same response.
//
// counts and countsOrder are the facet's identity and special-value count queries (missing,
// universe, and the specials), computed as root filter aggregations in the first phase and folded
// into the metadata under their names.
//
// stepDown lowers the histogram start when the min known endpoint value is determined by a
// to value: to values are indexed as exclusive range upper bounds, so a claim ending exactly
// at the min would not overlap a first bucket starting there. It is given the value and the
// histogram span and returns a value below it, by one step of the value's apparent precision
// (time or amount specific).
func histogramGet( //nolint:maintidx
	ctx context.Context,
	getSearchService func() *esSearch.Search,
	query types.QueryVariant,
	contexts []histContext,
	sessionFrom, sessionTo *float64,
	stepDown func(v, span float64) float64,
	counts map[string]types.QueryVariant,
	countsOrder []string,
) ([]HistogramResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	// Run min/max aggregations per context to determine the data range and doc count, alongside the
	// identity counts.
	minMaxSearchService := getSearchService().Size(0).Query(query)
	for i := range contexts {
		h := &contexts[i]
		minMaxSearchService = minMaxSearchService.AddAggregation(h.Name, h.Wrap(h.MinMaxInner()))
	}
	minMaxSearchService = identityCounts(minMaxSearchService, counts, countsOrder)

	// For point session bounds (gte equal to lte) the histogram collapses to a single bucket
	// and the availability count would not correspond to any click outcome, so the documents
	// actually matching the point bounds are counted as well, mirroring how the filter query
	// itself matches them (the range field intersecting the bounds).
	pointSession := sessionFrom != nil && sessionTo != nil && *sessionFrom == *sessionTo
	if pointSession {
		for i := range contexts {
			h := &contexts[i]
			selectedInner := esdsl.NewAggregations().
				AddAggregation("selected", esdsl.NewAggregations().
					Filter(esdsl.NewNumberRangeQuery(h.Path+".range").Gte(types.Float64(*sessionFrom)).Lte(types.Float64(*sessionTo))).
					AddAggregation("docs", esdsl.NewAggregations().
						ReverseNested(esdsl.NewReverseNestedAggregation()))).
				AggregationsCaster()
			minMaxSearchService = minMaxSearchService.AddAggregation("selected:"+h.Name, h.WithName("selected:"+h.Name).Wrap(selectedInner))
		}
	}

	m := metrics.Duration(internalStore.MetricElasticSearch1).Start()
	res, err := minMaxSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.Took) * time.Millisecond

	merged := mergedMinMax{DocCount: 0, MinValue: nil, MaxValue: nil, MinIsToEnd: false}
	for i := range contexts {
		h := &contexts[i]
		aggs, errE := h.Unwrap(res.Aggregations)
		if errE != nil {
			return nil, nil, errE
		}
		errE = merged.Fold(aggs)
		if errE != nil {
			return nil, nil, errE
		}
	}

	metadata := map[string]any{}
	errE := parseIdentityCounts(res.Aggregations, countsOrder, metadata)
	if errE != nil {
		return nil, nil, errE
	}

	if merged.DocCount == 0 {
		metadata["total"] = "0"
		return []HistogramResult{}, metadata, nil
	}

	// No known endpoint values, so there is nothing to span a histogram with. This happens when all
	// matching claims have both endpoints open (none): such claims index only sentinel range bounds.
	// Claims with unknown endpoints never appear here because the converter collapses an interval
	// with one unknown endpoint to a point claim and converts a fully unknown interval to an
	// unknown record, which is not indexed under the value path at all.
	if merged.MinValue == nil || merged.MaxValue == nil {
		metadata["total"] = "0"
		return []HistogramResult{}, metadata, nil
	}

	// The data has a single known endpoint value, return a single bucket, even when session
	// bounds are set, so that selecting the single value round-trips to the same response.
	// The from and to metadata bounds can be used by the client to filter to the single
	// value. When the value is a known to endpoint of an open start claim, the from bound is
	// lowered by one precision step like for the histogram, because such a claim does not
	// contain the value itself (to endpoints are indexed as exclusive range upper bounds).
	// There is no histogram span to refine the step against, so the step is unrefined.
	if *merged.MinValue == *merged.MaxValue {
		fromValue := *merged.MinValue
		if merged.MinIsToEnd {
			fromValue = stepDown(fromValue, math.Inf(1))
		}
		metadata["total"] = "1"
		metadata["from"] = strconv.FormatFloat(fromValue, 'f', -1, 64)
		metadata["to"] = strconv.FormatFloat(*merged.MaxValue, 'f', -1, 64)
		return []HistogramResult{{From: *merged.MinValue, Count: merged.DocCount}}, metadata, nil
	}

	var histogramFrom, histogramTo float64
	if sessionFrom != nil && sessionTo != nil {
		// Use session bounds directly.
		histogramFrom = *sessionFrom
		histogramTo = *sessionTo
		// Equal session bounds cannot span a histogram, return a single bucket at the value,
		// counting the documents actually matching the point bounds.
		if histogramFrom == histogramTo {
			var selectedCount int64
			for i := range contexts {
				h := contexts[i].WithName("selected:" + contexts[i].Name)
				aggs, errE := h.Unwrap(res.Aggregations)
				if errE != nil {
					return nil, nil, errE
				}
				selected, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "selected")
				if errE != nil {
					return nil, nil, errE
				}
				docs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](selected.Aggregations, "docs")
				if errE != nil {
					return nil, nil, errE
				}
				selectedCount += docs.DocCount
			}
			valString := strconv.FormatFloat(histogramFrom, 'f', -1, 64)
			metadata["total"] = "1"
			metadata["from"] = valString
			metadata["to"] = valString
			return []HistogramResult{{From: histogramFrom, Count: selectedCount}}, metadata, nil
		}
		// Widen the selection so the slider can be dragged outward, clamped to the data range.
		histogramFrom, histogramTo = expandSessionBounds(histogramFrom, histogramTo, *merged.MinValue, *merged.MaxValue)
	} else {
		histogramFrom = *merged.MinValue
		histogramTo = *merged.MaxValue
		if merged.MinIsToEnd {
			// The min is a known to endpoint and those are indexed as exclusive range upper bounds,
			// so a claim ending exactly at the min would not overlap a first bucket starting there.
			// Lower the histogram start so that such claims are counted.
			histogramFrom = stepDown(histogramFrom, histogramTo-histogramFrom)
		}
	}

	// Compute interval and upper bound for the histogram. The upper bound may be
	// adjusted (e.g., rounded up for integer intervals) so the range is evenly divisible.
	interval, upperBound, intervalString := computeInterval(histogramFrom, histogramTo)

	// Compute offset so that bucket boundaries align with histogramFrom.
	offset := math.Mod(histogramFrom, interval)
	if offset < 0 {
		offset += interval
	}

	// Second query: the histogram per context, with the shared interval, offset, and bounds, so
	// bucket keys align and can be merged by key.
	histogramSearchService := getSearchService().Size(0).Query(query)
	for i := range contexts {
		h := &contexts[i]
		histInner := esdsl.NewAggregations().
			AddAggregation("hist", esdsl.NewAggregations().
				Histogram(esdsl.NewHistogramAggregation().
					Field(h.Path+".range").
					Interval(types.Float64(interval)).
					Offset(types.Float64(offset)).
					ExtendedBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(histogramFrom)).Max(types.Float64(upperBound))).
					HardBounds(esdsl.NewExtendedBoundsdouble().Min(types.Float64(histogramFrom)).Max(types.Float64(upperBound)))).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))).
			AggregationsCaster()
		histogramSearchService = histogramSearchService.AddAggregation(h.Name, h.Wrap(histInner))
	}

	m = metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err = histogramSearchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal2).Duration = time.Duration(res.Took) * time.Millisecond

	// Merge buckets by key: the extended bounds make every context produce the same bucket keys, so
	// the merge indexes into one slice. Counts sum across contexts; a document contributing the same
	// bucket through two parent collections is the residual overcount described in the package
	// comment (the same property stated in two value types with the same sub-value).
	var results []HistogramResult
	byKey := map[float64]int{}
	for i := range contexts {
		h := &contexts[i]
		aggs, errE := h.Unwrap(res.Aggregations)
		if errE != nil {
			return nil, nil, errE
		}
		histAgg, errE := internalSearch.AggAs[types.HistogramAggregate](aggs, "hist")
		if errE != nil {
			return nil, nil, errE
		}
		buckets, ok := histAgg.Buckets.([]types.HistogramBucket)
		if !ok {
			errE := errors.New("unexpected bucket type for histogram")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", histAgg.Buckets)
			return nil, nil, errE
		}
		for _, bucket := range buckets {
			bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
			if errE != nil {
				return nil, nil, errE
			}
			key := float64(bucket.Key)
			if idx, ok := byKey[key]; ok {
				results[idx].Count += bucketDocs.DocCount
				continue
			}
			byKey[key] = len(results)
			results = append(results, HistogramResult{From: key, Count: bucketDocs.DocCount})
		}
	}

	metadata["total"] = strconv.Itoa(len(results))
	metadata["from"] = strconv.FormatFloat(histogramFrom, 'f', -1, 64)
	metadata["to"] = strconv.FormatFloat(histogramTo, 'f', -1, 64)
	metadata["interval"] = intervalString

	return results, metadata, nil
}

// WithName returns a copy of the context under a different aggregation name, used when the same
// context appears twice in one request (for example the point-session selected count).
func (h *histContext) WithName(name string) *histContext {
	c := *h
	c.Name = name
	return &c
}
