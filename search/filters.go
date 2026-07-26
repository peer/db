package search

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
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

// matchedDocsAggregation builds a discovery facet's per-bucket "matched" filter holding a "matchedDocs"
// reverse-nested count. It runs the value-query match over the bucket's records; the resulting count decides
// only whether the facet is kept, never the facet's reported count (which is the bucket's full "docs" count).
func matchedDocsAggregation(match types.QueryVariant) *types.Aggregations {
	return esdsl.NewAggregations().
		Filter(match).
		AddAggregation("matchedDocs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		AggregationsCaster()
}

// matchedDocCount reads a discovery bucket's per-bucket "matched" filter and its "matchedDocs" reverse-nested
// count, the number of the bucket's records that match the value query. It is used only to decide whether a
// facet is kept, never as the facet's reported count.
func matchedDocCount(aggs map[string]types.Aggregate) (int64, errors.E) {
	matched, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "matched")
	if errE != nil {
		return 0, errE
	}
	matchedDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](matched.Aggregations, "matchedDocs")
	if errE != nil {
		return 0, errE
	}
	return matchedDocs.DocCount, nil
}

// relClaimTypeSet records which rel claim types a property has in scope.
type relClaimTypeSet struct {
	Ref     bool
	Has     bool
	None    bool
	Unknown bool
}

// PooledHas reports whether the property belongs to the pooled has facet: its only rel claim
// type is has. Amount and time claims are checked separately by the caller.
func (k relClaimTypeSet) PooledHas() bool {
	return k.Has && !k.Ref && !k.None && !k.Unknown
}

// relFacetInfo is the parsed discovery data of one property's rel records within one context.
type relFacetInfo struct {
	Docs          int64
	ValuelessDocs int64
	ClaimTypes    relClaimTypeSet
	// HasHierarchy is true when the property has a non-leaf ref record (a value expanded as an ancestor
	// of a narrower value), the marker of a hierarchy that can carry a direct entry. It gates the direct
	// special so that search surfaces only the ref facets that can show a direct row, not every ref facet.
	HasHierarchy bool
	Matched      int64
}

// parseRelFacetBuckets parses a rel discovery terms aggregation (buckets keyed by prop, each with
// "docs", "valueless", "claimTypes", and optionally "matched" sub-aggregations) into per-property
// info, merged into out by summing counts and unioning claim types (used both for the single top-level context
// and the per-parent-collection sub contexts).
func parseRelFacetBuckets(buckets []types.StringTermsBucket, valueQueryActive bool, out map[string]*relFacetInfo, order *[]string) errors.E {
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for rel facet bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return errE
		}
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		valueless, errE := internalSearch.AggAs[types.FilterAggregate](bucket.Aggregations, "valueless")
		if errE != nil {
			return errE
		}
		valuelessDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](valueless.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		claimTypesAgg, errE := internalSearch.AggAs[types.StringTermsAggregate](bucket.Aggregations, "claimTypes")
		if errE != nil {
			return errE
		}
		hierarchy, errE := internalSearch.AggAs[types.FilterAggregate](bucket.Aggregations, "hierarchy")
		if errE != nil {
			return errE
		}
		info := out[key]
		if info == nil {
			info = &relFacetInfo{
				Docs: 0, ValuelessDocs: 0,
				ClaimTypes:   relClaimTypeSet{Ref: false, Has: false, None: false, Unknown: false},
				HasHierarchy: false, Matched: 0,
			}
			out[key] = info
			*order = append(*order, key)
		}
		info.Docs += bucketDocs.DocCount
		info.ValuelessDocs += valuelessDocs.DocCount
		info.HasHierarchy = info.HasHierarchy || hierarchy.DocCount > 0
		if claimTypeBuckets, ok := claimTypesAgg.Buckets.([]types.StringTermsBucket); ok {
			for _, ctb := range claimTypeBuckets {
				switch ctb.Key {
				case internalSearch.ClaimTypeRef:
					info.ClaimTypes.Ref = true
				case internalSearch.ClaimTypeHas:
					info.ClaimTypes.Has = true
				case internalSearch.ClaimTypeNone:
					info.ClaimTypes.None = true
				case internalSearch.ClaimTypeUnknown:
					info.ClaimTypes.Unknown = true
				}
			}
		}
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return errE
			}
			info.Matched += matchedCount
		}
	}
	return nil
}

// valueFacetInfo is the parsed discovery data of one property's amount or time records within one
// context, keyed by (prop, unit) for amounts and prop for times.
type valueFacetInfo struct {
	Docs    int64
	Matched int64
}

// hiddenFacetClauses builds, for each of the given record property fields, a terms query matching the
// hidden facet properties. Used as must_not clauses of a discovery aggregation's scoped filter, they drop
// the records of hidden facets before bucketing, so hidden facets do not take up discovery slots (see
// MaxResultsCount) and do not count towards the per-type totals. The facets stay indexed and their filters
// stay functional; they are only not offered by discovery. With no hidden properties there are no clauses.
func hiddenFacetClauses(hidden map[string]bool, propFields ...string) []types.QueryVariant {
	if len(hidden) == 0 {
		return nil
	}
	// Sorted, so the emitted query is deterministic.
	ids := slices.Sorted(maps.Keys(hidden))
	values := make([]types.FieldValueVariant, len(ids))
	for i, id := range ids {
		values[i] = esdsl.NewFieldValue().String(id)
	}
	clauses := make([]types.QueryVariant, 0, len(propFields))
	for _, field := range propFields {
		clauses = append(clauses, esdsl.NewTermsQuery().AddTermsQuery(field, esdsl.NewTermsQueryField().FieldValues(values...)))
	}
	return clauses
}

// hiddenExceptSelected returns the hidden facet properties minus the given selected ones. Hiding removes
// a property from what a facet offers, never from what the session already selected: a selected hidden
// property keeps its value row and keeps counting towards the facet's count, so the count never
// contradicts the selection the user can see. It is used by the pooled has facets, the only facets whose
// values are properties and so the only ones where a hidden property can be a selected value.
func hiddenExceptSelected(hidden map[string]bool, selected []HasValue) map[string]bool {
	if len(hidden) == 0 || len(selected) == 0 {
		return hidden
	}
	out := make(map[string]bool, len(hidden))
	for id := range hidden {
		out[id] = true
	}
	for _, value := range selected {
		delete(out, value.ID.String())
	}
	return out
}

// selectedHasProps returns the has-properties the session's pooled has filters select: the top-level
// facet's ones (a has filter with an empty property path) when sub is false, and the pooled sub-has
// facets' ones (a has filter with a parent property) when sub is true. The sub set is the union across
// parent properties, because one sub discovery aggregation serves every parent-property bucket: a hidden
// property selected under one parent property therefore keeps counting under every parent property,
// which only ever adds to an already upper-bound count.
func selectedHasProps(filters []Filter, sub bool) []HasValue {
	var out []HasValue
	for i := range filters {
		f := &filters[i]
		if f.Has == nil {
			continue
		}
		if sub != (len(f.Prop) == 1) {
			continue
		}
		out = append(out, f.Has.Props...)
	}
	return out
}

// pooledHasQuery matches, at path, the has records of the properties a pooled has facet offers: the has
// claim type, minus the hidden properties (see hiddenExceptSelected). Both the inactive facet's count and
// the active facet's availability count are built from it, so they agree by construction.
func pooledHasQuery(path string, hidden map[string]bool, selected []HasValue) types.QueryVariant { //nolint:ireturn
	return esdsl.NewBoolQuery().
		Must(claimTypeTerm(path, internalSearch.ClaimTypeHas)).
		MustNot(hiddenFacetClauses(hiddenExceptSelected(hidden, selected), path+".prop")...)
}

// scopedDiscoveryAggregation wraps a discovery aggregation in a "scoped" filter dropping the records of
// hidden facet properties before the terms are computed, so a hidden facet is neither discovered nor
// counted. name is the aggregation name the discovery keeps inside the scope ("props" for a collection's
// per-property discovery, "parents" for a parent-collection's parent-property enumeration) and propFields
// are the record property fields to check. With no hidden properties the filter matches every record.
func scopedDiscoveryAggregation(hidden map[string]bool, name string, inner types.AggregationsVariant, propFields ...string) types.AggregationsVariant { //nolint:ireturn
	return esdsl.NewAggregations().
		Filter(esdsl.NewBoolQuery().MustNot(hiddenFacetClauses(hidden, propFields...)...)).
		AddAggregation(name, inner)
}

// relDiscoveryAggregation builds the rel discovery terms aggregation over path: per-property
// buckets with the full document count, the valueless (has/none/unknown) document count, the claim
// types present, the hierarchy marker (a non-leaf ref record, so the property can carry a direct
// entry), and the value-query match gate.
func relDiscoveryAggregation(path string, match types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	valuelessQuery := esdsl.NewBoolQuery().MustNot(claimTypeTerm(path, internalSearch.ClaimTypeRef))
	// A non-leaf ref record marks a value expanded as an ancestor of a narrower value. isLeaf is indexed
	// omitempty, so non-leaf records have no isLeaf field: match ref AND NOT isLeaf==true.
	hierarchyQuery := esdsl.NewBoolQuery().
		Must(claimTypeTerm(path, internalSearch.ClaimTypeRef)).
		MustNot(esdsl.NewTermQuery(path+".isLeaf", esdsl.NewFieldValue().Bool(true)))
	agg := esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field(path+".prop").Size(MaxResultsCount).
			Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation())).
		AddAggregation("valueless", esdsl.NewAggregations().
			Filter(valuelessQuery).
			AddAggregation("docs", esdsl.NewAggregations().
				ReverseNested(esdsl.NewReverseNestedAggregation()))).
		AddAggregation("claimTypes", esdsl.NewAggregations().
			Terms(esdsl.NewTermsAggregation().Field(path+".claimType").Size(4))). //nolint:mnd
		AddAggregation("hierarchy", esdsl.NewAggregations().
			Filter(hierarchyQuery))
	if match != nil {
		agg = agg.AddAggregation("matched", matchedDocsAggregation(match))
	}
	return agg
}

// amountDiscoveryAggregation builds the amount discovery multi-terms aggregation over path:
// per-(prop, unit) buckets with document counts and the value-query match gate. Units are document
// IDs, so valid units can never be the string "__missing__".
func amountDiscoveryAggregation(path string, match types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	agg := esdsl.NewAggregations().
		MultiTerms(esdsl.NewMultiTermsAggregation().Terms(
			esdsl.NewMultiTermLookup().Field(path+".prop"),
			esdsl.NewMultiTermLookup().Field(path+".unit").Missing(esdsl.NewMissing().String("__missing__")),
		).Size(MaxResultsCount).Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation()))
	if match != nil {
		agg = agg.AddAggregation("matched", matchedDocsAggregation(match))
	}
	return agg
}

// timeDiscoveryAggregation builds the time discovery terms aggregation over path: per-property
// buckets with document counts and the value-query match gate.
func timeDiscoveryAggregation(path string, match types.QueryVariant) types.AggregationsVariant { //nolint:ireturn
	agg := esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field(path+".prop").Size(MaxResultsCount).
			Order(esdsl.NewAggregateOrder().Map(map[string]sortorder.SortOrder{"docs": sortorder.Desc}))).
		AddAggregation("docs", esdsl.NewAggregations().
			ReverseNested(esdsl.NewReverseNestedAggregation()))
	if match != nil {
		agg = agg.AddAggregation("matched", matchedDocsAggregation(match))
	}
	return agg
}

// filteredDiscoveryAggregation wraps a discovery aggregation in nested -> filter(match) -> props so
// the terms enumerate only the facets with a record matching the value query. It surfaces facets
// that match the query but rank beyond the unfiltered discovery's Size cap. The unfiltered pass
// still provides the full, value-query-independent counts for the facets it covers; a facet found
// only here (beyond the cap) takes its count and type from this pass, so its count is the matching
// document count (equal to the full count when the facet was reached through its property name,
// and a lower bound when reached through a value name). The filter also drops the records of hidden
// facet properties, so this pass cannot surface a facet the unfiltered pass hides.
func filteredDiscoveryAggregation(path string, match types.QueryVariant, inner types.AggregationsVariant, hidden map[string]bool) types.AggregationsVariant { //nolint:ireturn
	return esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path(path)).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewBoolQuery().
				Filter(match).
				MustNot(hiddenFacetClauses(hidden, path+".prop")...)).
			AddAggregation("props", inner))
}

// relSpecialPresenceQuery builds the rel-record query over path matching the claim-type special values
// a value query requested: none, unknown and has property are rel records of that claim type; direct is
// a non-leaf ref record (isLeaf false), the marker that a value has narrower values and so can carry a
// direct entry. It drives the beyond-cap special discovery pass, the analogue of the text-match
// beyond-cap passes, letting a property whose only match is one of these specials surface even when it
// ranks beyond the unfiltered discovery's Size cap by document count. path is the rel collection (the
// top-level rel path or a sub rel path). It returns nil when no such special was requested. Missing has
// no rel-record signal (it is an absence relative to the universe) and is near-universal, so it is
// deliberately left scoped to the in-cap pass.
func relSpecialPresenceQuery(path string, specials requestedSpecials) types.QueryVariant { //nolint:ireturn
	var shoulds []types.QueryVariant
	if specials.None {
		shoulds = append(shoulds, claimTypeTerm(path, internalSearch.ClaimTypeNone))
	}
	if specials.Unknown {
		shoulds = append(shoulds, claimTypeTerm(path, internalSearch.ClaimTypeUnknown))
	}
	if specials.HasProperty {
		shoulds = append(shoulds, claimTypeTerm(path, internalSearch.ClaimTypeHas))
	}
	if specials.Direct {
		// A non-leaf ref record (a value expanded as an ancestor of a narrower value) is the marker of a
		// property with hierarchy, the only kind that can carry a direct entry. isLeaf is indexed
		// omitempty, so non-leaf records have no isLeaf field: match ref records that are not leaves as
		// ref AND NOT isLeaf==true rather than isLeaf==false.
		shoulds = append(shoulds, esdsl.NewBoolQuery().
			Must(claimTypeTerm(path, internalSearch.ClaimTypeRef)).
			MustNot(esdsl.NewTermQuery(path+".isLeaf", esdsl.NewFieldValue().Bool(true))))
	}
	if len(shoulds) == 0 {
		return nil
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// parentDiscoveryBuckets builds the per-parent-property terms aggregation used by the sub-level
// discovery: parent-property buckets, each holding the unfiltered sub-collection discovery
// (subRel/subAmount/subTime, with the pooled sub-has count under subRel) and, when a value query is
// active, the parent-name match gate and the filtered sub passes (subRelMatched/subAmountMatched/
// subTimeMatched) that surface sub facets beyond the sub cap. It is used both under the unfiltered
// parent enumeration (sub:<parent>) and under the filtered parent enumeration (sub:<parent>:beyondParents),
// so a parent property beyond the parent cap gets the same sub facets as one within it.
func parentDiscoveryBuckets( //nolint:ireturn
	parent, subRel, subAmount, subTime string,
	subRelMatch, subAmountMatch, subTimeMatch, subRelSpecialMatch types.QueryVariant,
	valueQuery string, enabledLanguages []string, hidden map[string]bool, selectedSubHas []HasValue,
) types.AggregationsVariant {
	buckets := esdsl.NewAggregations().
		Terms(esdsl.NewTermsAggregation().Field(parentPath(parent)+".prop").Size(MaxResultsCount)).
		AddAggregation("subRel", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(subRel)).
			AddAggregation("scoped", scopedDiscoveryAggregation(hidden, "props", relDiscoveryAggregation(subRel, subRelMatch), subRel+".prop")).
			// The pooled sub-has facet's count for this parent property in this parent collection:
			// distinct documents with any has sub-claim, of a property the facet offers, under a parent
			// claim of this property. The nested(subRel) context scopes to sub-claims of the
			// parent-property claims; reverse_nested counts the distinct root documents. Summed across
			// parent collections by the parser.
			AddAggregation("pooledHasDocs", esdsl.NewAggregations().
				Filter(pooledHasQuery(subRel, hidden, selectedSubHas)).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation())))).
		AddAggregation("subAmount", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(subAmount)).
			AddAggregation("scoped", scopedDiscoveryAggregation(hidden, "props", amountDiscoveryAggregation(subAmount, subAmountMatch), subAmount+".prop"))).
		AddAggregation("subTime", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(subTime)).
			AddAggregation("scoped", scopedDiscoveryAggregation(hidden, "props", timeDiscoveryAggregation(subTime, subTimeMatch), subTime+".prop")))
	if valueQuery != "" {
		buckets = buckets.
			AddAggregation("parentMatched", esdsl.NewAggregations().
				Filter(propLabelMatchQuery(
					[]string{parentPath(parent) + ".propNaming"}, []string{parentPath(parent) + ".propDisplay"}, valueQuery, enabledLanguages))).
			AddAggregation("subRelMatched", filteredDiscoveryAggregation(subRel, subRelMatch, relDiscoveryAggregation(subRel, nil), hidden)).
			AddAggregation("subAmountMatched", filteredDiscoveryAggregation(subAmount, subAmountMatch, amountDiscoveryAggregation(subAmount, nil), hidden)).
			AddAggregation("subTimeMatched", filteredDiscoveryAggregation(subTime, subTimeMatch, timeDiscoveryAggregation(subTime, nil), hidden))
		// A matched claim-type special surfaces sub properties that carry it but rank beyond the sub cap,
		// the analogue of subRelMatched. Folded by mergeMatchedSubFacets like the text sub pass.
		if subRelSpecialMatch != nil {
			buckets = buckets.
				AddAggregation("subRelSpecialMatched", filteredDiscoveryAggregation(subRel, subRelSpecialMatch, relDiscoveryAggregation(subRel, nil), hidden))
		}
	}
	return buckets
}

// parseAmountFacetBuckets parses an amount discovery multi-terms aggregation into per-(prop, unit)
// info, merged into out.
func parseAmountFacetBuckets(buckets []types.MultiTermsBucket, valueQueryActive bool, out map[[2]string]*valueFacetInfo, order *[][2]string) errors.E {
	for _, bucket := range buckets {
		if len(bucket.Key) < 2 { //nolint:mnd
			return errors.New("unexpected key length for amount bucket")
		}
		propKey, ok := bucket.Key[0].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket prop")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[0])
			return errE
		}
		unitKey, ok := bucket.Key[1].(string)
		if !ok {
			errE := errors.New("unexpected key type for amount bucket unit")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key[1])
			return errE
		}
		if unitKey == "__missing__" {
			unitKey = ""
		}
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		key := [2]string{propKey, unitKey}
		info := out[key]
		if info == nil {
			info = &valueFacetInfo{Docs: 0, Matched: 0}
			out[key] = info
			*order = append(*order, key)
		}
		info.Docs += bucketDocs.DocCount
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return errE
			}
			info.Matched += matchedCount
		}
	}
	return nil
}

// parseTimeFacetBuckets parses a time discovery terms aggregation into per-property info, merged
// into out.
func parseTimeFacetBuckets(buckets []types.StringTermsBucket, valueQueryActive bool, out map[string]*valueFacetInfo, order *[]string) errors.E {
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for time facet bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return errE
		}
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		info := out[key]
		if info == nil {
			info = &valueFacetInfo{Docs: 0, Matched: 0}
			out[key] = info
			*order = append(*order, key)
		}
		info.Docs += bucketDocs.DocCount
		if valueQueryActive {
			matchedCount, errE := matchedDocCount(bucket.Aggregations)
			if errE != nil {
				return errE
			}
			info.Matched += matchedCount
		}
	}
	return nil
}

// mergeMatchedRelFacets folds a filtered rel discovery pass (nested -> filter -> props terms under
// name) into set: a facet already present from the unfiltered pass keeps its full value-query-
// independent info; a facet present only here (matched but beyond the unfiltered cap) is added with
// its Matched forced positive so it renders, and marked in RelBeyond so it does not count toward the
// total.
func mergeMatchedRelFacets(aggs map[string]types.Aggregate, name string, set *facetSet) errors.E { //nolint:dupl
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "props")
	if errE != nil {
		return errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name)
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return errE
	}
	tmp := map[string]*relFacetInfo{}
	var tmpOrder []string
	errE = parseRelFacetBuckets(buckets, false, tmp, &tmpOrder)
	if errE != nil {
		return errE
	}
	for _, prop := range tmpOrder {
		if _, present := set.Rel[prop]; present {
			continue
		}
		info := tmp[prop]
		info.Matched = info.Docs
		set.Rel[prop] = info
		set.RelOrder = append(set.RelOrder, prop)
		set.RelBeyond[prop] = true
	}
	return nil
}

// mergeMatchedAmountFacets folds a filtered amount discovery pass into set, adding (prop, unit)
// facets present only there, mirroring mergeMatchedRelFacets.
func mergeMatchedAmountFacets(aggs map[string]types.Aggregate, name string, set *facetSet) errors.E {
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return errE
	}
	terms, errE := internalSearch.AggAs[types.MultiTermsAggregate](filter.Aggregations, "props")
	if errE != nil {
		return errE
	}
	buckets, ok := terms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name)
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return errE
	}
	tmp := map[[2]string]*valueFacetInfo{}
	var tmpOrder [][2]string
	errE = parseAmountFacetBuckets(buckets, false, tmp, &tmpOrder)
	if errE != nil {
		return errE
	}
	for _, key := range tmpOrder {
		if _, present := set.Amount[key]; present {
			continue
		}
		info := tmp[key]
		info.Matched = info.Docs
		set.Amount[key] = info
		set.AmountOrder = append(set.AmountOrder, key)
		set.AmountBeyond[key] = true
	}
	return nil
}

// mergeMatchedTimeFacets folds a filtered time discovery pass into set, adding prop facets present
// only there, mirroring mergeMatchedRelFacets.
func mergeMatchedTimeFacets(aggs map[string]types.Aggregate, name string, set *facetSet) errors.E { //nolint:dupl
	nested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, name)
	if errE != nil {
		return errE
	}
	filter, errE := internalSearch.AggAs[types.FilterAggregate](nested.Aggregations, "filter")
	if errE != nil {
		return errE
	}
	terms, errE := internalSearch.AggAs[types.StringTermsAggregate](filter.Aggregations, "props")
	if errE != nil {
		return errE
	}
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for " + name)
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return errE
	}
	tmp := map[string]*valueFacetInfo{}
	var tmpOrder []string
	errE = parseTimeFacetBuckets(buckets, false, tmp, &tmpOrder)
	if errE != nil {
		return errE
	}
	for _, prop := range tmpOrder {
		if _, present := set.Time[prop]; present {
			continue
		}
		info := tmp[prop]
		info.Matched = info.Docs
		set.Time[prop] = info
		set.TimeOrder = append(set.TimeOrder, prop)
		set.TimeBeyond[prop] = true
	}
	return nil
}

// parseParentBucket folds one parent-property bucket's unfiltered sub discovery (subRel/subAmount/
// subTime, the pooled sub-has count, and the parent-name match count) into set, tracking terms
// saturation in saturated. It is the shared body of the unfiltered parent enumeration and the
// filtered (beyond-cap parent) enumeration; the filtered sub passes are merged separately.
func parseParentBucket(parentBucket types.StringTermsBucket, set *facetSet, valueQueryActive bool, saturated *bool) errors.E {
	subRelNested, errE := internalSearch.AggAs[types.NestedAggregate](parentBucket.Aggregations, "subRel")
	if errE != nil {
		return errE
	}
	subRelScoped, errE := internalSearch.AggAs[types.FilterAggregate](subRelNested.Aggregations, "scoped")
	if errE != nil {
		return errE
	}
	subRelTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](subRelScoped.Aggregations, "props")
	if errE != nil {
		return errE
	}
	*saturated = *saturated || termsSaturated(subRelTerms.SumOtherDocCount)
	if buckets, ok := subRelTerms.Buckets.([]types.StringTermsBucket); ok {
		errE = parseRelFacetBuckets(buckets, valueQueryActive, set.Rel, &set.RelOrder)
		if errE != nil {
			return errE
		}
	}
	pooledHasFilter, errE := internalSearch.AggAs[types.FilterAggregate](subRelNested.Aggregations, "pooledHasDocs")
	if errE != nil {
		return errE
	}
	pooledHasDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](pooledHasFilter.Aggregations, "docs")
	if errE != nil {
		return errE
	}
	set.PooledHasDocs += pooledHasDocs.DocCount
	subAmountNested, errE := internalSearch.AggAs[types.NestedAggregate](parentBucket.Aggregations, "subAmount")
	if errE != nil {
		return errE
	}
	subAmountScoped, errE := internalSearch.AggAs[types.FilterAggregate](subAmountNested.Aggregations, "scoped")
	if errE != nil {
		return errE
	}
	subAmountTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](subAmountScoped.Aggregations, "props")
	if errE != nil {
		return errE
	}
	*saturated = *saturated || termsSaturated(subAmountTerms.SumOtherDocCount)
	if buckets, ok := subAmountTerms.Buckets.([]types.MultiTermsBucket); ok {
		errE = parseAmountFacetBuckets(buckets, valueQueryActive, set.Amount, &set.AmountOrder)
		if errE != nil {
			return errE
		}
	}
	subTimeNested, errE := internalSearch.AggAs[types.NestedAggregate](parentBucket.Aggregations, "subTime")
	if errE != nil {
		return errE
	}
	subTimeScoped, errE := internalSearch.AggAs[types.FilterAggregate](subTimeNested.Aggregations, "scoped")
	if errE != nil {
		return errE
	}
	subTimeTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](subTimeScoped.Aggregations, "props")
	if errE != nil {
		return errE
	}
	*saturated = *saturated || termsSaturated(subTimeTerms.SumOtherDocCount)
	if buckets, ok := subTimeTerms.Buckets.([]types.StringTermsBucket); ok {
		errE = parseTimeFacetBuckets(buckets, valueQueryActive, set.Time, &set.TimeOrder)
		if errE != nil {
			return errE
		}
	}
	if valueQueryActive {
		parentMatched, errE := internalSearch.AggAs[types.FilterAggregate](parentBucket.Aggregations, "parentMatched")
		if errE != nil {
			return errE
		}
		set.ParentMatched += parentMatched.DocCount
	}
	return nil
}

// mergeMatchedSubFacets folds the filtered sub passes (subRelMatched/subAmountMatched/subTimeMatched)
// of one parent-property bucket into set, adding sub facets beyond the unfiltered sub cap and marking
// them beyond. When foldSpecial is true a subRelSpecialMatched pass is present (a claim-type special was
// matched) and is folded too, surfacing special-bearing sub properties beyond the sub cap.
func mergeMatchedSubFacets(parentBucket types.StringTermsBucket, set *facetSet, foldSpecial bool) errors.E {
	errE := mergeMatchedRelFacets(parentBucket.Aggregations, "subRelMatched", set)
	if errE != nil {
		return errE
	}
	errE = mergeMatchedAmountFacets(parentBucket.Aggregations, "subAmountMatched", set)
	if errE != nil {
		return errE
	}
	errE = mergeMatchedTimeFacets(parentBucket.Aggregations, "subTimeMatched", set)
	if errE != nil {
		return errE
	}
	if foldSpecial {
		return mergeMatchedRelFacets(parentBucket.Aggregations, "subRelSpecialMatched", set)
	}
	return nil
}

// markAllBeyond marks every facet in set as beyond, used for a parent property surfaced only by the
// filtered parent enumeration (beyond the parent cap): all of its sub facets render without counting
// toward the value-query-independent total.
func markAllBeyond(set *facetSet) {
	for _, prop := range set.RelOrder {
		set.RelBeyond[prop] = true
	}
	for _, key := range set.AmountOrder {
		set.AmountBeyond[key] = true
	}
	for _, prop := range set.TimeOrder {
		set.TimeBeyond[prop] = true
	}
}

// facetSet assembles the discovered facets of one level (top-level, or one parent property's sub
// level) from its rel, amount, and time discovery data. propsPrefix is prepended to every facet's
// Props (the parent property for a sub level).
type facetSet struct {
	RelOrder    []string
	Rel         map[string]*relFacetInfo
	AmountOrder [][2]string
	Amount      map[[2]string]*valueFacetInfo
	TimeOrder   []string
	Time        map[string]*valueFacetInfo
	// ParentMatched is the sub level's parent-name match count: when positive, every facet of the
	// level passes the value-query gate (the facet was reached through the parent property's name).
	ParentMatched int64
	// PooledHasDocs is the pooled has facet's distinct-document count (unused at the top level, which
	// reads it from the "pooledHas" root aggregation instead). For a sub level it is the number of
	// documents with any sub-has claim under the parent property, summed across the parent
	// collections: a document with such a sub-has under the same parent property in two parent
	// collections is the documented cross-collection overcount.
	PooledHasDocs int64
	// RelBeyond, AmountBeyond and TimeBeyond mark facets surfaced only by the filtered value-query
	// discovery pass, beyond the unfiltered discovery's Size cap (see the matched-discovery merge in
	// FiltersGet). They are emitted like any other facet, but do not count toward totalFacets, which
	// stays value-query-independent (the available-filters total is already a lower bound, marked
	// "+", when the cap is saturated).
	RelBeyond    map[string]bool
	AmountBeyond map[[2]string]bool
	TimeBeyond   map[string]bool
}

func newFacetSet() *facetSet {
	return &facetSet{
		RelOrder:      nil,
		Rel:           map[string]*relFacetInfo{},
		AmountOrder:   nil,
		Amount:        map[[2]string]*valueFacetInfo{},
		TimeOrder:     nil,
		Time:          map[string]*valueFacetInfo{},
		ParentMatched: 0,
		PooledHasDocs: 0,
		RelBeyond:     map[string]bool{},
		AmountBeyond:  map[[2]string]bool{},
		TimeBeyond:    map[string]bool{},
	}
}

// Results converts the level's discovery data into FilterResult entries plus the level's pooled has
// facet member properties. A property's facets: a "ref" (value-list) facet when it has ref records,
// or when it has valueless records and no valued facet at all (a specials-only facet); an "amount"
// facet per (prop, unit); a "time" facet per prop. A property whose only facetable statements are
// has claims joins the pooled has facet instead. Counts are reachable-through counts: the rel
// facet's count is the property's full rel document count (values plus specials, one context, so
// exact); amount and time facets add the property's valueless document count (documents reachable
// only through the shared specials). That addition is a cross-collection sum: a document with both
// a valued and a valueless statement for the property counts twice. It affects only these headline
// counts, never the facet-detail identity counts, which are root filters. Both overcount triggers
// are described in the package comment.
//
// The pooled has facet's count is not computed here (the distinct-document count is not in the
// per-property buckets): the caller reads it from a document-level aggregation. Results only reports
// which properties are its members (pooledHasProps) and includes it in totalFacets when it exists.
//
// The value query gates only which facets are returned, never their counts or the total: hidden
// entries stay out of the returned slice, and totalFacets counts every discovered facet found by
// the unfiltered pass (including the pooled has facet when it has any member) but not the ones the
// filtered pass surfaced beyond the cap (marked in the *Beyond sets), so it stays value-query-
// independent (stable as the box is typed in).
func (s *facetSet) Results(propsPrefix []string, valueQueryActive bool, specials requestedSpecials) ([]FilterResult, []string, int) {
	out := []FilterResult{}
	var pooledHasProps []string
	pooledHasExists := false
	totalFacets := 0
	passes := func(matched int64) bool {
		if !valueQueryActive {
			return true
		}
		return matched > 0 || s.ParentMatched > 0
	}
	// keep folds the text-match gate together with the special-label gate: a facet is returned when its
	// records match the typed text, or when the query matched a special value label that applies to it.
	keep := func(matched int64, ct relClaimTypeSet, isRef, hasHierarchy bool) bool {
		return passes(matched) || specialFacetPasses(specials, ct, isRef, hasHierarchy)
	}
	amountProps := map[string]bool{}
	for _, key := range s.AmountOrder {
		amountProps[key[0]] = true
	}
	timeProps := map[string]bool{}
	for _, key := range s.TimeOrder {
		timeProps[key] = true
	}
	for _, prop := range s.RelOrder {
		info := s.Rel[prop]
		hasValuedElsewhere := amountProps[prop] || timeProps[prop]
		// A facet surfaced only by the filtered value-query pass (beyond the cap) is emitted but does
		// not count toward the value-query-independent total.
		countsToTotal := !s.RelBeyond[prop]
		switch {
		case info.ClaimTypes.Ref:
			if countsToTotal {
				totalFacets++
			}
			if keep(info.Matched, info.ClaimTypes, true, info.HasHierarchy) {
				out = append(out, FilterResult{
					Props:    append(slices.Clone(propsPrefix), prop),
					Type:     "ref",
					Unit:     "",
					FilterID: "",
					Count:    info.Docs,
				})
			}
		case info.ClaimTypes.PooledHas() && !hasValuedElsewhere:
			if countsToTotal {
				pooledHasExists = true
			}
			// The pooled has facet is a property list with no missing/none/unknown/direct rows, so of the
			// special labels only "has property" surfaces it.
			if passes(info.Matched) || specials.HasProperty {
				pooledHasProps = append(pooledHasProps, prop)
			}
		case !hasValuedElsewhere:
			// Valueless statements (none, unknown, or has beside them) with no valued facet anywhere:
			// a specials-only value-list facet. It carries no ref values, so the direct special never
			// surfaces it (isRef is false).
			if countsToTotal {
				totalFacets++
			}
			if keep(info.Matched, info.ClaimTypes, false, info.HasHierarchy) {
				out = append(out, FilterResult{
					Props:    append(slices.Clone(propsPrefix), prop),
					Type:     "ref",
					Unit:     "",
					FilterID: "",
					Count:    info.Docs,
				})
			}
		}
		// Valueless statements beside an amount or time facet surface through that facet's specials;
		// their reachable documents are added to those facets' counts below.
	}
	if pooledHasExists {
		totalFacets++
	}
	for _, key := range s.AmountOrder {
		info := s.Amount[key]
		count := info.Docs
		var ct relClaimTypeSet
		if relInfo, ok := s.Rel[key[0]]; ok {
			ct = relInfo.ClaimTypes
			if !relInfo.ClaimTypes.Ref {
				count += relInfo.ValuelessDocs
			}
		}
		if !s.AmountBeyond[key] {
			totalFacets++
		}
		if keep(info.Matched, ct, false, false) {
			out = append(out, FilterResult{
				Props:    append(slices.Clone(propsPrefix), key[0]),
				Type:     "amount",
				Unit:     key[1],
				FilterID: "",
				Count:    count,
			})
		}
	}
	for _, prop := range s.TimeOrder {
		info := s.Time[prop]
		count := info.Docs
		var ct relClaimTypeSet
		if relInfo, ok := s.Rel[prop]; ok {
			ct = relInfo.ClaimTypes
			if !relInfo.ClaimTypes.Ref {
				count += relInfo.ValuelessDocs
			}
		}
		if !s.TimeBeyond[prop] {
			totalFacets++
		}
		if keep(info.Matched, ct, false, false) {
			out = append(out, FilterResult{
				Props:    append(slices.Clone(propsPrefix), prop),
				Type:     "time",
				Unit:     "",
				FilterID: "",
				Count:    count,
			})
		}
	}
	return out, pooledHasProps, totalFacets
}

// specialFacetPasses reports whether a facet must be kept because the value query matched a special
// value label that applies to it. Missing applies to every facet (nearly every property is missing on
// some documents, and the exact per-facet count then drops facets with none). None, unknown and has
// property apply when the facet's property has that rel claim type. Direct applies to a value-list
// (ref) facet that has hierarchy (hasHierarchy), the only kind that can carry a per-value direct entry;
// gating on hierarchy keeps the direct search from flooding the pane with every ref facet. It is
// additive to the text-match gate and is inert when no special was matched.
func specialFacetPasses(specials requestedSpecials, ct relClaimTypeSet, isRef, hasHierarchy bool) bool {
	switch {
	case specials.Missing:
		return true
	case specials.None && ct.None:
		return true
	case specials.Unknown && ct.Unknown:
		return true
	case specials.HasProperty && ct.Has:
		return true
	case specials.Direct && isRef && hasHierarchy:
		return true
	default:
		return false
	}
}

// FiltersGet retrieves all available filters for the current search: one entry per discovered
// facet (value-list, amount per unit, time), the pooled has facet (top-level and per parent
// property), and one entry per active filter with its availability count.
//
// hiddenFacetProperties drops, inside every discovery aggregation, the records whose property path
// contains one of the given property IDs, top-level and sub-facets alike, so hidden facets are not
// discovered, take up none of the discovery slots (see MaxResultsCount), and do not count towards the
// per-type totals; active filters are returned regardless, so a filter already in the session stays
// visible.
func FiltersGet( //nolint:maintidx
	ctx context.Context, getSearchService func() *esSearch.Search, searchSession *Session, languages *Languages,
	valueQuery string, hiddenFacetProperties map[string]bool, extraFilters ...types.QueryVariant,
) ([]FilterResult, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	enabledLanguages := languages.enabled()

	// The access filter goes on the top-level query; ES scopes every non-global aggregation to
	// the documents it matches, so facet counts never include documents the caller cannot
	// access. The per-active-filter aggregations below are global (they must escape the
	// filtered query to exclude their own filter), so they re-apply extraFilters themselves.
	query := searchSession.ToQuery(enabledLanguages, extraFilters...)

	searchService := getSearchService().Size(0).Query(query)

	valueQueryActive := valueQuery != ""

	// The value query also matches the special value labels (missing, none, unknown, has property,
	// direct) so those facet rows can be found by name. A matched special keeps the facets it applies
	// to (in facetSet.Results) even when the typed text matches none of their real records.
	specials := matchedSpecials(valueQuery, languages.special())

	// The rel-record query for a matched claim-type special (none/unknown/has/direct), or nil. It drives
	// a beyond-cap discovery pass surfacing special-bearing properties that rank beyond the cap.
	relSpecialMatch := relSpecialPresenceQuery(relPath, specials)

	// When a value query is active, each discovery aggregation decides which facets to return, but never their
	// counts: a facet is kept when at least one of its records matches the query, either through one of the
	// facet's values or through the facet's own property name (for a sub facet, also its parent property's
	// name, matched at parent level). This mirrors the matching the per-facet value endpoints use, so a facet
	// that appears here always has at least one value to show. The reported counts and the totals stay outside
	// the value query, so they do not change as the box is typed in.
	var relMatch, amountMatch, timeMatch types.QueryVariant
	if valueQueryActive {
		relMatch = labelMatchQuery(
			[]string{relPath + ".toNaming"}, []string{relPath + ".toDisplay"},
			[]string{relPath + ".propNaming"}, []string{relPath + ".propDisplay"}, valueQuery, enabledLanguages)
		amountMatch = amountTimeMatchQuery(
			[]string{amountPath + ".propNaming"}, []string{amountPath + ".propDisplay"},
			[]string{amountPath + ".fromDisplay", amountPath + ".toDisplay"}, valueQuery, enabledLanguages)
		timeMatch = amountTimeMatchQuery(
			[]string{timePath + ".propNaming"}, []string{timePath + ".propDisplay"},
			[]string{timePath + ".fromDisplay", timePath + ".toDisplay"}, valueQuery, enabledLanguages)
	}

	// Top-level discovery: one aggregation per facetable collection, each scoped to the records of
	// properties which are not hidden.
	searchService = searchService.
		AddAggregation("rel", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(relPath)).
			AddAggregation("scoped", scopedDiscoveryAggregation(hiddenFacetProperties, "props", relDiscoveryAggregation(relPath, relMatch), relPath+".prop"))).
		AddAggregation("amount", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(amountPath)).
			AddAggregation("scoped", scopedDiscoveryAggregation(
				hiddenFacetProperties, "props", amountDiscoveryAggregation(amountPath, amountMatch), amountPath+".prop"))).
		AddAggregation("time", esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(timePath)).
			AddAggregation("scoped", scopedDiscoveryAggregation(hiddenFacetProperties, "props", timeDiscoveryAggregation(timePath, timeMatch), timePath+".prop"))).
		// The pooled has facet's count: distinct documents with any has claim of a property the facet
		// offers. It is a document-level root filter (the same query the active pooled-has availability
		// count uses), so the inactive heading and the active count agree by construction.
		AddAggregation("pooledHas", esdsl.NewAggregations().
			Filter(esdsl.NewNestedQuery(
				pooledHasQuery(relPath, hiddenFacetProperties, selectedHasProps(searchSession.Filters, false)),
			).Path(relPath)))

	// When a value query is active, a second filtered discovery pass per top-level collection
	// enumerates only the facets with a matching record, surfacing facets that match the query but
	// rank beyond the unfiltered discovery's Size cap by document count. The unfiltered pass above
	// still provides the full, value-query-independent counts for the facets it covers.
	if valueQueryActive {
		searchService = searchService.
			AddAggregation("relMatched", filteredDiscoveryAggregation(relPath, relMatch, relDiscoveryAggregation(relPath, nil), hiddenFacetProperties)).
			AddAggregation("amountMatched", filteredDiscoveryAggregation(amountPath, amountMatch, amountDiscoveryAggregation(amountPath, nil), hiddenFacetProperties)).
			AddAggregation("timeMatched", filteredDiscoveryAggregation(timePath, timeMatch, timeDiscoveryAggregation(timePath, nil), hiddenFacetProperties))
		// A matched claim-type special (none/unknown/has/direct) surfaces the rel properties that carry it
		// but rank beyond the cap, the analogue of the text passes above. The inner aggregation is the same
		// rel discovery, so mergeMatchedRelFacets folds it in exactly like the text pass.
		if relSpecialMatch != nil {
			searchService = searchService.
				AddAggregation("relSpecialMatched", filteredDiscoveryAggregation(relPath, relSpecialMatch, relDiscoveryAggregation(relPath, nil), hiddenFacetProperties))
		}
	}

	// Sub-level discovery: per parent collection, parent-property buckets holding the same
	// per-collection discovery aggregations one level down, plus the parent-name match gate.
	selectedSubHas := selectedHasProps(searchSession.Filters, true)
	for _, parent := range parentCollections {
		subRel := subPath(parent, "rel")
		subAmount := subPath(parent, "amount")
		subTime := subPath(parent, "time")
		var subRelMatch, subAmountMatch, subTimeMatch, subRelSpecialMatch types.QueryVariant
		if valueQueryActive {
			subRelMatch = labelMatchQuery(
				[]string{subRel + ".toNaming"}, []string{subRel + ".toDisplay"},
				[]string{subRel + ".propNaming"}, []string{subRel + ".propDisplay"}, valueQuery, enabledLanguages)
			subAmountMatch = amountTimeMatchQuery(
				[]string{subAmount + ".propNaming"}, []string{subAmount + ".propDisplay"},
				[]string{subAmount + ".fromDisplay", subAmount + ".toDisplay"}, valueQuery, enabledLanguages)
			subTimeMatch = amountTimeMatchQuery(
				[]string{subTime + ".propNaming"}, []string{subTime + ".propDisplay"},
				[]string{subTime + ".fromDisplay", subTime + ".toDisplay"}, valueQuery, enabledLanguages)
			subRelSpecialMatch = relSpecialPresenceQuery(subRel, specials)
		}
		// The parent enumeration is scoped on the parent claim's own property, so a hidden parent property
		// takes none of the parent slots and none of its sub facets are discovered.
		searchService = searchService.AddAggregation("sub:"+parent, esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
			AddAggregation("scoped", scopedDiscoveryAggregation(hiddenFacetProperties, "parents", parentDiscoveryBuckets(
				parent, subRel, subAmount, subTime, subRelMatch, subAmountMatch, subTimeMatch, subRelSpecialMatch,
				valueQuery, enabledLanguages, hiddenFacetProperties, selectedSubHas), parentPath(parent)+".prop")))

		// When a value query is active, a filtered parent enumeration surfaces parent properties that
		// match the query but rank beyond the parent-property Size cap by document count. A parent
		// matches when its own name matches or when it has a sub-claim matching the query, so its whole
		// set of sub facets (identical structure to the unfiltered enumeration) becomes reachable. The
		// parser only keeps parent properties from here that the unfiltered enumeration missed.
		if valueQueryActive {
			// A parent matches when its own name matches, or when it has a sub-claim matching the query by
			// text or, for a claim-type special, by presence, so a parent beyond the parent cap whose only
			// match is a special sub-claim still surfaces its sub facets.
			parentShoulds := []types.QueryVariant{
				propLabelMatchQuery(
					[]string{parentPath(parent) + ".propNaming"}, []string{parentPath(parent) + ".propDisplay"}, valueQuery, enabledLanguages),
				esdsl.NewNestedQuery(subRelMatch).Path(subRel),
				esdsl.NewNestedQuery(subAmountMatch).Path(subAmount),
				esdsl.NewNestedQuery(subTimeMatch).Path(subTime),
			}
			if subRelSpecialMatch != nil {
				parentShoulds = append(parentShoulds, esdsl.NewNestedQuery(subRelSpecialMatch).Path(subRel))
			}
			// Hidden parent properties are dropped here too, so this pass cannot surface a parent property
			// the unfiltered enumeration hides.
			parentMatchFilter := esdsl.NewBoolQuery().Should(parentShoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)).
				MustNot(hiddenFacetClauses(hiddenFacetProperties, parentPath(parent)+".prop")...)
			searchService = searchService.AddAggregation("sub:"+parent+":beyondParents", esdsl.NewAggregations().
				Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
				AddAggregation("filter", esdsl.NewAggregations().
					Filter(parentMatchFilter).
					AddAggregation("parents", parentDiscoveryBuckets(
						parent, subRel, subAmount, subTime, subRelMatch, subAmountMatch, subTimeMatch, subRelSpecialMatch,
						valueQuery, enabledLanguages, hiddenFacetProperties, selectedSubHas))))
		}
	}

	// For each active filter, add an aggregation that computes the facet's availability count
	// excluding the facet's own selections. The top-level request query applies every filter, so
	// the exclusion has to escape it: each active aggregation is global and re-applies the
	// excluded session query (with the access extraFilters) itself. This ensures active filters
	// always appear in results with correct counts, unaffected by their own path's selections.
	// For a specials filter the scoped aggregation also carries the detection sub-aggregations
	// resolving which facet type (and units) the path renders as, from the data rather than from
	// sibling filters, so a specials-only selection still renders the property's real facet.
	for i, f := range searchSession.Filters {
		if f.ID == nil {
			// This should not be possible.
			continue
		}
		presence := activeFilterPresenceQuery(&searchSession.Filters[i], hiddenFacetProperties)
		scoped := esdsl.NewAggregations().
			Filter(searchSession.ToQueryExcluding(activeFilterExcludeIDs(searchSession, &searchSession.Filters[i]), enabledLanguages, extraFilters...)).
			AddAggregation("count", esdsl.NewAggregations().Filter(presence))
		if f.Specials != nil {
			for name, agg := range activeSpecialsDetectionAggs(&searchSession.Filters[i]) {
				scoped = scoped.AddAggregation(name, agg)
			}
		}
		activeAgg := esdsl.NewAggregations().
			Global(esdsl.NewGlobalAggregation()).
			AddAggregation("scoped", scoped)
		searchService = searchService.AddAggregation(fmt.Sprintf("active_%d", i), activeAgg)
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	// Parse the top level. saturated records whether any discovery terms aggregation dropped keys
	// past its Size cap (a positive sum_other_doc_count), which makes totalFacets a lower bound.
	topSet := newFacetSet()
	saturated := false
	relNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "rel")
	if errE != nil {
		return nil, nil, errE
	}
	relScoped, errE := internalSearch.AggAs[types.FilterAggregate](relNested.Aggregations, "scoped")
	if errE != nil {
		return nil, nil, errE
	}
	relTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](relScoped.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	saturated = saturated || termsSaturated(relTerms.SumOtherDocCount)
	relBuckets, ok := relTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for rel")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", relTerms.Buckets)
		return nil, nil, errE
	}
	errE = parseRelFacetBuckets(relBuckets, valueQueryActive, topSet.Rel, &topSet.RelOrder)
	if errE != nil {
		return nil, nil, errE
	}
	amountNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "amount")
	if errE != nil {
		return nil, nil, errE
	}
	amountScoped, errE := internalSearch.AggAs[types.FilterAggregate](amountNested.Aggregations, "scoped")
	if errE != nil {
		return nil, nil, errE
	}
	amountTerms, errE := internalSearch.AggAs[types.MultiTermsAggregate](amountScoped.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	saturated = saturated || termsSaturated(amountTerms.SumOtherDocCount)
	amountBuckets, ok := amountTerms.Buckets.([]types.MultiTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for amount")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", amountTerms.Buckets)
		return nil, nil, errE
	}
	errE = parseAmountFacetBuckets(amountBuckets, valueQueryActive, topSet.Amount, &topSet.AmountOrder)
	if errE != nil {
		return nil, nil, errE
	}
	timeNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "time")
	if errE != nil {
		return nil, nil, errE
	}
	timeScoped, errE := internalSearch.AggAs[types.FilterAggregate](timeNested.Aggregations, "scoped")
	if errE != nil {
		return nil, nil, errE
	}
	timeTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](timeScoped.Aggregations, "props")
	if errE != nil {
		return nil, nil, errE
	}
	saturated = saturated || termsSaturated(timeTerms.SumOtherDocCount)
	timeBuckets, ok := timeTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for time")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", timeTerms.Buckets)
		return nil, nil, errE
	}
	errE = parseTimeFacetBuckets(timeBuckets, valueQueryActive, topSet.Time, &topSet.TimeOrder)
	if errE != nil {
		return nil, nil, errE
	}

	// Fold in the filtered value-query passes: facets matching the query but beyond the unfiltered
	// cap by document count are added to the set (marked as beyond so they render without growing
	// the value-query-independent total).
	if valueQueryActive {
		errE = mergeMatchedRelFacets(res.Aggregations, "relMatched", topSet)
		if errE != nil {
			return nil, nil, errE
		}
		errE = mergeMatchedAmountFacets(res.Aggregations, "amountMatched", topSet)
		if errE != nil {
			return nil, nil, errE
		}
		errE = mergeMatchedTimeFacets(res.Aggregations, "timeMatched", topSet)
		if errE != nil {
			return nil, nil, errE
		}
		// The beyond-cap special pass surfaces special-bearing rel properties (a value-list facet, or a
		// specials-only one). It also enriches the claim types of a property already present through its
		// amount or time facet, so that facet passes the special gate when its valueless special record
		// ranked beyond the rel cap. A property beyond both the rel and its amount/time cap renders as a
		// specials-only value-list facet (its amount/time facet is not enumerated here).
		if relSpecialMatch != nil {
			errE = mergeMatchedRelFacets(res.Aggregations, "relSpecialMatched", topSet)
			if errE != nil {
				return nil, nil, errE
			}
		}
	}

	results, pooledHasProps, totalFacets := topSet.Results(nil, valueQueryActive, specials)

	// The pooled top-level has facet: a single filter over the properties whose only facetable
	// statements are has claims. Its count is distinct documents with any has claim (the pooledHas
	// root aggregation). This is an upper bound on documents with a pooled (has-only) property: a
	// document whose has-claims are all for migrated properties (properties that also have another
	// facetable claim type in scope, shown in their own facet) is counted here too. It matches the
	// active pooled-has availability count, which is the same query.
	if len(pooledHasProps) > 0 {
		pooledHas, errE := internalSearch.AggAs[types.FilterAggregate](res.Aggregations, "pooledHas")
		if errE != nil {
			return nil, nil, errE
		}
		results = append(results, FilterResult{
			Props:    nil,
			Type:     "has",
			Unit:     "",
			FilterID: "",
			Count:    pooledHas.DocCount,
		})
	}

	// Parse the sub levels: merge the per-parent-collection parent-property buckets by parent
	// property, then assemble each parent property's facets.
	subSets := map[string]*facetSet{}
	var subOrder []string
	for _, parent := range parentCollections {
		subNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "sub:"+parent)
		if errE != nil {
			return nil, nil, errE
		}
		parentScoped, errE := internalSearch.AggAs[types.FilterAggregate](subNested.Aggregations, "scoped")
		if errE != nil {
			return nil, nil, errE
		}
		parentTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](parentScoped.Aggregations, "parents")
		if errE != nil {
			return nil, nil, errE
		}
		saturated = saturated || termsSaturated(parentTerms.SumOtherDocCount)
		parentBuckets, ok := parentTerms.Buckets.([]types.StringTermsBucket)
		if !ok {
			errE := errors.New("unexpected bucket type for sub parents")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", parentTerms.Buckets)
			return nil, nil, errE
		}
		for _, parentBucket := range parentBuckets {
			parentProp, ok := parentBucket.Key.(string)
			if !ok {
				continue
			}
			set := subSets[parentProp]
			if set == nil {
				set = newFacetSet()
				subSets[parentProp] = set
				subOrder = append(subOrder, parentProp)
			}
			errE = parseParentBucket(parentBucket, set, valueQueryActive, &saturated)
			if errE != nil {
				return nil, nil, errE
			}
		}
	}

	// Fold in the filtered sub-discovery passes across all parent collections, after all unfiltered
	// sub parsing above: a sub facet matching the query but beyond the unfiltered cap within its parent
	// property is added to that parent property's set (marked beyond so it renders without growing the
	// total). Doing it in a second pass keeps a sub facet that is unfiltered in any parent collection
	// from being treated as beyond (and its count from being corrupted by a matched-scoped merge).
	if valueQueryActive {
		for _, parent := range parentCollections {
			subNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "sub:"+parent)
			if errE != nil {
				return nil, nil, errE
			}
			parentScoped, errE := internalSearch.AggAs[types.FilterAggregate](subNested.Aggregations, "scoped")
			if errE != nil {
				return nil, nil, errE
			}
			parentTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](parentScoped.Aggregations, "parents")
			if errE != nil {
				return nil, nil, errE
			}
			parentBuckets, ok := parentTerms.Buckets.([]types.StringTermsBucket)
			if !ok {
				continue
			}
			for _, parentBucket := range parentBuckets {
				parentProp, ok := parentBucket.Key.(string)
				if !ok {
					continue
				}
				set := subSets[parentProp]
				if set == nil {
					continue
				}
				errE = mergeMatchedSubFacets(parentBucket, set, relSpecialMatch != nil)
				if errE != nil {
					return nil, nil, errE
				}
			}
		}

		// Fold in the filtered parent enumeration: parent properties matching the query but beyond the
		// parent-property cap have no bucket in the unfiltered enumeration above, so their whole sub
		// facet set is assembled here (all marked beyond, so they render without growing the total). A
		// parent already present from the unfiltered enumeration is skipped; the beyond enumeration is
		// still Size-capped on matching parents (refine the query to reach past it).
		for _, parent := range parentCollections {
			beyondNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "sub:"+parent+":beyondParents")
			if errE != nil {
				return nil, nil, errE
			}
			beyondFilter, errE := internalSearch.AggAs[types.FilterAggregate](beyondNested.Aggregations, "filter")
			if errE != nil {
				return nil, nil, errE
			}
			parentTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](beyondFilter.Aggregations, "parents")
			if errE != nil {
				return nil, nil, errE
			}
			saturated = saturated || termsSaturated(parentTerms.SumOtherDocCount)
			parentBuckets, ok := parentTerms.Buckets.([]types.StringTermsBucket)
			if !ok {
				continue
			}
			for _, parentBucket := range parentBuckets {
				parentProp, ok := parentBucket.Key.(string)
				if !ok {
					continue
				}
				if _, present := subSets[parentProp]; present {
					// Within the parent cap: already assembled by the unfiltered enumeration above.
					continue
				}
				set := newFacetSet()
				errE = parseParentBucket(parentBucket, set, valueQueryActive, &saturated)
				if errE != nil {
					return nil, nil, errE
				}
				errE = mergeMatchedSubFacets(parentBucket, set, relSpecialMatch != nil)
				if errE != nil {
					return nil, nil, errE
				}
				markAllBeyond(set)
				subSets[parentProp] = set
				subOrder = append(subOrder, parentProp)
			}
		}
	}

	for _, parentProp := range subOrder {
		set := subSets[parentProp]
		subResults, subPooledProps, subTotalFacets := set.Results([]string{parentProp}, valueQueryActive, specials)
		results = append(results, subResults...)
		totalFacets += subTotalFacets
		// The pooled sub-has facet of this parent property. Its count is distinct documents with any
		// has sub-claim under the parent property (set.pooledHasDocs), the same upper-bound semantics
		// as the top-level pooled has facet, summed across parent collections.
		if len(subPooledProps) > 0 {
			results = append(results, FilterResult{
				Props:    []string{parentProp},
				Type:     "has",
				Unit:     "",
				FilterID: "",
				Count:    set.PooledHasDocs,
			})
		}
	}

	// The available-filters total is the number of discovered facets (before active-filter
	// entries), counted outside the value query so it stays stable as the box is typed in. The
	// per-level terms aggregations are capped at MaxResultsCount buckets: when any of them dropped
	// keys past that cap (saturated), a facet type had more distinct facets than were seen, so the
	// total is a lower bound and is marked with a trailing "+" (the search results total convention).
	total := strconv.Itoa(totalFacets)
	if saturated {
		total += "+"
	}

	// Parse per-active-filter aggregation results and append them with FilterID set.
	// Main results (without FilterID) remain as inactive filter options. specialsValuedPaths
	// records the paths whose active specials filter resolved to a valued facet, so the
	// specials-only value-list facet discovery synthesized for them (within the narrowed scope
	// the path has no valued records) can be dropped below.
	specialsValuedPaths := map[string]bool{}
	for i := range searchSession.Filters {
		f := &searchSession.Filters[i]
		if f.ID == nil {
			// This should not be possible.
			continue
		}

		props := make([]string, 0, len(f.Prop))
		for _, p := range f.Prop {
			props = append(props, p.String())
		}

		aggName := fmt.Sprintf("active_%d", i)
		activeGlobal, errE := internalSearch.AggAs[types.GlobalAggregate](res.Aggregations, aggName)
		if errE != nil {
			return nil, nil, errE
		}
		scoped, errE := internalSearch.AggAs[types.FilterAggregate](activeGlobal.Aggregations, "scoped")
		if errE != nil {
			return nil, nil, errE
		}
		countFilter, errE := internalSearch.AggAs[types.FilterAggregate](scoped.Aggregations, "count")
		if errE != nil {
			return nil, nil, errE
		}

		if f.Specials != nil {
			entries, errE := activeSpecialsResults(scoped.Aggregations, f, props, countFilter.DocCount)
			if errE != nil {
				return nil, nil, errE
			}
			for _, entry := range entries {
				if entry.Type != "ref" {
					specialsValuedPaths[strings.Join(props, "/")] = true
				}
			}
			results = append(results, entries...)
			continue
		}

		result := FilterResult{Props: props, Type: activeFilterType(f), Unit: "", FilterID: f.ID.String(), Count: countFilter.DocCount}
		if len(props) == 0 {
			result.Props = nil
		}
		if f.Amount != nil && f.Amount.Unit != nil {
			result.Unit = f.Amount.Unit.String()
		}
		results = append(results, result)
	}

	// An active specials filter that resolved to a valued facet supersedes the specials-only
	// value-list facet synthesized for its path by discovery: with no valued (ref) statements for
	// the path anywhere (otherwise the specials filter would have resolved to a value-list facet),
	// an inactive value-list entry for it can only be the synthesized one.
	if len(specialsValuedPaths) > 0 {
		results = slices.DeleteFunc(results, func(r FilterResult) bool {
			return r.FilterID == "" && r.Type == "ref" && specialsValuedPaths[strings.Join(r.Props, "/")]
		})
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

	return results, map[string]any{
		"total": total,
	}, nil
}

// termsSaturated reports whether a terms (or multi-terms) aggregation dropped distinct keys past its
// Size cap: a positive sum_other_doc_count means buckets beyond the cap exist, so a count of the
// returned buckets is a lower bound on the distinct keys.
func termsSaturated(sumOtherDocCount *int64) bool {
	return sumOtherDocCount != nil && *sumOtherDocCount > 0
}

// activeFilterType resolves the facet type an active valued filter renders as. Specials filters do
// not go through here: their type comes from the detection sub-aggregations (activeSpecialsResults),
// resolved from the data rather than from sibling filters.
func activeFilterType(f *Filter) string {
	switch {
	case f.Ref != nil:
		return "ref"
	case f.Amount != nil:
		return "amount"
	case f.Time != nil:
		return "time"
	case f.Has != nil:
		return "has"
	}
	return "ref"
}

// activeFilterExcludeIDs returns the session-query exclusions for an active filter's availability
// count: the facet's own filter and the path's specials filter (FacetExcludeIDs), and for a
// specials filter additionally the path's valued filters, so the specials entry's count is computed
// under the same rest-of-search scope as the valued entries of its path.
func activeFilterExcludeIDs(session *Session, f *Filter) []identifier.Identifier {
	out := session.FacetExcludeIDs(f.Prop, f.ID)
	if f.Specials == nil {
		return out
	}
	for i := range session.Filters {
		sibling := &session.Filters[i]
		if sibling.ID == nil || !SamePropPath(sibling.Prop, f.Prop) {
			continue
		}
		if !idInList(*sibling.ID, out) {
			out = append(out, *sibling.ID)
		}
	}
	return out
}

// relValuelessQuery matches documents with a valueless rel statement (has, none, or unknown) for
// prop: a rel record of any claimType other than ref.
func relValuelessQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return esdsl.NewNestedQuery(esdsl.NewBoolQuery().
		Must(propTerm(relPath, prop)).
		MustNot(claimTypeTerm(relPath, internalSearch.ClaimTypeRef)),
	).Path(relPath)
}

// subRelValuedQuery matches documents with a rel sub record for (parentProp, prop) under any parent
// collection, restricted to the valued claim type (valued true, the ref claim type) or to the
// valueless claim types (valued false, has/none/unknown).
func subRelValuedQuery(parentProp, prop identifier.Identifier, valued bool) types.QueryVariant { //nolint:ireturn
	arms := make([]types.QueryVariant, 0, len(parentCollections))
	for _, parent := range parentCollections {
		subRel := subPath(parent, "rel")
		inner := esdsl.NewBoolQuery().Must(propTerm(subRel, prop))
		if valued {
			inner = inner.Must(claimTypeTerm(subRel, internalSearch.ClaimTypeRef))
		} else {
			inner = inner.MustNot(claimTypeTerm(subRel, internalSearch.ClaimTypeRef))
		}
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			propTerm(parentPath(parent), parentProp),
			esdsl.NewNestedQuery(inner).Path(subRel),
		)).Path(parentPath(parent)))
	}
	return oneOrShould(arms)
}

// subTimePresenceQuery matches documents with a time sub record for (parentProp, prop) under any
// parent collection.
func subTimePresenceQuery(parentProp, prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	arms := make([]types.QueryVariant, 0, len(parentCollections))
	for _, parent := range parentCollections {
		subTime := subPath(parent, "time")
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			propTerm(parentPath(parent), parentProp),
			esdsl.NewNestedQuery(propTerm(subTime, prop)).Path(subTime),
		)).Path(parentPath(parent)))
	}
	return oneOrShould(arms)
}

// activeSpecialsDetectionAggs builds, for an active specials filter, the sub-aggregations
// detecting which facet type (and amount units) the filter's path renders as: the document counts
// of valued (ref) rel statements, time statements, and valueless rel statements for the path, plus
// the amount unit buckets. They run under the active filter's excluded scope, so a specials-only
// selection does not hide the path's real facet type.
func activeSpecialsDetectionAggs(f *Filter) map[string]types.AggregationsVariant {
	aggs := map[string]types.AggregationsVariant{}
	if len(f.Prop) == 1 {
		prop := f.Prop[0]
		aggs["valued"] = esdsl.NewAggregations().Filter(
			esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				propTerm(relPath, prop), claimTypeTerm(relPath, internalSearch.ClaimTypeRef),
			)).Path(relPath))
		aggs["timeDocs"] = esdsl.NewAggregations().Filter(esdsl.NewNestedQuery(propTerm(timePath, prop)).Path(timePath))
		aggs["valueless"] = esdsl.NewAggregations().Filter(relValuelessQuery(prop))
		aggs["units"] = esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(amountPath)).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(propTerm(amountPath, prop)).
				AddAggregation("units", esdsl.NewAggregations().
					Terms(esdsl.NewTermsAggregation().Field(amountPath+".unit").Size(MaxResultsCount).
						Missing(esdsl.NewMissing().String("__missing__"))).
					AddAggregation("docs", esdsl.NewAggregations().
						ReverseNested(esdsl.NewReverseNestedAggregation()))))
		return aggs
	}
	parentProp, prop := f.Prop[0], f.Prop[1]
	aggs["valued"] = esdsl.NewAggregations().Filter(subRelValuedQuery(parentProp, prop, true))
	aggs["timeDocs"] = esdsl.NewAggregations().Filter(subTimePresenceQuery(parentProp, prop))
	aggs["valueless"] = esdsl.NewAggregations().Filter(subRelValuedQuery(parentProp, prop, false))
	for _, parent := range parentCollections {
		subAmount := subPath(parent, "amount")
		aggs["units:"+parent] = esdsl.NewAggregations().
			Nested(esdsl.NewNestedAggregation().Path(parentPath(parent))).
			AddAggregation("filter", esdsl.NewAggregations().
				Filter(propTerm(parentPath(parent), parentProp)).
				AddAggregation("sub", esdsl.NewAggregations().
					Nested(esdsl.NewNestedAggregation().Path(subAmount)).
					AddAggregation("propFilter", esdsl.NewAggregations().
						Filter(propTerm(subAmount, prop)).
						AddAggregation("units", esdsl.NewAggregations().
							Terms(esdsl.NewTermsAggregation().Field(subAmount+".unit").Size(MaxResultsCount).
								Missing(esdsl.NewMissing().String("__missing__"))).
							AddAggregation("docs", esdsl.NewAggregations().
								ReverseNested(esdsl.NewReverseNestedAggregation()))))))
	}
	return aggs
}

// addUnitBuckets folds one unit terms aggregation's buckets (each with a "docs" reverse_nested
// count) into the units map, recording first-seen order in unitOrder.
func addUnitBuckets(terms *types.StringTermsAggregate, units map[string]int64, unitOrder *[]string) errors.E {
	buckets, ok := terms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for units")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", terms.Buckets)
		return errE
	}
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			errE := errors.New("unexpected key type for units bucket")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", bucket.Key)
			return errE
		}
		bucketDocs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return errE
		}
		if _, ok := units[key]; !ok {
			*unitOrder = append(*unitOrder, key)
		}
		units[key] += bucketDocs.DocCount
	}
	return nil
}

// activeSpecialsResults resolves an active specials filter's facet entries from its detection
// sub-aggregations: the path renders as its real facet type (a value-list facet when valued (ref)
// statements exist, else one amount facet per unit, else a time facet, else a specials-only
// value-list facet), each entry carrying the filter's ID and a reachable-through count. Amount and
// time entry counts add the valueless document count to the valued one, the same cross-collection
// sum (and the same overcount trigger) as the discovery counts in facetSet.Results.
func activeSpecialsResults(aggs map[string]types.Aggregate, f *Filter, props []string, presenceCount int64) ([]FilterResult, errors.E) {
	valued, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "valued")
	if errE != nil {
		return nil, errE
	}
	timeDocs, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "timeDocs")
	if errE != nil {
		return nil, errE
	}
	valueless, errE := internalSearch.AggAs[types.FilterAggregate](aggs, "valueless")
	if errE != nil {
		return nil, errE
	}

	units := map[string]int64{}
	var unitOrder []string
	if len(f.Prop) == 1 {
		unitsNested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, "units")
		if errE != nil {
			return nil, errE
		}
		unitsFilter, errE := internalSearch.AggAs[types.FilterAggregate](unitsNested.Aggregations, "filter")
		if errE != nil {
			return nil, errE
		}
		unitsTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](unitsFilter.Aggregations, "units")
		if errE != nil {
			return nil, errE
		}
		errE = addUnitBuckets(unitsTerms, units, &unitOrder)
		if errE != nil {
			return nil, errE
		}
	} else {
		for _, parent := range parentCollections {
			unitsNested, errE := internalSearch.AggAs[types.NestedAggregate](aggs, "units:"+parent)
			if errE != nil {
				return nil, errE
			}
			unitsFilter, errE := internalSearch.AggAs[types.FilterAggregate](unitsNested.Aggregations, "filter")
			if errE != nil {
				return nil, errE
			}
			subNested, errE := internalSearch.AggAs[types.NestedAggregate](unitsFilter.Aggregations, "sub")
			if errE != nil {
				return nil, errE
			}
			propFilter, errE := internalSearch.AggAs[types.FilterAggregate](subNested.Aggregations, "propFilter")
			if errE != nil {
				return nil, errE
			}
			unitsTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](propFilter.Aggregations, "units")
			if errE != nil {
				return nil, errE
			}
			errE = addUnitBuckets(unitsTerms, units, &unitOrder)
			if errE != nil {
				return nil, errE
			}
		}
	}

	filterID := f.ID.String()
	switch {
	case valued.DocCount > 0:
		return []FilterResult{{Props: props, Type: "ref", Unit: "", FilterID: filterID, Count: presenceCount}}, nil
	case len(unitOrder) > 0:
		out := make([]FilterResult, 0, len(unitOrder))
		for _, unit := range unitOrder {
			unitValue := unit
			if unitValue == "__missing__" {
				unitValue = ""
			}
			out = append(out, FilterResult{Props: props, Type: "amount", Unit: unitValue, FilterID: filterID, Count: units[unit] + valueless.DocCount})
		}
		return out, nil
	case timeDocs.DocCount > 0:
		return []FilterResult{{Props: props, Type: "time", Unit: "", FilterID: filterID, Count: timeDocs.DocCount + valueless.DocCount}}, nil
	default:
		return []FilterResult{{Props: props, Type: "ref", Unit: "", FilterID: filterID, Count: presenceCount}}, nil
	}
}

// activeFilterPresenceQuery builds the availability count query of an active filter: the documents
// reachable through the filter's facet, whatever their values. Amount and time filters count
// documents with records in their collection for the property or with a valueless rel statement
// for it (those documents are reachable through the facet's specials); ref filters count documents
// with any rel record; a specials filter counts documents with any facetable statement for its
// path; the pooled has filter counts documents with any has statement of a property it offers (the
// hidden ones it does not offer are dropped, except those the filter itself selects, exactly as in the
// inactive facet's count); sub filters count documents with a matching sub record under any parent claim
// for the parent property.
func activeFilterPresenceQuery(f *Filter, hidden map[string]bool) types.QueryVariant { //nolint:ireturn
	sub := len(f.Prop) == 2 //nolint:mnd
	switch {
	case f.Has != nil && len(f.Prop) == 0:
		return esdsl.NewNestedQuery(pooledHasQuery(relPath, hidden, f.Has.Props)).Path(relPath)
	case f.Has != nil:
		parentProp := f.Prop[0]
		var arms []types.QueryVariant
		for _, parent := range parentCollections {
			subRel := subPath(parent, "rel")
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				propTerm(parentPath(parent), parentProp),
				esdsl.NewNestedQuery(pooledHasQuery(subRel, hidden, f.Has.Props)).Path(subRel),
			)).Path(parentPath(parent)))
		}
		return oneOrShould(arms)
	case f.Ref != nil && !sub:
		return esdsl.NewNestedQuery(propTerm(relPath, f.Prop[0])).Path(relPath)
	case f.Amount != nil && !sub:
		return oneOrShould([]types.QueryVariant{
			esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(amountPath, f.Prop[0]), unitTerm(amountPath, f.Amount.Unit))).Path(amountPath),
			relValuelessQuery(f.Prop[0]),
		})
	case f.Time != nil && !sub:
		return oneOrShould([]types.QueryVariant{
			esdsl.NewNestedQuery(propTerm(timePath, f.Prop[0])).Path(timePath),
			relValuelessQuery(f.Prop[0]),
		})
	case f.Specials != nil && !sub:
		return oneOrShould(topFacetablePresenceClauses(f.Prop[0]))
	}
	// Sub filters: a matching sub record under any parent claim for the parent property.
	parentProp, prop := f.Prop[0], f.Prop[1]
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		var inner types.QueryVariant
		switch {
		case f.Ref != nil:
			subRel := subPath(parent, "rel")
			inner = esdsl.NewNestedQuery(propTerm(subRel, prop)).Path(subRel)
		case f.Amount != nil:
			subAmount := subPath(parent, "amount")
			subRel := subPath(parent, "rel")
			inner = oneOrShould([]types.QueryVariant{
				esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(subAmount, prop), unitTerm(subAmount, f.Amount.Unit))).Path(subAmount),
				esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(subRel, prop)).MustNot(claimTypeTerm(subRel, internalSearch.ClaimTypeRef))).Path(subRel),
			})
		case f.Time != nil:
			subTime := subPath(parent, "time")
			subRel := subPath(parent, "rel")
			inner = oneOrShould([]types.QueryVariant{
				esdsl.NewNestedQuery(propTerm(subTime, prop)).Path(subTime),
				esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(subRel, prop)).MustNot(claimTypeTerm(subRel, internalSearch.ClaimTypeRef))).Path(subRel),
			})
		default:
			inner = facetableSubPresence(parent, prop)
		}
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			propTerm(parentPath(parent), parentProp),
			inner,
		)).Path(parentPath(parent)))
	}
	return oneOrShould(arms)
}
