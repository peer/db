package store

import (
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// RFC3339Milli is the time format string for RFC3339 with millisecond precision.
const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamp which is represented in JSON with millisecond
// precision and not (Go default) nanosecond precision.
type Time time.Time

// MarshalJSON marshals Time to JSON with millisecond precision.
func (t Time) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(RFC3339Milli)+len(`""`))
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, RFC3339Milli)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON unmarshals Time from JSON with millisecond precision.
func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("Time.UnmarshalJSON: input is not a JSON string")
	}
	data = data[len(`"`) : len(data)-len(`"`)]
	tt, err := time.Parse(RFC3339Milli, string(data))
	*t = Time(tt)
	return errors.WithStack(err)
}

// InverseRelation contains data about a relation claim from another document.
// When document A has a relation claim with property X pointing to document B,
// then document B's metadata will contain an InverseRelation entry with
// Source=A and Target=B.
type InverseRelation struct {
	// Claim is the ID of the relation claim in the source document (A).
	Claim identifier.Identifier `json:"claim"`
	// Source is the ID of the source document (A) that has the forward relation claim.
	Source identifier.Identifier `json:"source"`
	// Prop is the property ID of the forward relation claim in the source document (X).
	Prop identifier.Identifier `json:"prop"`
	// Target is the ID of the target document (B) that the relation points to.
	Target identifier.Identifier `json:"-"`
	// Confidence is the confidence of the forward relation claim.
	Confidence document.Confidence `json:"confidence"`
}

// DocumentMetadata contains metadata about a document including its timestamp.
type DocumentMetadata struct {
	At Time `json:"at"`

	// InverseRelations contains inverse relation data for relation claims from other
	// documents that point to this document.
	InverseRelations []InverseRelation `json:"inverseRelations,omitempty"`
}

// inverseRelationKey identifies an inverse relation by its source document and claim ID.
// We validate that claim IDs are unique per source document but we do not validate that
// they are unique globally, so both fields are needed to cover all cases.
type inverseRelationKey struct {
	Source identifier.Identifier
	Claim  identifier.Identifier
}

// AddInverseRelations adds inverse relations to this metadata,
// if they do not already exist (comparison is done by (Source, Claim) pair).
func (m *DocumentMetadata) AddInverseRelations(relations []InverseRelation) {
	existing := make(map[inverseRelationKey]bool, len(m.InverseRelations))
	for _, ir := range m.InverseRelations {
		existing[inverseRelationKey{Source: ir.Source, Claim: ir.Claim}] = true
	}
	for _, ir := range relations {
		key := inverseRelationKey{Source: ir.Source, Claim: ir.Claim}
		if !existing[key] {
			m.InverseRelations = append(m.InverseRelations, ir)
			existing[key] = true
		}
	}
}

// RemoveInverseRelations removes specific inverse relations identified by their claim IDs.
// Only relations whose Claim field matches one of the provided relations'
// Claim fields are removed.
func (m *DocumentMetadata) RemoveInverseRelations(relations []InverseRelation) {
	if len(relations) == 0 || len(m.InverseRelations) == 0 {
		return
	}

	// Build a set of claim IDs to remove.
	toRemove := make(map[identifier.Identifier]bool, len(relations))
	for i := range relations {
		toRemove[relations[i].Claim] = true
	}

	kept := make([]InverseRelation, 0, len(m.InverseRelations))
	for i := range m.InverseRelations {
		if !toRemove[m.InverseRelations[i].Claim] {
			kept = append(kept, m.InverseRelations[i])
		}
	}

	if len(kept) == 0 {
		m.InverseRelations = nil
	} else {
		m.InverseRelations = kept
	}
}

// CarryOver sets system-managed fields in this metadata based on old metadata.
//
// This should be called on new metadata before committing a new version of a
// document to maintain fields managed by background processes (e.g., the bridge).
func (m *DocumentMetadata) CarryOver(old *DocumentMetadata) {
	if old == nil {
		return
	}
	m.InverseRelations = old.InverseRelations
}

// NoMetadata represents an empty metadata structure.
type NoMetadata struct{}
