// Package shortcut implements the compact string encoding for identifiers and paths of identifiers that
// PeerDB uses in search shortcuts (SEARCH_SHORTCUT and FIELD_VALUES claims) and in embed configuration
// (EMBED_PROPERTY claims and the embed struct tag). The grammar is parsed here in full so the encoding
// stays identical everywhere; each caller only applies its own rules to the parsed result (how many
// segments a key or value may have, which literals are allowed where, which positions require an
// identifier).
//
// The separators, from coarsest to finest:
//
//   - "&" separates the entries of a list.
//   - "=" separates the key (left) from the value (right) within an entry, at its first occurrence.
//   - ":" separates the segments of a path.
//   - "," separates the base parts of a single identifier token.
//
// A single identifier token is either a "," list of non-empty base parts hashed together via
// identifier.From, or a 22-character base58 identifier. A token that is neither (a no-comma string that is
// not a valid identifier, for example "self" or "reverse") is a literal, left for the caller to interpret.
package shortcut

import (
	"slices"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

const (
	// EntrySeparator separates the entries of a list.
	EntrySeparator = "&"
	// KeyValueSeparator separates the key from the value within an entry.
	KeyValueSeparator = "="
	// PathSeparator separates the segments of a path.
	PathSeparator = ":"
	// PartSeparator separates the base parts of a single identifier token.
	PartSeparator = ","
)

// Reserved literal tokens that callers interpret (they are not identifiers).
const (
	// SelfValue is the value token a caller resolves to the containing document's own identifier.
	SelfValue = "self"
	// ReverseKey is the key token that scopes a search to documents referencing the value via any property.
	ReverseKey = "reverse"
	// MissingValue is the value token that selects a property's "missing" bucket (documents with no claim for it).
	MissingValue = "missing"
	// DirectValue is the leading value segment that marks a "direct" (most-specific) target: the following
	// segment, separated by PathSeparator, is the target identifier.
	DirectValue = "direct"
)

// Segment is one ":"-separated piece of an entry's key or value. Exactly one of its fields carries the
// piece: when it is an identifier token (a "," list of base parts hashed via identifier.From, or a
// 22-character base58 identifier) Path holds the resolved identifier as its single element and Literal is
// empty; otherwise it is a literal (a token that is not a valid identifier, for example "self" or
// "reverse") and Literal holds it while Path is nil.
type Segment struct {
	Path    []identifier.Identifier
	Literal string
}

// IsIdentifier reports whether the segment is an identifier token rather than a literal.
func (s Segment) IsIdentifier() bool {
	return len(s.Path) > 0
}

// Identifier returns the resolved identifier of an identifier segment. It must only be called when
// IsIdentifier reports true.
func (s Segment) Identifier() identifier.Identifier {
	return s.Path[0]
}

// Entry is one "&"-separated entry: a key and a value separated by "=", each a ":"-separated path of
// segments. Both Key and Value are always non-empty, since ParseEntry rejects a missing "=" or an empty
// side. Callers still police segment counts, allowed literals, and which positions require an identifier.
type Entry struct {
	Key   []Segment
	Value []Segment
}

// Parse parses a list of "&"-separated entries. It returns an error when an entry is malformed (a missing
// "=" or an empty side) or carries a malformed identifier token (a "," list with an empty part). Barewords
// and segment counts are not errors here; they are left for the caller to police.
func Parse(s string) ([]Entry, errors.E) {
	var entries []Entry
	for raw := range strings.SplitSeq(s, EntrySeparator) {
		entry, errE := ParseEntry(raw)
		if errE != nil {
			return nil, errE
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ParseEntry parses a single "key=value" entry into its key and value segments. A missing "=" or an empty
// side (an empty key or an empty value) is an error: an entry always has a non-empty key and value.
func ParseEntry(entry string) (Entry, errors.E) {
	key, value, _ := strings.Cut(entry, KeyValueSeparator)
	if key == "" || value == "" {
		errE := errors.New("entry must have a non-empty key and value separated by '='")
		errors.Details(errE)["entry"] = entry
		return Entry{}, errE
	}
	keySegments, errE := parseSide(key)
	if errE != nil {
		errors.Details(errE)["entry"] = entry
		return Entry{}, errE
	}
	valueSegments, errE := parseSide(value)
	if errE != nil {
		errors.Details(errE)["entry"] = entry
		return Entry{}, errE
	}
	return Entry{Key: keySegments, Value: valueSegments}, nil
}

// parseSide parses one non-empty side of an entry (its key or its value) into segments.
func parseSide(side string) ([]Segment, errors.E) {
	var segments []Segment
	for token := range strings.SplitSeq(side, PathSeparator) {
		segment, errE := parseSegment(token)
		if errE != nil {
			return nil, errE
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

// parseSegment resolves a single token into a Segment: an identifier when the token is a "," list of
// non-empty parts or a 22-character base58 identifier, otherwise a literal. A "," list with an empty part
// is a malformed identifier and so an error.
func parseSegment(token string) (Segment, errors.E) {
	if strings.Contains(token, PartSeparator) {
		parts := strings.Split(token, PartSeparator)
		if slices.Contains(parts, "") {
			errE := errors.New("empty identifier part")
			errors.Details(errE)["token"] = token
			return Segment{}, errE
		}
		return Segment{Path: []identifier.Identifier{identifier.From(parts...)}, Literal: ""}, nil
	}
	id, ok := maybeIdentifier(token)
	if ok {
		return Segment{Path: []identifier.Identifier{id}, Literal: ""}, nil
	}
	// A token that is not a valid identifier is a literal, left for the caller to interpret.
	return Segment{Path: nil, Literal: token}, nil
}

// maybeIdentifier resolves token as a 22-character base58 identifier, returning false when it is not one.
// It exists so that a non-identifier token becomes a literal rather than an error.
func maybeIdentifier(token string) (identifier.Identifier, bool) {
	id, errE := identifier.MaybeString(token)
	return id, errE == nil
}
