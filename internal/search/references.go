package search

import (
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// InverseRelation contains data about a relation claim from another document
// and the resolved inverse property to use for the synthetic reverse claim.
//
// When document A has a relation claim with property X pointing to document B,
// and property X has an inverse property Y (either from INVERSE_PROPERTY_OF on
// the property, or from INVERSE_PROPERTY on a class field), then the bridge
// records for document B an InverseRelation with Source=A, SourceProp=X,
// TargetProp=Y, and Target=B. Rows are maintained per visibility level and only
// from documents which are sources at the level (see Bridge.SourceCheck), so
// rendering needs no further filtering.
//
// Within one target document's rows, a relation is identified by its source
// document, claim ID, and target property. We validate that claim IDs are unique
// per source document but we do not validate that they are unique globally, so
// both the source and the claim identify the forward claim. The target property
// is part of the identity because the same claim can produce multiple inverse
// relations with different target properties (e.g., when multiple properties
// declare INVERSE_PROPERTY_OF the same property). The target is part of the
// identity as well: value-hierarchy expansion lands the same claim's relation on
// the direct target and each of its ancestors, one relation per target.
type InverseRelation struct {
	// Claim is the ID of the relation claim in the source document (A).
	Claim identifier.Identifier
	// Source is the ID of the source document (A) that has the forward relation claim.
	Source identifier.Identifier
	// TargetProp is the resolved inverse property ID (Y) to use for the synthetic
	// reverse claim on the target document (B). Resolved at creation time from either
	// field-level INVERSE_PROPERTY (takes precedence) or property-level INVERSE_PROPERTY_OF.
	TargetProp identifier.Identifier
	// SourceProp is the property ID of the forward relation claim in the source document (X).
	SourceProp identifier.Identifier
	// Target is the ID of the target document (B) that the relation points to.
	Target identifier.Identifier
	// Confidence is the confidence of the forward relation claim.
	Confidence document.Confidence
}

// Reference contains data about a reference claim from a source document to a target document, as
// maintained per visibility level in the bridge's references table. Every reference claim of a
// document present at a level, at any depth of nesting (claims below low confidence are skipped
// together with their whole subtree), produces one row per target it (transitively, across value
// hierarchies) references. This is deeper than the indexed entries reach (they flatten only top-level
// claims and their direct sub-claims). Claims whose properties resolve inverse properties additionally
// produce InverseRelation rows in the separate inverse-relations table.
//
// Within one target document's rows, a row is identified by its source document and claim ID: we
// validate that claim IDs are unique per source document but we do not validate that they are unique
// globally, so both fields identify the claim. The target is part of the identity as well:
// value-hierarchy expansion lands the same claim on the direct target and each of its ancestors, one
// row per target.
//
// The claim's property and confidence are not stored: no consumer reads them (confidence gates only
// row existence), and storing them would churn rows, and thus target re-renders, on edits which change
// nothing derived from these rows.
type Reference struct {
	// Claim is the ID of the reference claim in the source document.
	Claim identifier.Identifier
	// Source is the ID of the source document having the reference claim.
	Source identifier.Identifier
	// Target is the ID of the target document the claim (transitively) references.
	Target identifier.Identifier
	// IsSource records whether the source document is a source at the row's level (see
	// Bridge.SourceCheck): only rows with IsSource are counted by counts.references, while refresh
	// lookups (which documents reference a given one) use all rows.
	IsSource bool
}
