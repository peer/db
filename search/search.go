package search

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/fieldvaluefactormodifier"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/functionboostmode"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/searchtype"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/totalhitsrelation"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	// MaxResultsCount is the maximum number of search results that can be returned.
	MaxResultsCount = 1000

	// topDisplayBoost is the boost factor applied to the document's top-level
	// rendered display label (per-language "display.<lang>") relative to the
	// main text field. Matches against the user-visible label of the document
	// itself rank higher than incidental matches inside aggregated text.
	topDisplayBoost = float32(3.0)

	// textDisMaxTieBreaker is the tie_breaker for the per-language text.<lang>
	// dis_max wrapper. dis_max scores a doc as max(per-clause score) +
	// tie_breaker * sum(other matching scores); a small non-zero value rewards
	// docs that match in multiple languages slightly, without letting language
	// tagging redundancy dominate the ranking.
	textDisMaxTieBreaker = 0.1

	// phraseBoost is the multiplicative bonus applied to the proximity-phrase
	// clauses on top of the base text/display recall score. The phrase clauses
	// only fire on docs already admitted by the recall layer (the bool's must),
	// so this boost rewards docs where the query terms appear close together
	// without affecting which docs are returned.
	phraseBoost = float32(2.0)

	// phraseSlop is the position tolerance for the match_phrase proximity
	// clauses: terms may be at most this many positions apart (and may appear
	// in any order, at a slop cost) and still count as a phrase match. Tuned
	// to allow short stop-word gaps and minor reordering while still requiring
	// physical adjacency in the doc.
	phraseSlop = 5

	// 40000 is the maximum precision threshold ES supports, so we use it to get the most accurate approximation.
	// For now we didn't notice any performance issues at data scale PeerDB is currently being used with, but
	// in the future we might want to make this configurable.
	maxPrecisionThreshold = 40000

	// scoreBoostMax is the target boost ratio between a corpus-p99 document and an
	// empty one under the counts.score ranking boost: a p99 document's _score is
	// multiplied by roughly scoreBoostMax times the multiplier an empty document
	// gets. It is the single tuning parameter of the boost.
	scoreBoostMax = 10.0

	// log2pOffset is the additive constant of the ElasticSearch log2p
	// field_value_factor modifier: the boost is log10(log2pOffset + factor*counts.score).
	log2pOffset = 2.0

	// scorePercentile is the corpus percentile of counts.score that ScoreFactor
	// normalizes to the scoreBoostMax boost ceiling.
	scorePercentile = 99.0
)

// distinctValuesTotal returns the number of distinct values represented by a terms
// aggregation that was capped at MaxResultsCount buckets, paired with a cardinality
// aggregation over the same field. When fewer than MaxResultsCount buckets came back
// the terms aggregation was not truncated, so the bucket count is the exact number
// of distinct values and we use it directly. Only when the aggregation is saturated
// (it returned the full MaxResultsCount buckets and may have omitted further values)
// do we fall back to the cardinality estimate for how many values exist beyond the
// cap. Cardinality is approximate and can over, as well as under, count (see
// https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-cardinality-aggregation.html#_counts_are_approximate),
// so trusting it when the exact count is already known would wrongly report values as
// "not shown". The max guards the saturated case against the estimate undercounting
// the buckets we already hold.
func distinctValuesTotal(bucketCount int, cardinality int64) int64 {
	if bucketCount < MaxResultsCount {
		return int64(bucketCount)
	}
	return max(int64(bucketCount), cardinality)
}

// ViewType represents the type of search view.
type ViewType string

// ViewType values.
const (
	ViewFeed  ViewType = "feed"
	ViewTable ViewType = "table"
)

// ToValue represents a target value in a reference filter.
type ToValue struct {
	ID identifier.Identifier `json:"id"`
}

// HasValue represents a selected property value in a has filter.
type HasValue struct {
	ID identifier.Identifier `json:"id"`
}

// RefFilter contains values for a reference filter.
//
// Direct holds values selected through their "direct" option: a value in Direct matches only
// documents for which the value is most-specific (it references the value but none of its narrower
// values). It parallels To (which matches the value and all its narrower values) and is OR-ed with
// To and Missing.
type RefFilter struct {
	To      []ToValue `json:"to,omitempty"`
	Direct  []ToValue `json:"direct,omitempty"`
	Missing bool      `json:"missing,omitempty"`
}

// ToQuery converts the RefFilter to an ElasticSearch query for the given property.
func (f *RefFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	missingQuery := esdsl.NewBoolQuery().MustNot(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
		).Path("claims.ref"),
	)

	// Missing only.
	if f.Missing && len(f.To) == 0 && len(f.Direct) == 0 {
		return missingQuery
	}

	// Build value queries (OR across all To and Direct values).
	shoulds := make([]types.QueryVariant, 0, len(f.To)+len(f.Direct)+1)
	for _, to := range f.To {
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(to.ID.String())),
			),
		).Path("claims.ref"))
	}

	// A "direct" value additionally requires isLeaf=true, so it matches only documents for which
	// the value is most-specific (none of its narrower values present).
	for _, to := range f.Direct {
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(to.ID.String())),
				esdsl.NewTermQuery("claims.ref.isLeaf", esdsl.NewFieldValue().Bool(true)),
			),
		).Path("claims.ref"))
	}

	// Values + missing: OR them together.
	if f.Missing {
		shoulds = append(shoulds, missingQuery)
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// Validate validates the RefFilter.
func (f *RefFilter) Validate() errors.E {
	if len(f.To) == 0 && len(f.Direct) == 0 && !f.Missing {
		return errors.New("to, direct, or missing has to be set")
	}
	return nil
}

// AmountFilter contains values for an amount filter.
//
// Exists matches documents which have the property (with the filter's unit), with any
// value. It is the only selection which matches documents whose claims have no known
// endpoint values at all (an interval with both endpoints none).
type AmountFilter struct {
	Unit    *identifier.Identifier `json:"unit,omitempty"`
	Gte     *float64               `json:"gte,omitempty"`
	Lte     *float64               `json:"lte,omitempty"`
	Missing bool                   `json:"missing,omitempty"`
	Exists  bool                   `json:"exists,omitempty"`
}

// ToQuery converts the AmountFilter to an ElasticSearch query for the given property.
func (f *AmountFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if f.Missing {
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
			).Path("claims.amount"),
		)
	}

	if f.Exists {
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
				amountUnitFilter(f.Unit),
			),
		).Path("claims.amount")
	}

	r := esdsl.NewNumberRangeQuery("claims.amount.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	must := []types.QueryVariant{
		esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
		r,
	}
	if f.Unit != nil {
		must = append(must, esdsl.NewTermQuery("claims.amount.unit", esdsl.NewFieldValue().String(f.Unit.String())))
	}
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(must...),
	).Path("claims.amount")
}

// Validate validates the AmountFilter.
func (f *AmountFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Missing && !f.Exists {
		return errors.New("gte and lte, missing, or exists has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Exists {
		return errors.New("gte/lte and exists cannot be both set")
	}
	if f.Missing && f.Exists {
		return errors.New("missing and exists cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// ToSubAmountQuery converts the AmountFilter to an ElasticSearch query on
// claims.subAmount for a sub-amount filter with parentProp and prop.
//
// parentToRestrictions, when non-empty, restricts the sub-claim match to
// entries whose claims.subAmount.parentTo is one of the listed values.
func (f *AmountFilter) ToSubAmountQuery(parentProp, prop identifier.Identifier, parentToRestrictions []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	addRestriction := func(must []types.QueryVariant) []types.QueryVariant {
		if len(parentToRestrictions) == 0 {
			return must
		}
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subAmount.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		return append(must, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	if f.Missing {
		missingMust := addRestriction([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subAmount.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subAmount.prop", esdsl.NewFieldValue().String(prop.String())),
		})
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewBoolQuery().Must(missingMust...),
			).Path("claims.subAmount"),
		)
	}

	if f.Exists {
		existsMust := addRestriction([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subAmount.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subAmount.prop", esdsl.NewFieldValue().String(prop.String())),
			subAmountUnitFilter(f.Unit),
		})
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(existsMust...),
		).Path("claims.subAmount")
	}

	r := esdsl.NewNumberRangeQuery("claims.subAmount.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	must := []types.QueryVariant{
		esdsl.NewTermQuery("claims.subAmount.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subAmount.prop", esdsl.NewFieldValue().String(prop.String())),
		r,
	}
	if f.Unit != nil {
		must = append(must, esdsl.NewTermQuery("claims.subAmount.unit", esdsl.NewFieldValue().String(f.Unit.String())))
	}
	must = addRestriction(must)
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(must...),
	).Path("claims.subAmount")
}

// TimeFilter contains values for a time filter.
//
// Gte and Lte are in seconds since Unix epoch.
//
// Exists matches documents which have the property, with any value. It is the only
// selection which matches documents whose claims have no known endpoint values at all
// (an interval with both endpoints none).
type TimeFilter struct {
	Gte     *float64 `json:"gte,omitempty"`
	Lte     *float64 `json:"lte,omitempty"`
	Missing bool     `json:"missing,omitempty"`
	Exists  bool     `json:"exists,omitempty"`
}

// ToQuery converts the TimeFilter to an ElasticSearch query for the given property.
func (f *TimeFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if f.Missing {
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String())),
			).Path("claims.time"),
		)
	}

	if f.Exists {
		return esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String())),
		).Path("claims.time")
	}

	r := esdsl.NewNumberRangeQuery("claims.time.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String())),
			r,
		),
	).Path("claims.time")
}

// Validate validates the TimeFilter.
func (f *TimeFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Missing && !f.Exists {
		return errors.New("gte and lte, missing, or exists has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Exists {
		return errors.New("gte/lte and exists cannot be both set")
	}
	if f.Missing && f.Exists {
		return errors.New("missing and exists cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// ToSubTimeQuery converts the TimeFilter to an ElasticSearch query on
// claims.subTime for a sub-time filter with parentProp and prop.
//
// parentToRestrictions, when non-empty, restricts the sub-claim match to
// entries whose claims.subTime.parentTo is one of the listed values.
func (f *TimeFilter) ToSubTimeQuery(parentProp, prop identifier.Identifier, parentToRestrictions []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	addRestriction := func(must []types.QueryVariant) []types.QueryVariant {
		if len(parentToRestrictions) == 0 {
			return must
		}
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subTime.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		return append(must, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	if f.Missing {
		missingMust := addRestriction([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subTime.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subTime.prop", esdsl.NewFieldValue().String(prop.String())),
		})
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewBoolQuery().Must(missingMust...),
			).Path("claims.subTime"),
		)
	}

	if f.Exists {
		existsMust := addRestriction([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subTime.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subTime.prop", esdsl.NewFieldValue().String(prop.String())),
		})
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(existsMust...),
		).Path("claims.subTime")
	}

	r := esdsl.NewNumberRangeQuery("claims.subTime.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	must := addRestriction([]types.QueryVariant{
		esdsl.NewTermQuery("claims.subTime.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
		esdsl.NewTermQuery("claims.subTime.prop", esdsl.NewFieldValue().String(prop.String())),
		r,
	})
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(must...),
	).Path("claims.subTime")
}

// HasFilter contains values for a has filter.
//
// Props lists the has-claim property IDs the filter matches against. The
// values are OR'd together: a document matches the filter when any of the
// listed properties is present as a simple has claim (top-level form) or as
// a sub-has under a parent property (sub-has form).
type HasFilter struct {
	Props []HasValue `json:"props,omitempty"`
}

// ToQuery converts the HasFilter to an ElasticSearch query against the
// top-level claims.has nested field.
func (f *HasFilter) ToQuery() types.QueryVariant { //nolint:ireturn
	// Build value queries (OR across all selected props). claims.has only
	// contains simple has claims; has claims with their own sub-claims are
	// flattened into the matching Sub* records on the parent document, so
	// the filter does not need to exclude them here.
	shoulds := make([]types.QueryVariant, 0, len(f.Props))
	for _, p := range f.Props {
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.has.prop", esdsl.NewFieldValue().String(p.ID.String())),
		).Path("claims.has"))
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// ToSubHasQuery converts the HasFilter to an ElasticSearch query on
// claims.subHas for a sub-has filter under the given parentProp.
//
// parentToRestrictions, when non-empty, restricts the sub-claim match to
// entries whose claims.subHas.parentTo is one of the listed values.
func (f *HasFilter) ToSubHasQuery(parentProp identifier.Identifier, parentToRestrictions []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	addRestriction := func(must []types.QueryVariant) []types.QueryVariant {
		if len(parentToRestrictions) == 0 {
			return must
		}
		shoulds := make([]types.QueryVariant, 0, len(parentToRestrictions))
		for _, pto := range parentToRestrictions {
			shoulds = append(shoulds, esdsl.NewTermQuery("claims.subHas.parentTo", esdsl.NewFieldValue().String(pto.String())))
		}
		return append(must, esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)))
	}

	shoulds := make([]types.QueryVariant, 0, len(f.Props))
	for _, p := range f.Props {
		must := addRestriction([]types.QueryVariant{
			esdsl.NewTermQuery("claims.subHas.parentProp", esdsl.NewFieldValue().String(parentProp.String())),
			esdsl.NewTermQuery("claims.subHas.prop", esdsl.NewFieldValue().String(p.ID.String())),
		})
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(must...),
		).Path("claims.subHas"))
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// Validate validates the HasFilter.
func (f *HasFilter) Validate() errors.E {
	if len(f.Props) == 0 {
		return errors.New("props has to be set")
	}
	return nil
}

// Filter represents a single active search filter.
//
// Exactly one of Ref, Amount, Time, or Has must be set. Sub-claim filters use
// a two-element Prop: Prop[0] is the parent claim's property, Prop[1] is the
// sub-claim's property. The Has filter takes a single Prop element in its
// sub-claim form (the parent claim's property); HasFilter.Props selects the
// sub-claim properties to match.
type Filter struct {
	ID     *identifier.Identifier  `json:"id,omitempty"`
	Base   []string                `json:"base,omitempty"`
	Prop   []identifier.Identifier `json:"prop"`
	Ref    *RefFilter              `json:"ref,omitempty"`
	Amount *AmountFilter           `json:"amount,omitempty"`
	Time   *TimeFilter             `json:"time,omitempty"`
	Has    *HasFilter              `json:"has,omitempty"`
}

// Validate validates the Filter to ensure it has a valid configuration.
func (f Filter) Validate(withoutSession bool) errors.E {
	if !withoutSession {
		if len(f.Base) < 2 { //nolint:mnd
			errE := errors.New("base must have at least two elements")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}

		expectedID := identifier.From(f.Base...)
		if f.ID == nil || *f.ID != expectedID {
			errE := errors.New("invalid filter ID")
			errors.Details(errE)["got"] = f.ID.String()
			errors.Details(errE)["expected"] = expectedID.String()
			return errE
		}
	} else {
		if len(f.Base) > 0 {
			errE := errors.New("base must be empty")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}
		if f.ID != nil {
			errE := errors.New("id must be empty")
			errors.Details(errE)["id"] = f.ID.String()
			return errE
		}
	}

	nonEmpty := 0
	if f.Ref != nil {
		nonEmpty++
	}
	if f.Amount != nil {
		nonEmpty++
	}
	if f.Time != nil {
		nonEmpty++
	}
	if f.Has != nil {
		nonEmpty++
	}
	if nonEmpty != 1 {
		return errors.New("exactly one of ref, amount, time, or has must be set")
	}

	// Ref, Amount, and Time filters take one prop at top level (the claim's
	// property) and two props in their sub-claim form (parentProp + prop).
	// The Has filter takes no prop at top level and a single prop (parentProp)
	// in its sub-has form; HasFilter.Props selects which sub-claim properties
	// to match.
	switch {
	case f.Has != nil:
		if len(f.Prop) > 1 {
			errE := errors.New("prop must have zero or one elements for has filter")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	default:
		if len(f.Prop) != 1 && len(f.Prop) != 2 {
			errE := errors.New("prop must have one or two elements")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	}

	if f.Ref != nil {
		return f.Ref.Validate()
	}
	if f.Amount != nil {
		return f.Amount.Validate()
	}
	if f.Time != nil {
		return f.Time.Validate()
	}
	return f.Has.Validate()
}

// GetFilterByID finds a filter by ID in the session's filters.
func (s *Session) GetFilterByID(id identifier.Identifier) (*Filter, errors.E) {
	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == id {
			return &s.Filters[i], nil
		}
	}
	return nil, errors.WithDetails(ErrNotFound, "filter", id)
}

// Built-in sort column types. Filter columns reuse the filter type strings ("ref", "amount", "time")
// and carry Prop. A built-in "time" column (the document's earliest time) is distinguished from a
// "time" filter column by the absence of Prop.
const (
	SortScore = "score"
	SortTime  = "time"
	SortLabel = "label"
)

// SortKey is one column in the effective sort order.
//
// Type is a built-in column ("score", "time", "label"), which never carries Prop, or a filter column
// ("ref", "amount", "time") which always carries Prop (a single property ID; sub-claim columns are not
// supported yet). Unit applies only to amount filter columns. Descending sorts high-to-low (default is
// ascending). Group, valid only on ref columns, groups results by that column's value; group keys must
// form a leading contiguous run of the sort order. Expand, valid only on grouped columns, renders each
// group value as a full result card instead of a one-line heading; it is purely presentational.
type SortKey struct {
	Type       string   `json:"type"`
	Prop       []string `json:"prop,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Descending bool     `json:"descending,omitempty"`
	Group      bool     `json:"group,omitempty"`
	Expand     bool     `json:"expand,omitempty"`
}

// isFilter reports whether the key is a filter column (carries a property) rather than a built-in column.
func (k SortKey) isFilter() bool {
	return len(k.Prop) > 0
}

// SessionData represents the data of the search session.
//
// When Reverse is set, the session is scoped to documents which have a ref claim
// (for any property) whose "to" target equals Reverse.
type SessionData struct {
	View     ViewType `json:"view,omitempty"`
	Query    string   `json:"query,omitempty"`
	Language string   `json:"language,omitempty"`
	Filters  []Filter `json:"filters,omitempty"`
	// Prefilters are filters of the same shape as Filters, but their queries go into the bool filter
	// clause instead of the scoring must clause, so they constrain the result set without contributing
	// to _score (no should-clause boosting from multi-value matches).
	Prefilters []Filter               `json:"prefilters,omitempty"`
	Reverse    *identifier.Identifier `json:"reverse,omitempty"`
	// ReverseExpand, valid only when Reverse is set, is purely presentational: in the print view it renders
	// the referenced target as its full result card instead of a one-line "results referencing" heading.
	ReverseExpand bool `json:"reverseExpand,omitempty"`
	// Sort is the effective sort order: an ordered list of columns. Empty means the default order
	// (relevance, then time, then display label). A leading run of group=true ref columns groups results.
	Sort []SortKey `json:"sort,omitempty"`
}

// validateFilters validates each filter in filters and records its ID in seen to detect
// duplicates. field is the error detail key identifying the set ("filter" or "prefilter").
func validateFilters(filters []Filter, field string, withoutSession bool, seen map[identifier.Identifier]bool) errors.E {
	for i, f := range filters {
		errE := f.Validate(withoutSession)
		if errE != nil {
			errors.Details(errE)[field] = i
			return errE
		}
		if !withoutSession {
			// We checked that f.ID is not nil in f.Validate().
			if seen[*f.ID] {
				errE := errors.New("duplicate filter ID")
				errors.Details(errE)["id"] = f.ID.String()
				errors.Details(errE)[field] = i
				return errE
			}
			seen[*f.ID] = true
		}
	}
	return nil
}

// Validate validates the session data.
//
// Validate uses ctx with Site.
func (s *SessionData) Validate(ctx context.Context, withoutSession bool) errors.E {
	// Filters and Prefilters share the seen-ID set, so an ID cannot be reused across the two sets.
	seenFilters := map[identifier.Identifier]bool{}
	errE := validateFilters(s.Filters, "filter", withoutSession, seenFilters)
	if errE != nil {
		return errE
	}
	errE = validateFilters(s.Prefilters, "prefilter", withoutSession, seenFilters)
	if errE != nil {
		return errE
	}

	if !withoutSession {
		if s.View == "" {
			s.View = ViewFeed
		}
		if s.View != ViewFeed && s.View != ViewTable {
			errE := errors.New("invalid view")
			errors.Details(errE)["view"] = s.View
			return errE
		}
	}

	errE = validateSort(s.Sort)
	if errE != nil {
		return errE
	}

	if s.ReverseExpand && s.Reverse == nil {
		return errors.New("reverseExpand is set without reverse")
	}

	st := waf.MustGetSite[*internalSite.Site](ctx)
	resolved, errE := internalSearch.ResolveLanguage(s.Language, st.LanguagePriority, st.DefaultLanguage)
	if errE != nil {
		return errE
	}
	s.Language = resolved

	return nil
}

// validateSort validates the sort order: every key targets a known column, filter columns carry a single
// property, only ref columns may be grouped, and group keys form a leading contiguous run.
func validateSort(sort []SortKey) errors.E {
	seenNonGroup := false
	for i := range sort {
		k := sort[i]
		if k.isFilter() {
			switch k.Type {
			case "ref", "amount", "time":
			default:
				errE := errors.New("invalid filter sort column type")
				errors.Details(errE)["type"] = k.Type
				errors.Details(errE)["sort"] = i
				return errE
			}
			if len(k.Prop) != 1 {
				errE := errors.New("filter sort column must have exactly one property")
				errors.Details(errE)["sort"] = i
				return errE
			}
			if k.Unit != "" && k.Type != "amount" {
				errE := errors.New("only amount sort columns may have a unit")
				errors.Details(errE)["sort"] = i
				return errE
			}
		} else {
			switch k.Type {
			case SortScore, SortTime, SortLabel:
			default:
				errE := errors.New("invalid built-in sort column type")
				errors.Details(errE)["type"] = k.Type
				errors.Details(errE)["sort"] = i
				return errE
			}
			if k.Unit != "" {
				errE := errors.New("built-in sort column may not have a unit")
				errors.Details(errE)["sort"] = i
				return errE
			}
		}
		if k.Group {
			if k.Type != "ref" || !k.isFilter() {
				errE := errors.New("only ref columns may be grouped")
				errors.Details(errE)["sort"] = i
				return errE
			}
			if seenNonGroup {
				errE := errors.New("group columns must be a leading run of the sort order")
				errors.Details(errE)["sort"] = i
				return errE
			}
		} else {
			seenNonGroup = true
		}
		if k.Expand && !k.Group {
			errE := errors.New("only grouped columns may be expanded")
			errors.Details(errE)["sort"] = i
			return errE
		}
	}
	return nil
}

// reverseScopeQuery returns a query matching documents that have a ref claim
// or a sub-reference claim with "to" equal to the given ID, regardless of which
// property the ref is for.
func reverseScopeQuery(id identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return esdsl.NewBoolQuery().Should(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(id.String())),
		).Path("claims.ref"),
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(id.String())),
		).Path("claims.subRef"),
	).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// withFilters returns musts as a bool query, adding any non-nil filters as filter clauses.
//
// When there are no scoring (must) clauses the documents are selected purely by membership, and we
// want their score to be 0. An empty bool query would instead match all documents with score 1, so
// when neither musts nor filters are present we add a match_all filter clause. Filter clauses do not
// contribute to the score, so such results stay at score 0 (the counts.score function_score, being a
// multiply, then leaves them at 0). Only a query or filters (which go into musts) produce non-zero scores.
func withFilters(musts, filters []types.QueryVariant) types.QueryVariant { //nolint:ireturn
	query := esdsl.NewBoolQuery().Must(musts...)
	fs := make([]types.QueryVariant, 0, len(filters))
	for _, f := range filters {
		if f != nil {
			fs = append(fs, f)
		}
	}
	if len(musts) == 0 && len(fs) == 0 {
		fs = append(fs, esdsl.NewMatchAllQuery())
	}
	if len(fs) > 0 {
		return query.Filter(fs...)
	}
	return query
}

// ToQuery converts the Session to an ElasticSearch query.
//
// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
// enabledLanguages is the site's indexed language set, used to scope the text-search query
// to the languages the index actually has (empty falls back to the global default).
// extraFilters are added as bool filter clauses (used for the per-caller access restriction).
func (s *SessionData) ToQuery(enabledLanguages []string, extraFilters ...types.QueryVariant) types.QueryVariant { //nolint:ireturn
	musts := make([]types.QueryVariant, 0, len(s.Filters)+1)

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.And, enabledLanguages))
	}

	for i := range s.Filters {
		musts = append(musts, s.filterQuery(&s.Filters[i], nil))
	}

	filters := make([]types.QueryVariant, 0, len(extraFilters)+len(s.Prefilters)+1)
	filters = append(filters, extraFilters...)

	// Prefilters constrain the result set like filters but go into the filter clause, so
	// they do not contribute to _score.
	for i := range s.Prefilters {
		filters = append(filters, s.filterQuery(&s.Prefilters[i], nil))
	}

	// Reverse scopes results to documents that reference the target (directly or via a
	// sub-reference). It is a pure membership constraint, so it goes in the filter clause
	// and does not contribute to _score.
	if s.Reverse != nil {
		filters = append(filters, reverseScopeQuery(*s.Reverse))
	}

	return withFilters(musts, filters)
}

// ToQueryExcluding converts the SessionData to an ElasticSearch query, excluding
// the filter with the given ID. This is used when fetching filter data so that
// the current filter's own restrictions do not affect its available values.
// enabledLanguages scopes the text-search query as in ToQuery.
func (s *SessionData) ToQueryExcluding( //nolint:ireturn
	excludeFilterID identifier.Identifier, enabledLanguages []string, extraFilters ...types.QueryVariant,
) types.QueryVariant {
	musts := make([]types.QueryVariant, 0, len(s.Filters)+1)

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.And, enabledLanguages))
	}

	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == excludeFilterID {
			continue
		}
		musts = append(musts, s.filterQuery(&s.Filters[i], &excludeFilterID))
	}

	filters := make([]types.QueryVariant, 0, len(extraFilters)+len(s.Prefilters)+1)
	filters = append(filters, extraFilters...)

	// Prefilters constrain the result set like filters but go into the filter clause, so
	// they do not contribute to _score. The excluded filter is skipped here too, in case
	// it is a prefilter.
	for i := range s.Prefilters {
		if s.Prefilters[i].ID != nil && *s.Prefilters[i].ID == excludeFilterID {
			continue
		}
		filters = append(filters, s.filterQuery(&s.Prefilters[i], &excludeFilterID))
	}

	// Reverse scopes results to documents that reference the target (directly or via a
	// sub-reference). It is a pure membership constraint, so it goes in the filter clause
	// and does not contribute to _score.
	if s.Reverse != nil {
		filters = append(filters, reverseScopeQuery(*s.Reverse))
	}

	return withFilters(musts, filters)
}

// filterQuery builds the ES query for the filter at idx, dispatching to the
// matching per-filter-type query builder and applying cross-filter
// restrictions for sub-claim filters: when the filter is a sub-claim filter
// and the session has sibling top-level ref filters on the same parent
// property with To values, the sub-claim match is constrained so its parentTo
// is one of those values. This way "location=L1 AND location > artist=A"
// matches only documents where A is nested under L1.
//
// excludeID, when non-nil, is the ID of a filter excluded from the session
// (the caller of ToQueryExcluding) and is also skipped when collecting parent
// ref filters that contribute restrictions.
//
// This is the single point that knows how to render any filter shape; it is
// the only place SessionData.ToQuery and ToQueryExcluding go through.
func (s *SessionData) filterQuery(f *Filter, excludeID *identifier.Identifier) types.QueryVariant { //nolint:ireturn
	switch {
	case f.Has != nil:
		if len(f.Prop) == 1 {
			return f.Has.ToSubHasQuery(f.Prop[0], s.collectParentToRestrictions(f, f.Prop[0], excludeID))
		}
		return f.Has.ToQuery()
	case f.Ref != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Ref.ToSubRefQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(f, f.Prop[0], excludeID))
		}
		return f.Ref.ToQuery(f.Prop[0])
	case f.Amount != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Amount.ToSubAmountQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(f, f.Prop[0], excludeID))
		}
		return f.Amount.ToQuery(f.Prop[0])
	case f.Time != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Time.ToSubTimeQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(f, f.Prop[0], excludeID))
		}
		return f.Time.ToQuery(f.Prop[0])
	}
	panic(errors.New("invalid filter"))
}

// collectParentToRestrictions returns the set of parentTo values that the sub-claim
// filter current should be restricted to, gathered from sibling top-level ref filters
// on the same parentProp. It scans both Filters and Prefilters, so a top-level ref in
// either set restricts sub-claim filters in either set. The current filter (matched by
// identity) and (if non-nil) the filter with excludeID are skipped.
func (s *SessionData) collectParentToRestrictions(current *Filter, parentProp identifier.Identifier, excludeID *identifier.Identifier) []identifier.Identifier {
	var restrictions []identifier.Identifier
	for _, set := range [][]Filter{s.Filters, s.Prefilters} {
		for i := range set {
			other := &set[i]
			if other == current {
				continue
			}
			if excludeID != nil && other.ID != nil && *other.ID == *excludeID {
				continue
			}
			if other.Ref == nil || len(other.Prop) != 1 || other.Prop[0] != parentProp {
				continue
			}
			for _, to := range other.Ref.To {
				restrictions = append(restrictions, to.ID)
			}
		}
	}
	return restrictions
}

// Session represents a search session.
//
// A search session includes WHAT is being searched for and HOW are
// results shown/visualized, but not WHERE the user is looking at.
type Session struct {
	SessionData

	ID      identifier.Identifier `json:"id"`
	Base    []string              `json:"base"`
	Version int                   `json:"version"`
}

// Validate validates the Session struct.
func (s *Session) Validate(ctx context.Context) errors.E {
	if len(s.Base) < 2 { //nolint:mnd
		errE := errors.New("base must have at least two elements")
		errors.Details(errE)["length"] = len(s.Base)
		return errE
	}

	expectedID := identifier.From(s.Base...)
	if s.ID != expectedID {
		errE := errors.New("invalid session ID")
		errors.Details(errE)["got"] = s.ID.String()
		errors.Details(errE)["expected"] = expectedID.String()
		return errE
	}

	return s.SessionData.Validate(ctx, false)
}

func documentTextSearchQuery(searchQuery string, defaultOperator operator.Operator, enabledLanguages []string) types.QueryVariant { //nolint:ireturn
	if searchQuery == "" {
		return esdsl.NewBoolQuery()
	}

	shoulds := make([]types.QueryVariant, 0, 3) //nolint:mnd
	shoulds = append(shoulds, esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(searchQuery)))
	// Search aggregated textual content (string, html-stripped, identifier, link).
	// Language-tagged content lives in its own text.<lang> bucket; language-neutral
	// content (IDs, numbers, dates, fallback-resolved display labels) lives only in
	// text.und. Each per-language clause searches [text.<lang>, text.und] together:
	// a multi-field simple_query_string ORs each term across the two fields, so an
	// AND query can satisfy one term from the language field and another from und
	// within the same clause, and the score is their sum. The outer dis_max then
	// picks the best language per doc.
	//
	// Each language has multi-fields indexed alongside the main field:
	//   - text.<lang>            stemmed/lemmatized (language-specific, ICU folded)
	//   - text.<lang>.unstemmed  surface form (ICU folded only, no stemming)
	//   - text.<lang>.exact      diacritic-preserved (lowercase only, no folding)
	// text.und uses the standard (ICU-folded, unstemmed) analyzer as its main field
	// and has a .exact sub-field but no .unstemmed (its main field already is the
	// unstemmed analyzer). Per language we emit three clauses, each combined with
	// text.und:
	//   - Exact-routed: [text.<lang>, text.und] with quote_field_suffix=".exact".
	//     Unquoted terms hit the main analyzers; quoted phrases route to the .exact
	//     sub-fields (both text.<lang>.exact and text.und.exact exist) for
	//     diacritic-preserved matching. Wildcards stay literal here.
	//   - Stemmed-phrase: [text.<lang>, text.und] with no suffix. Quoted phrases get
	//     stemmed-phrase matching on text.<lang> (inflected forms) and folded-phrase
	//     matching on text.und. Unquoted terms duplicate the exact-routed clause;
	//     dis_max collapses the duplicate.
	//   - Unstemmed/wildcard: [text.<lang>.unstemmed, text.und] with
	//     analyze_wildcard=true. Both are und_text, so wildcards get folded
	//     before prefix matching. The und companion is text.und directly.
	// "und" rides inside every clause via its main field or .exact, so it needs no
	// standalone clauses. enabledLanguages is the site's indexed set; it falls back to the
	// global SupportedLanguages when empty. Both always contain non-und languages, so the
	// loop always emits clauses.
	const undField = "text." + document.UndeterminedLanguage
	langs := enabledLanguages
	if len(langs) == 0 {
		langs = slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	}
	textQueries := make([]types.QueryVariant, 0, len(langs)*3) //nolint:mnd
	for _, lang := range langs {
		if lang == document.UndeterminedLanguage {
			continue
		}
		field := "text." + lang
		// Exact-routed clause: quoted phrases go to .exact (diacritic-preserved),
		// unquoted terms hit the main fields.
		textQueries = append(textQueries,
			esdsl.NewSimpleQueryStringQuery(searchQuery).
				Fields(field, undField).
				DefaultOperator(defaultOperator).
				QuoteFieldSuffix(".exact"),
		)
		// Stemmed-phrase clause: quoted phrases match the stemmed language field
		// (inflected forms) and the folded und field. Unquoted terms duplicate the
		// exact-routed clause; dis_max collapses the duplicate score.
		textQueries = append(textQueries,
			esdsl.NewSimpleQueryStringQuery(searchQuery).
				Fields(field, undField).
				DefaultOperator(defaultOperator),
		)
		// Unstemmed clause: wildcards analyzed against surface tokens; both fields
		// are und_text so the typed prefix gets lowercased and ICU-folded.
		textQueries = append(textQueries,
			esdsl.NewSimpleQueryStringQuery(searchQuery).
				Fields(field+".unstemmed", undField).
				DefaultOperator(defaultOperator).
				AnalyzeWildcard(true),
		)
	}
	if len(textQueries) == 0 {
		// Only "und" is enabled, so the loop above emitted nothing. Search text.und
		// directly: exact-routed for quoted phrases, analyze_wildcard for the rest.
		textQueries = append(textQueries,
			esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(undField).DefaultOperator(defaultOperator).QuoteFieldSuffix(".exact"),
			esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(undField).DefaultOperator(defaultOperator).AnalyzeWildcard(true),
		)
	}
	shoulds = append(shoulds, esdsl.NewDisMaxQuery().Queries(textQueries...).TieBreaker(textDisMaxTieBreaker))
	// Search the document's top-level rendered display label across languages.
	// Each language is a separate clause inside a dis_max so per-doc the best
	// matching language wins (instead of summing across redundant translations),
	// and each clause is boosted so a match against the document's user-visible
	// label outranks an incidental match inside aggregated text.
	//
	// Each display.<lang> main analyzer is und_text (no stemming) and
	// has an .exact sub-field with diacritic preservation, mirroring text.und.
	// quote_field_suffix routes quoted phrases to .exact. analyze_wildcard
	// keeps wildcards on the main field so the typed prefix gets lowercased
	// and ICU-folded before prefix matching.
	displayQueries := make([]types.QueryVariant, 0, len(langs))
	for _, lang := range langs {
		displayQueries = append(displayQueries,
			esdsl.NewSimpleQueryStringQuery(searchQuery).
				Fields("display."+lang).
				DefaultOperator(defaultOperator).
				QuoteFieldSuffix(".exact").
				AnalyzeWildcard(true).
				Boost(topDisplayBoost),
		)
	}
	shoulds = append(shoulds, esdsl.NewDisMaxQuery().Queries(displayQueries...).TieBreaker(textDisMaxTieBreaker))
	recall := esdsl.NewBoolQuery().Should(shoulds...)

	// Phrase proximity boost. Single-term queries get no benefit (match_phrase
	// with one token degenerates to a term match that's already covered by the
	// recall layer), so we skip them.
	if len(strings.Fields(searchQuery)) < 2 { //nolint:mnd
		return recall
	}

	// Build per-language match_phrase clauses against text and display. The
	// analyzer of each field tokenizes the input the same way it tokenized
	// indexed content, so any simple_query_string operator characters in the
	// query are dropped as punctuation here. That's fine because the phrase
	// clauses are gated by the recall layer below: they only contribute score
	// to docs the simple_query_string-driven recall already admitted.
	// dis_max picks the best language per doc instead of summing across
	// translations. tie_breaker rewards multi-language matches lightly.
	phraseQueries := make([]types.QueryVariant, 0, len(langs)*2) //nolint:mnd
	for _, lang := range langs {
		phraseQueries = append(phraseQueries,
			esdsl.NewMatchPhraseQuery("text."+lang, searchQuery).Slop(phraseSlop),
			esdsl.NewMatchPhraseQuery("display."+lang, searchQuery).Slop(phraseSlop).Boost(topDisplayBoost),
		)
	}
	phrase := esdsl.NewDisMaxQuery().Queries(phraseQueries...).TieBreaker(textDisMaxTieBreaker).Boost(phraseBoost)

	// must wraps the recall query so only docs admitted by simple_query_string
	// can score. The outer should adds the phrase clause as a pure boost on
	// top of the recall score.
	return esdsl.NewBoolQuery().Must(recall).Should(phrase)
}

// labelMatchQuery builds the query that narrows a facet to records whose name matches the user-typed text q,
// OR-ing two kinds of label match so a facet can be reached either through one of its values or through a
// property name:
//
//   - Value labels (valueNamingFields/valueDisplayFields, for example claims.ref.toNaming / toDisplay) are
//     the referenced document's name. A value can be any document, so these are matched the same way the main
//     result search matches documents: a simple_query_string giving stemmed recall over the naming strings
//     and exact-routed, diacritic-folded, prefix matching over the display label. The frontend's trailing "*"
//     is kept here for prefix search, exactly as the main search and the reference autocomplete input do.
//   - Property labels (propNamingFields/propDisplayFields, for example claims.ref.propDisplay, a parent
//     property's claims.subRef.parentPropDisplay, or a has-property's claims.has.propDisplay) come from the
//     controlled set of properties and are matched with match_phrase_prefix over the whole label, which
//     prefix-matches a multi-word name as an ordered phrase (so "instance of" matches) and does its own
//     prefixing, so the trailing "*" is stripped there.
//
// The result feeds an aggregation filter, which runs in filter context (facet values are ordered by document
// count, not scored), so relevance and phrase-proximity boosts would be inert and are intentionally omitted;
// only clauses that change the matched set are included. enabledLanguages selects the per-language sub-fields
// present in the index; an empty list falls back to all supported ones.
func labelMatchQuery( //nolint:ireturn
	valueNamingFields, valueDisplayFields, propNamingFields, propDisplayFields []string, q string, enabledLanguages []string,
) types.QueryVariant {
	langs := enabledLanguages
	if len(langs) == 0 {
		langs = slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	}

	// Value labels (a referenced document's name) match like the main result search: the prefix is an
	// analyze_wildcard query, keeping the typed trailing "*".
	shoulds := valueSearchClauses(valueNamingFields, valueDisplayFields, q, langs)
	// Property labels (a controlled property name) match with the same regular recall, but a match_phrase_prefix
	// prefix, so a multi-word name like "instance of" matches as an ordered phrase.
	shoulds = append(shoulds, propSearchClauses(propNamingFields, propDisplayFields, q, langs)...)

	// A query that is only wildcards over no value fields leaves no clause (every facet is shown in full).
	if len(shoulds) == 0 {
		return esdsl.NewMatchAllQuery()
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// propLabelMatchQuery is labelMatchQuery for facets whose only searchable label is a property name (has, and
// the sub-claim discovery and missing-bucket gating). Those have no value documents to search.
func propLabelMatchQuery(namingFields, displayFields []string, q string, enabledLanguages []string) types.QueryVariant { //nolint:ireturn
	return labelMatchQuery(nil, nil, namingFields, displayFields, q, enabledLanguages)
}

// amountTimeMatchQuery matches an amount or time facet (top-level or sub) by a property name or by the
// formatted display of its value bounds. The property name fields (per-language propNaming/propDisplay, plus
// the parent property's for sub-facets) use propSearchClauses, the same match_phrase_prefix plus regular
// recall as other property labels. The value bounds (the flat und_text from/toDisplay fields, which carry no
// per-language buckets or sub-fields) are matched value-style, with an analyze_wildcard simple_query_string
// that keeps the typed trailing "*", so a typed year or number surfaces its facet. A match on either label
// surfaces the whole facet (its histogram still renders in full, since amount and time facets do not narrow
// their values by the query).
func amountTimeMatchQuery(propNamingFields, propDisplayFields, valueDisplayFields []string, q string, enabledLanguages []string) types.QueryVariant { //nolint:ireturn
	langs := enabledLanguages
	if len(langs) == 0 {
		langs = slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	}

	shoulds := propSearchClauses(propNamingFields, propDisplayFields, q, langs)
	for _, field := range valueDisplayFields {
		shoulds = append(shoulds, esdsl.NewSimpleQueryStringQuery(q).Fields(field).DefaultOperator(operator.And).AnalyzeWildcard(true))
	}

	if len(shoulds) == 0 {
		return esdsl.NewMatchAllQuery()
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// regularRecallClauses builds the non-prefix recall shared by value and property label matching, via
// simple_query_string: each naming field matched stemmed per language combined with the language-neutral und
// bucket (inflected full-word recall), and each display field matched with quoted phrases routed to its
// diacritic-preserved "exact" sub-field. The query is passed verbatim.
func regularRecallClauses(namingFields, displayFields []string, q string, langs []string) []types.QueryVariant {
	var clauses []types.QueryVariant
	for _, namingField := range namingFields {
		undNaming := namingField + "." + document.UndeterminedLanguage
		hasLang := false
		for _, lang := range langs {
			if lang == document.UndeterminedLanguage {
				continue
			}
			hasLang = true
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(namingField+"."+lang, undNaming).DefaultOperator(operator.And))
		}
		if !hasLang {
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(undNaming).DefaultOperator(operator.And))
		}
	}
	for _, displayField := range displayFields {
		for _, lang := range langs {
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(displayField+"."+lang).DefaultOperator(operator.And).QuoteFieldSuffix(".exact"))
		}
	}
	return clauses
}

// valueSearchClauses matches a value label (a referenced document's name) like the main result search:
// regularRecallClauses plus an analyze_wildcard simple_query_string over the unstemmed surface fields (a
// language bucket's .unstemmed sub-field with the und bucket, and the und_text display field), so the typed
// trailing "*" prefix-matches regardless of case or diacritics. The query keeps its trailing "*".
func valueSearchClauses(namingFields, displayFields []string, q string, langs []string) []types.QueryVariant {
	clauses := regularRecallClauses(namingFields, displayFields, q, langs)
	for _, namingField := range namingFields {
		undNaming := namingField + "." + document.UndeterminedLanguage
		hasLang := false
		for _, lang := range langs {
			if lang == document.UndeterminedLanguage {
				continue
			}
			hasLang = true
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(namingField+"."+lang+".unstemmed", undNaming).DefaultOperator(operator.And).AnalyzeWildcard(true))
		}
		if !hasLang {
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(undNaming).DefaultOperator(operator.And).AnalyzeWildcard(true))
		}
	}
	for _, displayField := range displayFields {
		for _, lang := range langs {
			clauses = append(clauses, esdsl.NewSimpleQueryStringQuery(q).Fields(displayField+"."+lang).DefaultOperator(operator.And).AnalyzeWildcard(true))
		}
	}
	return clauses
}

// propSearchClauses matches a property label (a controlled property name) with the same regularRecallClauses
// as valueSearchClauses, differing only in the prefix: a match_phrase_prefix over the unstemmed surface fields
// (a language bucket's .unstemmed sub-field or the und bucket, and the und_text display field) rather than an
// analyze_wildcard query, so a multi-word name like "instance of" matches as an ordered phrase. It takes the
// raw query and strips the trailing "*" itself (match_phrase_prefix prefixes the last term intrinsically); an
// all-wildcard query then adds no prefix clause.
func propSearchClauses(namingFields, displayFields []string, q string, langs []string) []types.QueryVariant {
	clauses := regularRecallClauses(namingFields, displayFields, q, langs)
	prefix := strings.TrimSuffix(q, "*")
	if prefix == "" {
		return clauses
	}
	for _, namingField := range namingFields {
		for _, lang := range langs {
			field := namingField + "." + lang
			if lang != document.UndeterminedLanguage {
				field += ".unstemmed"
			}
			clauses = append(clauses, esdsl.NewMatchPhrasePrefixQuery(field, prefix))
		}
	}
	for _, displayField := range displayFields {
		for _, lang := range langs {
			clauses = append(clauses, esdsl.NewMatchPhrasePrefixQuery(displayField+"."+lang, prefix))
		}
	}
	return clauses
}

// TODO: Use a database instead.
var searches = sync.Map{} //nolint:gochecknoglobals

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// CreateSession creates a new search session.
func CreateSession(ctx context.Context, session *Session) errors.E {
	errE := session.Validate(ctx)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(session.ID, session)

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit store twice?

	return nil
}

// UpdateSession updates an existing search session.
func UpdateSession(ctx context.Context, session *Session) errors.E {
	errE := session.Validate(ctx)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(session.ID, session)

	return nil
}

// GetSessionFromID resolves an existing search session if possible.
func GetSessionFromID(ctx context.Context, value string) (*Session, errors.E) {
	id, errE := identifier.MaybeString(value)
	if errE != nil {
		return nil, errors.WrapWith(errE, ErrNotFound)
	}

	return GetSession(ctx, id)
}

// GetSession resolves an existing search session if possible.
func GetSession(_ context.Context, id identifier.Identifier) (*Session, errors.E) {
	session, ok := searches.Load(id)
	if !ok {
		return nil, errors.WithDetails(ErrNotFound, "id", id)
	}
	// We make a shallow copy so that it is closer to how would a real database retrieve it.
	s := *session.(*Session) //nolint:forcetypeassert,errcheck
	return &s, nil
}

// Result represents a search result document.
//
// When results are grouped, a node with Group set is a group heading: ID is the referenced value's
// document ID, Count is the number of documents in the group, and Group holds the nested sub-groups or
// the documents in that group. A node without Group is a plain result document (a leaf).
//
// A group heading whose ID is MissingValueID is the synthetic "missing" group: it holds the documents
// that are missing this level's grouping property (the frontend renders it with a localized label).
type Result struct {
	ID    string   `json:"id"`
	Count *int64   `json:"count,omitempty"`
	Group []Result `json:"group,omitempty"`
}

// ResultsGet retrieves search results for a given search session.
func ResultsGet(
	ctx context.Context, getSearchService func() *esSearch.Search, searchData *SessionData, enabledLanguages []string, factor float64,
	extraFilters ...types.QueryVariant,
) ([]Result, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchData.ToQuery(enabledLanguages, extraFilters...)

	// TODO: Add a constant-score search mode (feature flag) for per-document ACL filters, so they cannot leak document existence through _score term statistics.
	//       Per-level indexes close the role/visibility side channels: each level's index computes IDF, term and document
	//       frequencies, and aggregation statistics only over that level's documents. They cannot close it for a per-document
	//       (per-user) ACL applied as a query-time filter (the SearchQueryHook threaded in here as extraFilters): a filter
	//       clause drops documents from the result set but not from the collection statistics. BM25 _score mixes per-document
	//       local stats (term frequency, field length) with collection-global stats (IDF, a function of how many documents in
	//       the whole index contain the term), so the _score of accessible hits still encodes statistics over inaccessible
	//       documents. A caller probing a rare term can read _score (or watch ranking shift across probes) and back out the
	//       rough number of documents behind the ACL that contain it: statistical existence, not content. The Dfsquerythenfetch
	//       below makes this exact rather than per-shard-noisy by summing document frequencies across shards. One index per
	//       user is not possible (unbounded, ACLs change), so the inaccessible documents stay in the index, and the only fix
	//       is to make nothing the caller observes depend on those statistics.
	//
	//       Avoid, whenever a per-document ACL filter can be active: exposing _score or ranking by it; the counts.score
	//       field_value_factor boost below (it carries the same leak when counts.score spans inaccessible documents); More-Like-This
	//       and significant_terms/significant_text (IDF-by-design, they select terms against the whole-index background, so
	//       constant_score does not help and they must be kept off any surface where ACL-restricted and accessible documents share an
	//       index); kNN/vector post-filtering (use a pre-filter, and build the embedding per level so a single global vector does not
	//       leak stripped fields through embedding inversion).
	//
	//       Safe under a per-document ACL filter (post-filter or document-local, nothing to change): ordinary terms, histogram, and
	//       cardinality facet counts; track_total_hits; highlighting on the matched document. Relevance scoring is the only place ES
	//       reaches past the filter into collection-global statistics.
	//
	//       Implement the flag by dropping relevance ranking when a per-document ACL is active: wrap the matching query in
	//       constant_score (every match scores the same), skip the counts.score function_score, do not expose _score (set
	//       track_scores false), and order only by stable per-document keys (time, then displaySort) that do not depend on collection
	//       statistics. Matching then reduces to a per-document boolean test and the order is independent of inaccessible documents,
	//       so nothing observable leaks term statistics. A mapping-level alternative to constant_score is to give the text field the
	//       boolean similarity (it scores on query boosts only, no TF/IDF). (Per-shard IDF noise is not a defense.)

	// Multiplicatively boost ranking by the document's counts.score (its own claims
	// plus the documents referencing it) so that, among equally relevant text
	// matches, richer and more connected documents rank higher. factor is corpus-derived
	// and scales the log2p curve; factor of 0 leaves the query unboosted.
	if factor > 0 {
		query = esdsl.NewFunctionScoreQuery().
			Query(query).
			Functions(
				esdsl.NewFunctionScore().FieldValueFactor(
					esdsl.NewFieldValueFactorScoreFunction().
						Field("counts.score").
						Factor(types.Float64(factor)).
						Modifier(fieldvaluefactormodifier.Log2p).
						Missing(0),
				),
			).
			BoostMode(functionboostmode.Multiply)
	}

	searchService := getSearchService()
	lang := searchData.Language

	// A leading run of group=true sort keys groups the results (feed view only); the remaining keys order
	// documents within each leaf group. Without group keys, the results are a flat sorted list ordered by
	// the sort keys, then the default tail (relevance, then earliest time, then display label).
	groupCols := leadingGroupKeys(searchData.Sort)
	grouped := len(groupCols) > 0 && searchData.View == ViewFeed

	// Score with global term/document frequencies across all shards (DFS) instead of each shard's local
	// statistics. With multiple shards a term's IDF otherwise depends on which shard a document happens to
	// land on, and that skew is amplified by deleted (re-indexed) documents whose term statistics linger
	// per shard until merged. The result is inconsistent BM25 scoring across documents and unstable ranking.
	if grouped {
		searchService = searchService.Size(0).Query(query).
			AddAggregation(groupAggName, buildGroupAggregation(groupCols, buildSort(searchData.Sort[len(groupCols):], lang), lang)).
			SearchType(searchtype.Dfsquerythenfetch)
	} else {
		searchService = searchService.From(0).Size(MaxResultsCount).Query(query).
			Sort(buildSort(searchData.Sort, lang)...).SearchType(searchtype.Dfsquerythenfetch)
	}

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	var results []Result
	if grouped {
		var errE errors.E
		results, errE = foldGroups(res.Aggregations, groupCols)
		if errE != nil {
			return nil, nil, errE
		}
		// foldGroups assembles every bucket's documents (up to groupTopK each); the flat path is bounded by
		// Size(MaxResultsCount), but Elasticsearch cannot bound across grouping buckets, so cap the assembled
		// tree to the first MaxResultsCount results here.
		results, _ = limitGroups(results, MaxResultsCount)
	} else {
		results = make([]Result, 0, len(res.Hits.Hits))
		for _, hit := range res.Hits.Hits {
			results = append(results, Result{ID: *hit.Id_}) //nolint:exhaustruct
		}
	}

	// Total is a string or a number.
	var total any
	if res.Hits.Total.Relation == totalhitsrelation.Gte {
		total = fmt.Sprintf("%d+", res.Hits.Total.Value)
	} else {
		total = res.Hits.Total.Value
	}

	return results, map[string]any{
		"total": total,
	}, nil
}

// ScoreFactor computes the field_value_factor coefficient for the counts.score
// ranking boost from the current corpus. It runs a percentiles aggregation over the
// whole index and returns (2^scoreBoostMax - 2)/p99, so that under the log2p
// modifier a document at the corpus 99th percentile of counts.score receives a boost
// roughly scoreBoostMax times that of an empty document. It returns 0 (no boost)
// when the corpus is too sparse to have a meaningful p99 (p99 < 1).
//
// The factor is corpus-global.
func ScoreFactor(ctx context.Context, getSearchService func() *esSearch.Search) (float64, errors.E) {
	searchService := getSearchService().Size(0).AddAggregation(
		"scoreP99",
		esdsl.NewAggregations().Percentiles(
			esdsl.NewPercentilesAggregation().Field("counts.score").Percents(scorePercentile).Keyed(false),
		),
	)

	res, err := searchService.Do(ctx)
	if err != nil {
		return 0, WithESError(err)
	}

	agg, errE := internalSearch.AggAs[types.TDigestPercentilesAggregate](res.Aggregations, "scoreP99")
	if errE != nil {
		return 0, errE
	}

	// With Keyed(false) the percentiles come back as an array; we requested a single
	// percentile, and its Value is nil when the corpus is empty.
	items, ok := agg.Values.([]types.ArrayPercentilesItem)
	if !ok || len(items) == 0 || items[0].Value == nil {
		return 0, nil
	}
	p99 := float64(*items[0].Value)
	if p99 < 1 {
		return 0, nil
	}

	return (math.Pow(log2pOffset, scoreBoostMax) - log2pOffset) / p99, nil
}
