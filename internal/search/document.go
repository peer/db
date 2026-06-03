package search

import (
	"math"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// Document represents data indexed by ElasticSearch.
//
// It should match generated mapping.
//
// It has some similarities to document.D, but it is optimized for searching.
//
// Text aggregates textual content (from IdentifierClaim, StringClaim, HTMLClaim,
// LinkClaim source claims) into per-language arrays at the document root so the
// text-search query can score multiple terms in the same field together and
// reward documents where matches come from several textual claims.
//
// Display holds, per supported language, the document's rendered display label
// together with its ancestor display labels (its hierarchy paths, split into
// individual labels), so the document is also findable and boosted by its
// categories/ancestors. It is indexed with the und_text analyzer per language
// because the values might contain mixed-language content.
//
// Time holds the document's earliest time: the lowest time value across all of
// its time claims (top-level and sub-claims). For a point timestamp that is the
// timestamp; for an interval it is the earliest bound.
//
// ReferencesCount is the number of other documents that reference this document,
// counted at index time and refreshed by a periodic job.
//
// ClaimsCount is the total number of claims the document has, counted
// recursively including sub-claims.
type Document struct {
	ID identifier.Identifier `json:"id"`

	Display map[string][]string `json:"display,omitempty"`

	Text map[string][]string `json:"text,omitempty"`

	Time *float64 `json:"time,omitempty"`

	ReferencesCount *int `json:"referencesCount,omitempty"`

	ClaimsCount *int `json:"claimsCount,omitempty"`

	Claims ClaimTypes `json:"claims,omitzero"`
}

// ClaimTypes organizes claims by their type. Synthetic sub-claim types
// (SubRef, SubAmount, SubTime, SubHas) flatten nested sub-claims from
// parent claims (ref, has, none, unknown) so they can be matched by sub-claim
// filters without ES join queries.
type ClaimTypes struct {
	Amount    AmountClaims    `json:"amount,omitempty"`
	Time      TimeClaims      `json:"time,omitempty"`
	Reference ReferenceClaims `json:"ref,omitempty"`
	Has       HasClaims       `json:"has,omitempty"`
	None      NoneClaims      `json:"none,omitempty"`
	Unknown   UnknownClaims   `json:"unknown,omitempty"`

	SubRef    SubRefClaims    `json:"subRef,omitempty"`
	SubAmount SubAmountClaims `json:"subAmount,omitempty"`
	SubTime   SubTimeClaims   `json:"subTime,omitempty"`
	SubHas    SubHasClaims    `json:"subHas,omitempty"`
}

// AmountInterval and TimeInterval source claims are mapped to AmountClaim and
// TimeClaim records respectively (top-level and sub-claim alike): a point
// claim becomes a range whose endpoints coincide, while an interval claim
// becomes a range over its bounds.

type (
	// AmountClaims is a slice of AmountClaim.
	AmountClaims = []AmountClaim
	// TimeClaims is a slice of TimeClaim.
	TimeClaims = []TimeClaim
	// ReferenceClaims is a slice of ReferenceClaim.
	ReferenceClaims = []ReferenceClaim
	// HasClaims is a slice of HasClaim.
	HasClaims = []HasClaim
	// NoneClaims is a slice of NoneClaim.
	NoneClaims = []NoneClaim
	// UnknownClaims is a slice of UnknownClaim.
	UnknownClaims = []UnknownClaim
	// SubRefClaims is a slice of SubRefClaim.
	SubRefClaims = []SubRefClaim
	// SubAmountClaims is a slice of SubAmountClaim.
	SubAmountClaims = []SubAmountClaim
	// SubTimeClaims is a slice of SubTimeClaim.
	SubTimeClaims = []SubTimeClaim
	// SubHasClaims is a slice of SubHasClaim.
	SubHasClaims = []SubHasClaim
)

// RangeFloat represents a numeric range.
//
// Exactly one of GreaterThan or GreaterThanOrEqual must be set.
// Exactly one of LessThan or LessThanOrEqual must be set.
type RangeFloat struct {
	GreaterThan        *float64 `json:"gt,omitempty"`
	GreaterThanOrEqual *float64 `json:"gte,omitempty"`
	LessThan           *float64 `json:"lt,omitempty"`
	LessThanOrEqual    *float64 `json:"lte,omitempty"`
}

// Validate checks that the range is valid for indexing into Elasticsearch
// as a range field. It returns an error for any shape Elasticsearch would
// reject:
//   - Both gt and gte set, or both lt and lte set, or neither lower nor
//     upper bound set.
//   - NaN or Inf bound values.
//   - Lower bound strictly greater than upper bound.
//   - Equal numeric bounds with at least one strict side. ES accepts
//     gte == lte (single-point range) but rejects any equal-bound
//     combination involving a strict side.
//   - Strict-strict ranges where the two bounds are within 1 ULP of
//     each other.
func (r *RangeFloat) Validate() errors.E {
	if r.GreaterThan != nil && r.GreaterThanOrEqual != nil {
		errE := errors.New("both greater than and greater than or equal are set")
		errors.Details(errE)["range"] = r
		return errE
	}
	if r.LessThan != nil && r.LessThanOrEqual != nil {
		errE := errors.New("both less than and less than or equal are set")
		errors.Details(errE)["range"] = r
		return errE
	}
	if r.GreaterThan == nil && r.GreaterThanOrEqual == nil {
		errE := errors.New("greater than bound is required")
		errors.Details(errE)["range"] = r
		return errE
	}
	if r.LessThan == nil && r.LessThanOrEqual == nil {
		errE := errors.New("less than bound is required")
		errors.Details(errE)["range"] = r
		return errE
	}

	var lower float64
	switch {
	case r.GreaterThan != nil:
		lower = *r.GreaterThan
	case r.GreaterThanOrEqual != nil:
		lower = *r.GreaterThanOrEqual
	}

	var upper float64
	switch {
	case r.LessThan != nil:
		upper = *r.LessThan
	case r.LessThanOrEqual != nil:
		upper = *r.LessThanOrEqual
	}

	if math.IsNaN(lower) || math.IsInf(lower, 0) {
		errE := errors.New("lower bound is not a finite number")
		errors.Details(errE)["range"] = r
		return errE
	}
	if math.IsNaN(upper) || math.IsInf(upper, 0) {
		errE := errors.New("upper bound is not a finite number")
		errors.Details(errE)["range"] = r
		return errE
	}

	switch {
	case lower < upper:
		// Normal case.
	case lower == upper:
		// ES accepts gte == lte (single-point range). Any other equal-bound
		// combination has at least one strict side and is rejected by ES.
		if r.GreaterThanOrEqual != nil && r.LessThanOrEqual != nil {
			return nil
		}
		errE := errors.New("equal bounds with at least one strict bound")
		errors.Details(errE)["range"] = r
		return errE
	default:
		// lower > upper: rejected. Upstream is responsible for swapping
		// bounds before reaching this point.
		errE := errors.New("lower bound is greater than upper bound")
		errors.Details(errE)["range"] = r
		return errE
	}

	// Strict-strict adjacency: when both bounds are strict and within 1 ULP
	// of each other, ES rejects the range as empty.
	if r.GreaterThan != nil && r.LessThan != nil {
		if math.Nextafter(*r.GreaterThan, math.Inf(1)) > math.Nextafter(*r.LessThan, math.Inf(-1)) {
			errE := errors.New("strict bounds within one ULP of each other")
			errors.Details(errE)["range"] = r
			return errE
		}
	}

	return nil
}

// AmountClaim represents a claim for numeric amount and unit.
//
// For search, we index amounts as both ranges and boundaries.
type AmountClaim struct {
	Prop        identifier.Identifier  `json:"prop"`
	PropDisplay map[string]string      `json:"propDisplay"`
	PropNaming  map[string][]string    `json:"propNaming"`
	Unit        *identifier.Identifier `json:"unit"`
	Range       RangeFloat             `json:"range"`
	From        *float64               `json:"from,omitempty"`
	FromDisplay string                 `json:"fromDisplay,omitempty"`
	To          *float64               `json:"to,omitempty"`
	ToDisplay   string                 `json:"toDisplay,omitempty"`
}

// TimeClaim represents a claim for time.
//
// For search, we index times as both ranges and boundaries.
// Times are stored as float64 seconds since Unix epoch.
type TimeClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	Range       RangeFloat            `json:"range"`
	From        *float64              `json:"from,omitempty"`
	FromDisplay string                `json:"fromDisplay,omitempty"`
	To          *float64              `json:"to,omitempty"`
	ToDisplay   string                `json:"toDisplay,omitempty"`
}

// ReferenceClaim represents a claim that relates this document to another document.
type ReferenceClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	To          identifier.Identifier `json:"to"`
	ToDisplay   map[string]string     `json:"toDisplay"`
	ToNaming    map[string][]string   `json:"toNaming"`
	// ToPath contains ID-based hierarchy paths from root to the target document.
	// Each path is prefixed with the hierarchy property ID and ":" separator
	// (e.g., "<property_ID>:<root_ID>/<parent_ID>/<this_ID>"), followed by
	// ancestor IDs joined by "/". Multiple paths exist when the target has
	// multiple parents in a hierarchy or participates in multiple hierarchies.
	ToPath []string `json:"toPath,omitempty"`
	// ToDisplayPath contains per-language display hierarchy paths from root to the
	// target document. Each path is a string of display labels joined by null bytes,
	// which ensures correct hierarchical sort order.
	ToDisplayPath map[string][]string `json:"toDisplayPath,omitempty"`
}

// HasClaim represents a claim with just a property.
//
// HasClaim entries hold simple has claims with no sub-claims. Any sub-claims of
// a has claim are flattened into the appropriate Sub* records on the parent
// document with ParentTo=ParentToHas, so the has filter that queries claims.has
// naturally sees only simple has claims.
type HasClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
}

// NoneClaim represents a claim that explicitly states no value exists for a property.
type NoneClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
}

// UnknownClaim represents a claim where the value for a property is known to exist but is unknown.
type UnknownClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
}

// TODO: Index the parent claim's own ID alongside ParentTo.
//       ParentTo on the four Sub* claim types is the parent claim's target identity
//       (the ref's To, or the ParentToHas/None/Unknown sentinel). It is NOT the
//       parent claim's own claim ID. This is fine when a document carries at most
//       one parent claim with a given (parentProp, parentTo) - cross-filter joins
//       between two sub-claim filters that share the same parent prop then narrow
//       to entries under the same parent record.
//       When a document carries multiple parent claims that share the same
//       (parentProp, parentTo) - e.g. the same venue listed twice under HAS_LOCATION
//       with different periods - the cross-filter cannot distinguish them: each
//       sub-claim filter independently matches "some entry under any of those
//       parents", so a session like:
//       HAS_LOCATION = L
//       HAS_LOCATION > HAS_ARTIST = A
//       HAS_LOCATION > PERIOD in [X,Y]
//       matches an exhibition where one of its L-entries lists A and another of its
//       L-entries has period in [X,Y], rather than requiring the same L-entry to
//       satisfy both.
//       Fix: add a ParentID identifier.Identifier on each Sub* type, populated by
//       extractSubClaims from the parent CoreClaim.ID. Sub-claim filter queries
//       would then group their per-(parentProp, sub-claim-type) restrictions by
//       ParentID, so the joins narrow to the same parent record. Cross-filter
//       against a sibling top-level ref filter would still key on ParentTo (since
//       the top-level filter selects by target identity).

// SubRefClaim represents a denormalized nested reference sub-claim
// flattened from parent claims (ref, has, none, unknown) for cross-filtering.
type SubRefClaim struct {
	ParentProp  identifier.Identifier `json:"parentProp"`
	ParentTo    string                `json:"parentTo"`
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	To          identifier.Identifier `json:"to"`
	ToDisplay   map[string]string     `json:"toDisplay"`
	ToNaming    map[string][]string   `json:"toNaming"`
	// ToPath contains ID-based hierarchy paths from root to the target document.
	// Each path is prefixed with the hierarchy property ID and ":" separator
	// (e.g., "<property_ID>:<root_ID>/<parent_ID>/<this_ID>"), followed by
	// ancestor IDs joined by "/". Multiple paths exist when the target has
	// multiple parents in a hierarchy or participates in multiple hierarchies.
	ToPath []string `json:"toPath,omitempty"`
	// ToDisplayPath contains per-language display hierarchy paths from root to the
	// target document. Each path is a string of display labels joined by null bytes,
	// which ensures correct hierarchical sort order.
	ToDisplayPath map[string][]string `json:"toDisplayPath,omitempty"`
}

// SubAmountClaim represents a denormalized nested amount sub-claim flattened
// from parent claims (ref, has, none, unknown) so sub-amount filters can
// match without ES join queries. AmountInterval source claims are stored
// here too as a range over their bounds.
type SubAmountClaim struct {
	ParentProp  identifier.Identifier  `json:"parentProp"`
	ParentTo    string                 `json:"parentTo"`
	Prop        identifier.Identifier  `json:"prop"`
	PropDisplay map[string]string      `json:"propDisplay"`
	PropNaming  map[string][]string    `json:"propNaming"`
	Unit        *identifier.Identifier `json:"unit"`
	Range       RangeFloat             `json:"range"`
	From        *float64               `json:"from,omitempty"`
	FromDisplay string                 `json:"fromDisplay,omitempty"`
	To          *float64               `json:"to,omitempty"`
	ToDisplay   string                 `json:"toDisplay,omitempty"`
}

// SubTimeClaim represents a denormalized nested time sub-claim flattened from
// parent claims (ref, has, none, unknown) so sub-time filters can match
// without ES join queries. TimeInterval source claims are stored here too as
// a range over their bounds. Times are stored as float64 seconds since Unix
// epoch.
type SubTimeClaim struct {
	ParentProp  identifier.Identifier `json:"parentProp"`
	ParentTo    string                `json:"parentTo"`
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	Range       RangeFloat            `json:"range"`
	From        *float64              `json:"from,omitempty"`
	FromDisplay string                `json:"fromDisplay,omitempty"`
	To          *float64              `json:"to,omitempty"`
	ToDisplay   string                `json:"toDisplay,omitempty"`
}

// SubHasClaim represents a denormalized nested has-only sub-claim flattened
// from parent claims (ref, has, none, unknown) so sub-has filters can match
// without ES join queries. Only simple has sub-claims (those with no further
// sub-claims of their own) are recorded here; has sub-claims with their own
// sub-claims contribute to the appropriate Sub* records of their content
// types but do not themselves appear as SubHasClaim entries.
type SubHasClaim struct {
	ParentProp  identifier.Identifier `json:"parentProp"`
	ParentTo    string                `json:"parentTo"`
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
}
