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
// DisplaySort holds, per supported language, only the document's primary rendered display label
// (no ancestor labels), as a single keyword used to sort results by the label shown to the user.
//
// Time holds the document's earliest time: the lowest time value across all of
// its time claims (top-level and sub-claims). For a point timestamp that is the
// timestamp; for an interval it is the earliest bound.
//
// LastUpdated holds the time (seconds since the Unix epoch) when the document was last updated,
// taken from the document's metadata At timestamp (not when it was last indexed).
//
// Counts holds the document's count metrics, nested under "counts".
type Document struct {
	ID identifier.Identifier `json:"id"`

	Display map[string][]string `json:"display,omitempty"`

	DisplaySort map[string]string `json:"displaySort,omitempty"`

	Text map[string][]string `json:"text,omitempty"`

	Time *float64 `json:"time,omitempty"`

	LastUpdated *float64 `json:"lastUpdated,omitempty"`

	Counts Counts `json:"counts,omitzero"`

	Claims ClaimTypes `json:"claims,omitzero"`

	// ReadableByRoles lists the roles whose role grants allow reading this document (including the
	// reserved everyone role under its empty name), evaluated at indexing time against the document's
	// claims exactly like the read path evaluates them (see auth.RoleGrants.AllowsDocument). The default
	// search query filter matches the caller's roles against it (see ReadAccessQuery). Role grants are
	// baked into the index through this field, so changing them requires a full reindex. Omitted when
	// the converter has no roles configured.
	ReadableByRoles []string `json:"readableByRoles,omitempty"`

	// ReadableByUsers lists the users the document's own permission claims grant the read action
	// (see auth.PermissionClaimGrants), evaluated at indexing time. The default search query filter
	// matches the caller's subject against it (see ReadAccessQuery). Together with ReadableByRoles it
	// materializes the two arms of auth.HasDocumentPermission for the read action; other actions are
	// not indexed, search never filters by them.
	ReadableByUsers []string `json:"readableByUsers,omitempty"`
}

// Counts holds a document's count metrics, used to boost search ranking.
//
// References is the number of stored reference claims in other documents referencing this document,
// computed at index time from the bridge-maintained references table and kept current by re-indexing a
// document when rows pointing at it change. It covers claims of documents which are sources at the
// level, expanded across value hierarchies like the forward index; synthetic inverse claims and
// embedded copies in entries do not count.
//
// Claims is the total number of claims the document has, counted recursively including sub-claims,
// with only claims at or above low confidence counting (a low-confidence claim is skipped together
// with its whole subtree, matching which claims produce reference rows).
//
// Score is Claims plus References, used to boost search ranking. Ignored documents
// (which have no References) get just their Claims.
type Counts struct {
	References *int `json:"references,omitempty"`

	Claims *int `json:"claims,omitempty"`

	Score *int `json:"score,omitempty"`
}

// ClaimType values for RelClaim records, discriminating the four target-or-nothing
// claim types sharing the claims.rel collection.
const (
	ClaimTypeRef     = "ref"
	ClaimTypeHas     = "has"
	ClaimTypeNone    = "none"
	ClaimTypeUnknown = "unknown"
)

// ClaimTypes organizes claims by their ElasticSearch collection. Each collection is a
// nested field under "claims" (or, for sub-claims, under a parent record's "sub" field).
//
// Rel holds the four target-or-nothing claim types (ref, has, none, unknown) in one
// claimType-discriminated collection, so a facet's value and special-value buckets (which must
// count against each other) live in a single nested context and per-document counts are
// exact without cross-collection deduplication.
//
// Amount and Time hold numeric and temporal claims; interval source claims map into them
// as a range over their bounds, while a point claim becomes a range whose endpoints are
// its precision window.
//
// Identifier, String, HTML and Link are the text-only collections: they surface no facets
// (and do not block a facet's missing bucket) but are indexed so code can make structured
// per-property queries over them.
type ClaimTypes struct {
	Rel        RelClaims        `json:"rel,omitempty"`
	Amount     AmountClaims     `json:"amount,omitempty"`
	Time       TimeClaims       `json:"time,omitempty"`
	Identifier IdentifierClaims `json:"id,omitempty"`
	String     StringClaims     `json:"string,omitempty"`
	HTML       HTMLClaims       `json:"html,omitempty"`
	Link       LinkClaims       `json:"link,omitempty"`
}

// Size returns the total number of records across all collections.
func (c *ClaimTypes) Size() int {
	if c == nil {
		return 0
	}
	return len(c.Rel) + len(c.Amount) + len(c.Time) + len(c.Identifier) + len(c.String) + len(c.HTML) + len(c.Link)
}

type (
	// RelClaims is a slice of RelClaim.
	RelClaims = []RelClaim
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
)

// Sub containers hold a record's sub-claims as the same ClaimTypes collections, nested one
// level inside the parent record (ElasticSearch multi-level nested). Only top-level records
// carry a Sub container; records inside a Sub container have Sub nil (the mapping defines a
// single sub level, and deeper sub-claims are not indexed, though they still count toward
// counts.claims and the document time). Because a sub record sits structurally under its
// parent record, queries correlate sub conditions on the same parent claim by nesting inner
// nested queries inside the parent's nested query, and no parent identity fields are needed
// on sub records.

// IdentifierClaim represents a claim with a string identifier value.
type IdentifierClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`
	Value       string                `json:"value"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}

// StringClaim represents a claim with a plain string value for a given language.
type StringClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`

	// String maps each language the claim resolves to (its IN_LANGUAGE sub-claims, or
	// detected language) to the claim's value. Every entry holds the same value.
	String map[string]string `json:"string"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}

// HTMLClaim represents a claim with HTML content, indexed as plain text. The HTML is
// converted to text in Go (parsed, then stripDoc) before indexing, per language, so each
// entry holds the plain-text rendering of the claim's HTML for that language.
type HTMLClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`

	// HTML maps each language the claim resolves to (its IN_LANGUAGE sub-claims, or detected
	// language) to the plain-text rendering of the claim's HTML for that language.
	HTML map[string]string `json:"html"`

	Sub *ClaimTypes `json:"sub,omitempty"`
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
	PropSortKey map[string]string      `json:"propSortKey,omitempty"`
	Unit        *identifier.Identifier `json:"unit"`
	Range       RangeFloat             `json:"range"`
	From        *float64               `json:"from,omitempty"`
	FromDisplay string                 `json:"fromDisplay,omitempty"`
	To          *float64               `json:"to,omitempty"`
	ToDisplay   string                 `json:"toDisplay,omitempty"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}

// TimeClaim represents a claim for time.
//
// For search, we index times as both ranges and boundaries.
// Times are stored as float64 seconds since Unix epoch.
type TimeClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`
	Range       RangeFloat            `json:"range"`
	From        *float64              `json:"from,omitempty"`
	FromDisplay string                `json:"fromDisplay,omitempty"`
	To          *float64              `json:"to,omitempty"`
	ToDisplay   string                `json:"toDisplay,omitempty"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}

// LinkClaim represents a claim with an IRI (Internationalized Resource Identifier) value.
type LinkClaim struct {
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`
	IRI         string                `json:"iri"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}

// RelClaim represents a target-or-nothing statement about a property: a reference to
// another document (ClaimType "ref"), a bare property assertion ("has"), an explicit
// statement that no value exists ("none"), or a statement that the value exists but is
// unknown ("unknown"). The To* fields are set only on ref records; has, none, and
// unknown records carry only the property fields and the discriminator.
type RelClaim struct {
	ClaimType   string                `json:"claimType"`
	Prop        identifier.Identifier `json:"prop"`
	PropDisplay map[string]string     `json:"propDisplay"`
	PropNaming  map[string][]string   `json:"propNaming"`
	PropSortKey map[string]string     `json:"propSortKey,omitempty"`

	To        *identifier.Identifier `json:"to,omitempty"`
	ToDisplay map[string]string      `json:"toDisplay,omitempty"`
	ToNaming  map[string][]string    `json:"toNaming,omitempty"`
	// ToSortKey is, per language, the value's own display label as a sort_key_normalizer keyword, so facet
	// values can be sorted by their display label (independent of the hierarchy ToPathSortKey).
	ToSortKey map[string]string `json:"toSortKey,omitempty"`
	// ToPath contains ID-based hierarchy paths from root to the target document.
	// Each path is prefixed with the hierarchy property ID and ":" separator
	// (e.g., "<property_ID>:<root_ID>/<parent_ID>/<this_ID>"), followed by
	// ancestor IDs joined by "/". Multiple paths exist when the target has
	// multiple parents in a hierarchy or participates in multiple hierarchies.
	ToPath []string `json:"toPath,omitempty"`
	// ToParent holds the immediate-parent value id for each of ToPath's chains (the segment before the last).
	// It is multi-valued because a value can have several parents under multiple inheritance, and absent for a
	// root value. It lets a terms(toParent) aggregation group a value's distinct children.
	ToParent []string `json:"toParent,omitempty"`
	// ToDisplayPath contains per-language display hierarchy paths from root to the target document. Each
	// path is a string of display labels joined by null bytes. It is not indexed (json:"-"): it is kept in
	// memory only to fold the hierarchy labels into the searchable text (addDisplayPathLabels) and to build
	// ToPathSortKey. Sorting uses ToPathSortKey instead.
	ToDisplayPath map[string][]string `json:"-"`
	// ToPathSortKey contains, per language, combination of ToDisplayPath and ToPath, one sort key per hierarchy
	// path: the display path (labels joined by null bytes), then SortKeySeparator, then the hex-encoded ToPath entry.
	// The label half folds under sort_key_normalizer for case/diacritic-insensitive ordering, while the
	// hex id half (lowercase, fold-stable) preserves the exact ToPath so grouping can recover it. It is the
	// single key for ref-column sorting and for grouping: ordering by it sorts by label with the id as a
	// tiebreaker, so distinct values that share a display label stay distinct.
	ToPathSortKey map[string][]string `json:"toPathSortKey,omitempty"`
	// IsLeaf is true when the target is a most-specific value: the containing scope (the whole
	// document for top-level records, the parent record's Sub container for sub records)
	// references the value but none of its narrower values (its descendants in the value
	// hierarchy) for the same property. It lets the reference filter count and select documents
	// that are exactly this value, with none of its narrower values ("direct").
	//
	// IsLeaf is a property of the whole scope (computed across all of its ref records for this
	// property), so a record whose To is its own claim's stated value can still have IsLeaf
	// false: when the scope states both a parent and a child directly, the parent is not
	// most-specific because the child (a narrower value) is also referenced.
	IsLeaf bool `json:"isLeaf,omitempty"`

	Sub *ClaimTypes `json:"sub,omitempty"`
}
