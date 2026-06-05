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
	// empty one under the scoreCount ranking boost: a p99 document's _score is
	// multiplied by roughly scoreBoostMax times the multiplier an empty document
	// gets. It is the single tuning parameter of the boost.
	scoreBoostMax = 10.0

	// log2pOffset is the additive constant of the ElasticSearch log2p
	// field_value_factor modifier: the boost is log10(log2pOffset + factor*scoreCount).
	log2pOffset = 2.0

	// scoreCountPercentile is the corpus percentile of scoreCount that ScoreFactor
	// normalizes to the scoreBoostMax boost ceiling.
	scoreCountPercentile = 99.0
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
type AmountFilter struct {
	Unit    *identifier.Identifier `json:"unit,omitempty"`
	Gte     *float64               `json:"gte,omitempty"`
	Lte     *float64               `json:"lte,omitempty"`
	Missing bool                   `json:"missing,omitempty"`
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
	if f.Gte == nil && f.Lte == nil && !f.Missing {
		return errors.New("both gte and lte or missing has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
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
type TimeFilter struct {
	Gte     *float64 `json:"gte,omitempty"`
	Lte     *float64 `json:"lte,omitempty"`
	Missing bool     `json:"missing,omitempty"`
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
	if f.Gte == nil && f.Lte == nil && !f.Missing {
		return errors.New("both gte and lte or missing has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
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

// SessionData represents the data of the search session.
//
// When Reverse is set, the session is scoped to documents which have a ref claim
// (for any property) whose "to" target equals Reverse.
type SessionData struct {
	View    ViewType               `json:"view,omitempty"`
	Query   string                 `json:"query,omitempty"`
	Filters []Filter               `json:"filters,omitempty"`
	Reverse *identifier.Identifier `json:"reverse,omitempty"`
}

// Validate validates the session data .
func (s *SessionData) Validate(withoutSession bool) errors.E {
	seenFilters := map[identifier.Identifier]bool{}
	for i, f := range s.Filters {
		errE := f.Validate(withoutSession)
		if errE != nil {
			errors.Details(errE)["filter"] = i
			return errE
		}
		if !withoutSession {
			// We checked that f.ID is not nil in f.Validate().
			if seenFilters[*f.ID] {
				errE := errors.New("duplicate filter ID")
				errors.Details(errE)["id"] = f.ID.String()
				errors.Details(errE)["filter"] = i
				return errE
			}
			seenFilters[*f.ID] = true
		}
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

// withExtraFilters returns musts as a bool query, adding any non-nil extra
// queries as filter clauses. A site's per-caller access restriction (from
// base.B.SearchQueryHook) is threaded into every search query this way.
func withExtraFilters(musts, extraFilters []types.QueryVariant) types.QueryVariant { //nolint:ireturn
	query := esdsl.NewBoolQuery().Must(musts...)
	filters := make([]types.QueryVariant, 0, len(extraFilters))
	for _, f := range extraFilters {
		if f != nil {
			filters = append(filters, f)
		}
	}
	if len(filters) > 0 {
		return query.Filter(filters...)
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
	musts := make([]types.QueryVariant, 0, len(s.Filters)+2) //nolint:mnd

	if s.Reverse != nil {
		musts = append(musts, reverseScopeQuery(*s.Reverse))
	}

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.And, enabledLanguages))
	}

	for i := range s.Filters {
		musts = append(musts, s.filterQuery(i, nil))
	}

	return withExtraFilters(musts, extraFilters)
}

// ToQueryExcluding converts the SessionData to an ElasticSearch query, excluding
// the filter with the given ID. This is used when fetching filter data so that
// the current filter's own restrictions do not affect its available values.
// enabledLanguages scopes the text-search query as in ToQuery.
func (s *SessionData) ToQueryExcluding( //nolint:ireturn
	excludeFilterID identifier.Identifier, enabledLanguages []string, extraFilters ...types.QueryVariant,
) types.QueryVariant {
	musts := make([]types.QueryVariant, 0, len(s.Filters)+2) //nolint:mnd

	if s.Reverse != nil {
		musts = append(musts, reverseScopeQuery(*s.Reverse))
	}

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.And, enabledLanguages))
	}

	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == excludeFilterID {
			continue
		}
		musts = append(musts, s.filterQuery(i, &excludeFilterID))
	}

	return withExtraFilters(musts, extraFilters)
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
func (s *SessionData) filterQuery(idx int, excludeID *identifier.Identifier) types.QueryVariant { //nolint:ireturn
	f := s.Filters[idx]
	switch {
	case f.Has != nil:
		if len(f.Prop) == 1 {
			return f.Has.ToSubHasQuery(f.Prop[0], s.collectParentToRestrictions(idx, f.Prop[0], excludeID))
		}
		return f.Has.ToQuery()
	case f.Ref != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Ref.ToSubRefQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(idx, f.Prop[0], excludeID))
		}
		return f.Ref.ToQuery(f.Prop[0])
	case f.Amount != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Amount.ToSubAmountQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(idx, f.Prop[0], excludeID))
		}
		return f.Amount.ToQuery(f.Prop[0])
	case f.Time != nil:
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Time.ToSubTimeQuery(f.Prop[0], f.Prop[1], s.collectParentToRestrictions(idx, f.Prop[0], excludeID))
		}
		return f.Time.ToQuery(f.Prop[0])
	}
	panic(errors.New("invalid filter"))
}

// collectParentToRestrictions returns the set of parentTo values that a
// sub-claim filter at idx should be restricted to, gathered from sibling
// top-level ref filters on the same parentProp. The filter at idx and (if
// non-nil) the filter with excludeID are skipped.
func (s *SessionData) collectParentToRestrictions(idx int, parentProp identifier.Identifier, excludeID *identifier.Identifier) []identifier.Identifier {
	var restrictions []identifier.Identifier
	for i := range s.Filters {
		if i == idx {
			continue
		}
		other := &s.Filters[i]
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
func (s *Session) Validate() errors.E {
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

	return s.SessionData.Validate(false)
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

// TODO: Use a database instead.
var searches = sync.Map{} //nolint:gochecknoglobals

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// CreateSession creates a new search session.
func CreateSession(_ context.Context, session *Session) errors.E {
	errE := session.Validate()
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(session.ID, session)

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit store twice?

	return nil
}

// UpdateSession updates an existing search session.
func UpdateSession(_ context.Context, session *Session) errors.E {
	errE := session.Validate()
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
type Result struct {
	ID string `json:"id"`
}

// ResultsGet retrieves search results for a given search session.
func ResultsGet(
	ctx context.Context, getSearchService func() *esSearch.Search, searchData *SessionData, enabledLanguages []string, factor float64,
	extraFilters ...types.QueryVariant,
) ([]Result, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchData.ToQuery(enabledLanguages, extraFilters...)

	// Multiplicatively boost ranking by the document's scoreCount (its own claims
	// plus the documents referencing it) so that, among equally relevant text
	// matches, richer and more connected documents rank higher. factor is corpus-derived
	// and scales the log2p curve; factor of 0 leaves the query unboosted.
	if factor > 0 {
		query = esdsl.NewFunctionScoreQuery().
			Query(query).
			Functions(
				esdsl.NewFunctionScore().FieldValueFactor(
					esdsl.NewFieldValueFactorScoreFunction().
						Field("scoreCount").
						Factor(types.Float64(factor)).
						Modifier(fieldvaluefactormodifier.Log2p).
						Missing(0),
				),
			).
			BoostMode(functionboostmode.Multiply)
	}

	searchService := getSearchService()

	// Score with global term/document frequencies across all shards (DFS) instead of
	// each shard's local statistics. With multiple shards a term's IDF otherwise depends
	// on which shard a document happens to land on, and that skew is amplified by deleted
	// (re-indexed) documents whose term statistics linger per shard until merged. The
	// result is inconsistent BM25 scoring across documents and unstable ranking. DFS makes
	// IDF uniform so ranking no longer depends on shard placement. Only the ranked results
	// query needs this. Queries which run with Size(0) and are not scored.
	searchService = searchService.From(0).Size(MaxResultsCount).Query(query).SearchType(searchtype.Dfsquerythenfetch)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	results := make([]Result, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		results = append(results, Result{ID: *hit.Id_})
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

// ScoreFactor computes the field_value_factor coefficient for the scoreCount
// ranking boost from the current corpus. It runs a percentiles aggregation over the
// whole index and returns (2^scoreBoostMax - 2)/p99, so that under the log2p
// modifier a document at the corpus 99th percentile of scoreCount receives a boost
// roughly scoreBoostMax times that of an empty document. It returns 0 (no boost)
// when the corpus is too sparse to have a meaningful p99 (p99 < 1).
//
// The factor is corpus-global.
func ScoreFactor(ctx context.Context, getSearchService func() *esSearch.Search) (float64, errors.E) {
	searchService := getSearchService().Size(0).AddAggregation(
		"scoreCountP99",
		esdsl.NewAggregations().Percentiles(
			esdsl.NewPercentilesAggregation().Field("scoreCount").Percents(scoreCountPercentile).Keyed(false),
		),
	)

	res, err := searchService.Do(ctx)
	if err != nil {
		return 0, WithESError(err)
	}

	agg, errE := internalSearch.AggAs[types.TDigestPercentilesAggregate](res.Aggregations, "scoreCountP99")
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
