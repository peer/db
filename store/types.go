package store

import (
	"time"

	"github.com/mohae/deepcopy"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// RFC3339Milli is the time format string for RFC3339 with millisecond precision.
const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamp which is represented in JSON with millisecond
// precision and not (Go default) nanosecond precision.
//
//nolint:recvcheck
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

// DeepCopy implements the github.com/mohae/deepcopy Interface so deepcopy.Copy copies a Time by value.
// Without it deepcopy reflects into the struct and, because Time is a named type over time.Time rather than
// the exact time.Time it special-cases, skips time.Time's unexported fields and zeroes the timestamp. A Time
// is a plain value, so returning it is a complete copy.
func (t Time) DeepCopy() any {
	return t
}

var _ deepcopy.Interface = Time{}

// InverseRelationKey identifies an inverse relation by its source document, claim ID,
// and target property. We validate that claim IDs are unique per source document but we
// do not validate that they are unique globally, so both source and claim fields are needed.
// TargetProp is included because the same source claim can produce multiple inverse relations
// with different target properties (e.g., when multiple properties declare INVERSE_PROPERTY_OF
// the same property).
type InverseRelationKey struct {
	// Claim is the ID of the relation claim in the source document (A).
	Claim identifier.Identifier `json:"claim"`
	// Source is the ID of the source document (A) that has the forward relation claim.
	Source identifier.Identifier `json:"source"`
	// TargetProp is the resolved inverse property ID (Y) to use for the synthetic
	// reverse claim on the target document (B). Resolved at creation time from either
	// field-level INVERSE_PROPERTY (takes precedence) or property-level INVERSE_PROPERTY_OF.
	TargetProp identifier.Identifier `json:"targetProp"`
}

// InverseRelation contains data about a relation claim from another document
// and the resolved inverse property to use for the synthetic reverse claim.
//
// When document A has a relation claim with property X pointing to document B,
// and property X has an inverse property Y (either from INVERSE_PROPERTY_OF on
// the property, or from INVERSE_PROPERTY on a class field), then document B's
// metadata will contain an InverseRelation with Source=A, SourceProp=X,
// TargetProp=Y, and Target=B.
type InverseRelation struct {
	InverseRelationKey

	// SourceProp is the property ID of the forward relation claim in the source document (X).
	SourceProp identifier.Identifier `json:"sourceProp"`
	// Target is the ID of the target document (B) that the relation points to.
	Target identifier.Identifier `json:"-"`
	// Confidence is the confidence of the forward relation claim.
	Confidence document.Confidence `json:"confidence"`
}

// DocumentMetadata contains metadata about a document including its timestamp.
type DocumentMetadata struct {
	At Time `json:"at"`

	// Users is the deduplicated, sorted-by-ID union of users who contributed
	// to this version: the user who began the edit session plus every user who
	// appended a change. The user who ended the session (committer) is NOT
	// included here; that user goes to CommitMetadata.User instead.
	Users []User `json:"users,omitempty"`

	// InverseRelations maps a visibility level name to the inverse relations visible at that level: inverse
	// relation data for relation claims from other documents that point to this document, as those source
	// documents are seen at that level. The sets are kept strictly separate so that indexing one level
	// never leaks a source not visible there.
	InverseRelations map[string][]InverseRelation `json:"inverseRelations,omitempty"`

	// Embedding maps each document that embeds claims from this document (a document with a reference claim to
	// this one on a field configured with EMBED_PROPERTY) to the source paths it embeds: each path is the
	// sequence of property IDs, within this document, that the embedding document copies (a single property for
	// a direct embed, or a property path for a nested one). It is maintained from the embedding side: a document
	// sets its own entry here when it is committed and embeds from this document, and removes it when it stops.
	// The paths are the union across visibility levels.
	Embedding map[identifier.Identifier][][]identifier.Identifier `json:"embedding,omitempty"`
}

// CommitMetadata contains metadata about a commit.
type CommitMetadata struct {
	Base []string `json:"base,omitempty"`

	// User is the user who invoked the End that produced this commit.
	// nil when the commit was made by an unauthenticated caller.
	User *User `json:"user,omitempty"`
}

// ChangesetID implements store.ChangesetID interface.
func (c *CommitMetadata) ChangesetID() identifier.Identifier {
	if len(c.Base) == 0 {
		panic(errors.New("base is empty"))
	}
	return identifier.From(c.Base...)
}

// AddInverseRelations adds inverse relations to the given visibility level, if they do not already exist at
// that level (comparison is done by InverseRelationKey).
func (m *DocumentMetadata) AddInverseRelations(level string, relations []InverseRelation) {
	if len(relations) == 0 {
		return
	}
	current := m.InverseRelations[level]
	existing := make(map[InverseRelationKey]bool, len(current))
	for _, ir := range current {
		existing[ir.InverseRelationKey] = true
	}
	for _, ir := range relations {
		if !existing[ir.InverseRelationKey] {
			current = append(current, ir)
			existing[ir.InverseRelationKey] = true
		}
	}
	if m.InverseRelations == nil {
		m.InverseRelations = map[string][]InverseRelation{}
	}
	m.InverseRelations[level] = current
}

// RemoveInverseRelations removes from the given visibility level the inverse relations identified by their
// claim IDs. Only relations whose Claim field matches one of the provided relations' Claim fields are removed.
// When a level's set becomes empty its key is dropped, and an empty map is reset to nil.
func (m *DocumentMetadata) RemoveInverseRelations(level string, relations []InverseRelation) {
	current := m.InverseRelations[level]
	if len(relations) == 0 || len(current) == 0 {
		return
	}

	// Build a set of claim IDs to remove.
	toRemove := make(map[identifier.Identifier]bool, len(relations))
	for i := range relations {
		toRemove[relations[i].Claim] = true
	}

	kept := make([]InverseRelation, 0, len(current))
	for i := range current {
		if !toRemove[current[i].Claim] {
			kept = append(kept, current[i])
		}
	}

	if len(kept) == 0 {
		delete(m.InverseRelations, level)
		if len(m.InverseRelations) == 0 {
			m.InverseRelations = nil
		}
	} else {
		m.InverseRelations[level] = kept
	}
}

// SetEmbedding records that the document with the given ID embeds claims from this document using the given
// source paths (the union across visibility levels), replacing any existing entry for it.
func (m *DocumentMetadata) SetEmbedding(id identifier.Identifier, paths [][]identifier.Identifier) {
	if m.Embedding == nil {
		m.Embedding = map[identifier.Identifier][][]identifier.Identifier{}
	}
	m.Embedding[id] = paths
}

// RemoveEmbedding removes the entry for the document with the given ID from the set of documents that embed
// claims from this document. When the set becomes empty it is reset to nil.
func (m *DocumentMetadata) RemoveEmbedding(id identifier.Identifier) {
	delete(m.Embedding, id)
	if len(m.Embedding) == 0 {
		m.Embedding = nil
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
	m.Embedding = old.Embedding
}

// NoMetadata represents an empty metadata structure.
type NoMetadata struct{}
