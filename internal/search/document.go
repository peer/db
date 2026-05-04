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
type Document struct {
	ID identifier.Identifier `json:"id"`

	Claims ClaimTypes `json:"claims,omitzero"`
}

// ClaimTypes organizes claims by their type.
type ClaimTypes struct {
	Identifier IdentifierClaims `json:"id,omitempty"`
	String     StringClaims     `json:"string,omitempty"`
	HTML       HTMLClaims       `json:"html,omitempty"`
	Amount     AmountClaims     `json:"amount,omitempty"`
	Time       TimeClaims       `json:"time,omitempty"`
	Link       LinkClaims       `json:"link,omitempty"`
	Reference  ReferenceClaims  `json:"ref,omitempty"`
	Has        HasClaims        `json:"has,omitempty"`
	None       NoneClaims       `json:"none,omitempty"`
	Unknown    UnknownClaims    `json:"unknown,omitempty"`
}

// There are no AmountInterval and TimeInterval claims because they are mapped
// to Amount and Time claims here, respectively.

type (
	// IdentifierClaims is a slice of IdentifierClaim.
	IdentifierClaims = []IdentifierClaim
	// StringClaims is a slice of StringClaim.
	StringClaims = []StringClaim
	// HTMLClaims is a slice of HTMLClaim.
	HTMLClaims = []HTMLClaim
	// AmountClaims is a slice of AmountClaim.
	AmountClaims = []AmountClaim
	// TimeClaims is a slice of TimeClaim.
	TimeClaims = []TimeClaim
	// LinkClaims is a slice of LinkClaim.
	LinkClaims = []LinkClaim
	// ReferenceClaims is a slice of ReferenceClaim.
	ReferenceClaims = []ReferenceClaim
	// HasClaims is a slice of HasClaim.
	HasClaims = []HasClaim
	// NoneClaims is a slice of NoneClaim.
	NoneClaims = []NoneClaim
	// UnknownClaims is a slice of UnknownClaim.
	UnknownClaims = []UnknownClaim
)

// IdentifierClaim represents a claim with a string identifier value.
type IdentifierClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	Value       string                `json:"value"`
}

// StringClaim represents a claim with a plain string value for a given language.
type StringClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`

	// Map contains exactly one value.
	String map[string]string `json:"string"`
}

// HTMLClaim represents a claim with HTML text content for a given language.
type HTMLClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`

	// Map contains exactly one value.
	HTML map[string]string `json:"html"`
}

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
// as a range field.
//
// Errors are returned for shapes Elasticsearch would reject:
//   - Both gt and gte set, or both lt and lte set, or neither lower nor
//     upper bound set.
//   - NaN or Inf bound values.
//   - Equal numeric bounds with at least one strict side. ES accepts
//     gte == lte (single-point range) but rejects any equal-bound
//     combination involving a strict side.
//   - Strict-strict ranges where the two bounds are within 1 ULP of
//     each other.
//
// When the lower bound is strictly greater than the upper bound, the
// bounds are swapped (because ES does not support interval direction
// like PeerDB does). It returns true if the bounds were swapped.
func (r *RangeFloat) Validate() (bool, errors.E) {
	if r.GreaterThan != nil && r.GreaterThanOrEqual != nil {
		errE := errors.New("both greater than and greater than or equal are set")
		errors.Details(errE)["range"] = r
		return false, errE
	}
	if r.LessThan != nil && r.LessThanOrEqual != nil {
		errE := errors.New("both less than and less than or equal are set")
		errors.Details(errE)["range"] = r
		return false, errE
	}
	if r.GreaterThan == nil && r.GreaterThanOrEqual == nil {
		errE := errors.New("greater than bound is required")
		errors.Details(errE)["range"] = r
		return false, errE
	}
	if r.LessThan == nil && r.LessThanOrEqual == nil {
		errE := errors.New("less than bound is required")
		errors.Details(errE)["range"] = r
		return false, errE
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

	// ES rejects non-finite (NaN or Inf) bound values.
	if math.IsNaN(lower) || math.IsInf(lower, 0) {
		errE := errors.New("lower bound is not a finite number")
		errors.Details(errE)["range"] = r
		return false, errE
	}
	if math.IsNaN(upper) || math.IsInf(upper, 0) {
		errE := errors.New("upper bound is not a finite number")
		errors.Details(errE)["range"] = r
		return false, errE
	}

	swapped := false
	switch {
	case lower < upper:
		// Normal case; nothing to do.
	case lower == upper:
		// ES accepts gte == lte (single-point range). Any other equal-bound
		// combination has at least one strict side and is rejected by ES.
		if r.GreaterThanOrEqual != nil && r.LessThanOrEqual != nil {
			return false, nil
		}
		errE := errors.New("equal bounds with at least one strict bound")
		errors.Details(errE)["range"] = r
		return false, errE
	default:
		// lower > upper: swap so the indexed range is well-formed.
		newR := RangeFloat{}
		if r.GreaterThan != nil {
			newR.LessThan = r.GreaterThan
		} else {
			newR.LessThanOrEqual = r.GreaterThanOrEqual
		}
		if r.LessThan != nil {
			newR.GreaterThan = r.LessThan
		} else {
			newR.GreaterThanOrEqual = r.LessThanOrEqual
		}
		*r = newR
		swapped = true
	}

	// Strict-strict adjacency: when both bounds are strict and within 1 ULP
	// of each other, ES rejects the range as empty.
	if r.GreaterThan != nil && r.LessThan != nil {
		if math.Nextafter(*r.GreaterThan, math.Inf(1)) > math.Nextafter(*r.LessThan, math.Inf(-1)) {
			errE := errors.New("strict bounds within one ULP of each other")
			errors.Details(errE)["range"] = r
			return swapped, errE
		}
	}

	return swapped, nil
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

// LinkClaim represents a claim with an IRI (Internationalized Resource Identifier) value.
type LinkClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	IRI         string                `json:"iri"`
}

// NestedReferenceClaim represents a nested reference claim.
type NestedReferenceClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	To          identifier.Identifier `json:"to"`
	ToDisplay   map[string]string     `json:"toDisplay"`
	ToNaming    map[string][]string   `json:"toNaming"`
}

// ReferenceClaim represents a claim that relates this document to another document.
//
// In addition, it supports a limited set of nested claims.
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

	// Nested claims.
	Reference []NestedReferenceClaim `json:"ref,omitempty"`
}

// HasClaim represents a claim with just a property.
//
// In addition, it supports a limited set of nested claims.
type HasClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`

	// Nested claims.
	Reference []NestedReferenceClaim `json:"ref,omitempty"`
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
