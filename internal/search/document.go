package search

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// SupportedLanguages is a set of supported languages in ElasticSearch mapping.
var SupportedLanguages = map[string]bool{ //nolint:gochecknoglobals
	"en": true,
	"sl": true,
	"pt": true,
}

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
	Reference  ReferenceClaims  `json:"ref,omitempty"`
	Relation   RelationClaims   `json:"rel,omitempty"`
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
	// ReferenceClaims is a slice of ReferenceClaim.
	ReferenceClaims = []ReferenceClaim
	// RelationClaims is a slice of RelationClaim.
	RelationClaims = []RelationClaim
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
	PropDisplay map[string][]string   `json:"propDisplay"`
	Value       string                `json:"value"`
}

// StringClaim represents a claim with a plain string value for a given language.
type StringClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`

	// Map contains exactly one value.
	String map[string]string `json:"string"`
}

// HTMLClaim represents a claim with HTML text content for a given language.
type HTMLClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`

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

// Validate checks that the range is valid.
func (r RangeFloat) Validate() errors.E {
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
	return nil
}

// AmountClaim represents a claim for numeric amount and unit.
//
// For search, we index amounts as both ranges and boundaries.
type AmountClaim struct {
	Prop        identifier.Identifier  `json:"prop"`
	PropDisplay map[string][]string    `json:"propDisplay"`
	Unit        *identifier.Identifier `json:"unit"`
	Range       RangeFloat             `json:"range"`
	From        *float64               `json:"from,omitempty"`
	FromDisplay string                 `json:"fromDisplay,omitempty"`
	To          *float64               `json:"to,omitempty"`
	ToDisplay   string                 `json:"toDisplay,omitempty"`
}

// RangeInt represents a numeric range.
//
// Exactly one of GreaterThan or GreaterThanOrEqual must be set.
// Exactly one of LessThan or LessThanOrEqual must be set.
type RangeInt struct {
	GreaterThan        *int64 `json:"gt,omitempty"`
	GreaterThanOrEqual *int64 `json:"gte,omitempty"`
	LessThan           *int64 `json:"lt,omitempty"`
	LessThanOrEqual    *int64 `json:"lte,omitempty"`
}

// Validate checks that the range is valid.
func (r RangeInt) Validate() errors.E {
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
	return nil
}

// TimeClaim represents a claim for timestamp.
//
// For search, we index timestamps as both ranges and boundaries.
type TimeClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`
	Range       RangeInt              `json:"range"`
	From        *int64                `json:"from,omitempty"`
	FromDisplay string                `json:"fromDisplay,omitempty"`
	To          *int64                `json:"to,omitempty"`
	ToDisplay   string                `json:"toDisplay,omitempty"`
}

// ReferenceClaim represents a claim with an IRI (Internationalized Resource Identifier) value.
type ReferenceClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`
	IRI         string                `json:"iri"`
}

// RelationClaim represents a claim that relates this document to another document.
//
// In addition, it supports a limited set of nested claims.
type RelationClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`
	To          identifier.Identifier `json:"to"`
	ToDisplay   map[string][]string   `json:"toDisplay"`

	// Nested claims.
	Relation RelationClaims `json:"rel,omitempty"`
}

// HasClaim represents a claim with just a property.
//
// In addition, it supports a limited set of nested claims.
type HasClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`

	// Nested claims.
	Relation RelationClaims `json:"rel,omitempty"`
}

// NoneClaim represents a claim that explicitly states no value exists for a property.
type NoneClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`
}

// UnknownClaim represents a claim where the value for a property is known to exist but is unknown.
type UnknownClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string][]string   `json:"propDisplay"`
}
