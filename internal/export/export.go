// Package export provides functionality for exporting PeerDB documents to CSV or JSON.
package export

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

const (
	// NoneValue is the sentinel string used for NoneClaim values.
	NoneValue = "__NONE__"
	// UnknownValue is the sentinel string used for UnknownClaim values.
	UnknownValue = "__UNKNOWN__"
	// HasColumn is the column name for simple HasClaim values.
	HasColumn = "__HAS__"
)

// GetDocFunc is a function that retrieves the latest version of a document by ID.
type GetDocFunc func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E)

// Config configures the export operation.
type Config struct {
	Format     string
	InstanceOf []string
	Properties []string
}

// PathSegment represents one segment in a property path.
type PathSegment struct {
	// A nil ID means wildcard (* or **) for that segment.
	ID *identifier.Identifier
	// Recursive is true for ** (matches at any depth), false for * (single level).
	Recursive bool
}

// Matches returns true if this segment matches the given property ID.
// Both * and ** segments match any property ID.
func (s PathSegment) Matches(propID identifier.Identifier) bool {
	return s.ID == nil || *s.ID == propID
}

// PropertySpec represents a parsed --property flag value as a path of segments.
type PropertySpec struct {
	Segments []PathSegment
}

// MatchResult describes how a property matched at a given depth.
type MatchResult struct {
	// Matched is true if the property should be included at this level.
	Matched bool
	// ChildSpecs are the specs to pass to child claims.
	// Empty slice means "no children should be included".
	// Non-empty slice means "filter children using these specs".
	ChildSpecs []PropertySpec
}

// MatchAtDepth checks whether propID matches any spec at the current level,
// and computes the specs to pass to the next depth.
func MatchAtDepth(propID identifier.Identifier, specs []PropertySpec) MatchResult {
	childSpecs := make([]PropertySpec, 0, len(specs))
	matched := false

	for _, spec := range specs {
		first := spec.Segments[0]

		if first.Recursive {
			// ** segment: persists for deeper levels.
			// Keep ** active for children so traversal continues.
			childSpecs = append(childSpecs, spec)

			remaining := spec.Segments[1:]
			if len(remaining) == 0 {
				// Bare **: matches everything at all depths.
				matched = true
			} else {
				// **.X: try to match X at this level.
				// Skip adjacent ** segments.
				for len(remaining) > 0 && remaining[0].Recursive {
					remaining = remaining[1:]
				}
				if len(remaining) > 0 && remaining[0].Matches(propID) {
					// X matches here: this property is included.
					matched = true
					afterNext := remaining[1:]
					if len(afterNext) > 0 {
						childSpecs = append(childSpecs, PropertySpec{Segments: afterNext})
					}
				}
			}
			continue
		}

		if !first.Matches(propID) {
			continue
		}

		matched = true
		remaining := spec.Segments[1:]

		if len(remaining) == 0 {
			// Terminal match: include this prop but no children.
			continue
		}

		// More segments remain: build child spec from remaining.
		childSpecs = append(childSpecs, PropertySpec{Segments: remaining})
	}

	if !matched && len(childSpecs) == 0 {
		return MatchResult{Matched: false, ChildSpecs: nil}
	}

	return MatchResult{Matched: matched, ChildSpecs: childSpecs}
}

// NameCache caches property display names to avoid repeated lookups.
type NameCache struct {
	Names  map[identifier.Identifier]string
	GetDoc GetDocFunc
}

// NewNameCache creates a new NameCache with the given document retrieval function.
func NewNameCache(getDoc GetDocFunc) *NameCache {
	return &NameCache{
		Names:  make(map[identifier.Identifier]string),
		GetDoc: getDoc,
	}
}

// DisplayName returns the display name for a property ID.
// Priority: mnemonic > english NAME > ID string.
func (c *NameCache) DisplayName(ctx context.Context, propID identifier.Identifier) string {
	if name, ok := c.Names[propID]; ok {
		return name
	}
	name := ResolveDisplayName(ctx, propID, c.GetDoc)
	c.Names[propID] = name
	return name
}

// Preload populates the cache from a slice of property documents.
func (c *NameCache) Preload(properties []*document.D) {
	for _, prop := range properties {
		if _, ok := c.Names[prop.ID]; ok {
			continue
		}
		name := DisplayNameFromDoc(prop)
		c.Names[prop.ID] = name
	}
}

// ResolveDisplayName fetches a document and determines its display name.
func ResolveDisplayName(ctx context.Context, propID identifier.Identifier, getDoc GetDocFunc) string {
	doc, errE := getDoc(ctx, propID)
	if errE != nil || doc == nil {
		return propID.String()
	}
	return DisplayNameFromDoc(doc)
}

// DisplayNameFromDoc extracts the display name from a document.
// Priority: MNEMONIC > NAME > ID string.
func DisplayNameFromDoc(doc *document.D) string {
	// Try MNEMONIC first (stored as StringClaim).
	mnemonic := document.GetBestClaimOfType[document.StringClaim](doc, internalCore.MnemonicPropID)
	if mnemonic != nil && mnemonic.String != "" {
		return mnemonic.String
	}
	// Try NAME.
	name := document.GetBestClaimOfType[document.StringClaim](doc, internalCore.NamePropID)
	if name != nil && name.String != "" {
		return name.String
	}
	return doc.ID.String()
}

// BuildMnemonicMap builds a mnemonic→ID map from property documents.
func BuildMnemonicMap(properties []*document.D) map[string]identifier.Identifier {
	mnemonics := make(map[string]identifier.Identifier, len(properties))
	for _, prop := range properties {
		mnemonic := document.GetBestClaimOfType[document.StringClaim](prop, internalCore.MnemonicPropID)
		if mnemonic != nil && mnemonic.String != "" {
			mnemonics[mnemonic.String] = prop.ID
		}
	}
	return mnemonics
}

// ResolveID resolves a mnemonic or ID string to an identifier.
// It first tries to find a matching mnemonic, then tries to parse as an ID.
func ResolveID(value string, mnemonics map[string]identifier.Identifier) (identifier.Identifier, errors.E) {
	// Try mnemonic first.
	if id, ok := mnemonics[value]; ok {
		return id, nil
	}
	// Try parsing as ID.
	id, errE := identifier.MaybeString(value)
	if errE != nil {
		return identifier.Identifier{}, errE
	}
	return id, nil
}

// ResolveIDs resolves a slice of mnemonic-or-ID strings to identifiers.
func ResolveIDs(values []string, mnemonics map[string]identifier.Identifier) ([]identifier.Identifier, errors.E) {
	result := make([]identifier.Identifier, 0, len(values))
	for _, v := range values {
		id, errE := ResolveID(v, mnemonics)
		if errE != nil {
			return nil, errE
		}
		result = append(result, id)
	}
	return result, nil
}

// ParsePropertySpecs parses --property flag values into PropertySpec structs.
func ParsePropertySpecs(props []string, mnemonics map[string]identifier.Identifier) ([]PropertySpec, errors.E) {
	if len(props) == 0 {
		// Default: same as **.
		return []PropertySpec{{Segments: []PathSegment{{ID: nil, Recursive: true}}}}, nil
	}
	result := make([]PropertySpec, 0, len(props))
	for _, p := range props {
		spec, errE := ParsePropertySpec(p, mnemonics)
		if errE != nil {
			return nil, errE
		}
		result = append(result, spec)
	}
	return result, nil
}

// ParsePropertySpec parses a single property spec string.
// Supports * (single-level wildcard) and ** (recursive wildcard).
func ParsePropertySpec(s string, mnemonics map[string]identifier.Identifier) (PropertySpec, errors.E) {
	parts := strings.Split(s, ".")
	segments := make([]PathSegment, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "**":
			segments = append(segments, PathSegment{ID: nil, Recursive: true})
		case "*":
			segments = append(segments, PathSegment{ID: nil, Recursive: false})
		default:
			id, errE := ResolveID(part, mnemonics)
			if errE != nil {
				return PropertySpec{Segments: nil}, errE
			}
			segments = append(segments, PathSegment{ID: &id, Recursive: false})
		}
	}
	return PropertySpec{Segments: segments}, nil
}

// ClaimValue extracts the string representation from a claim.
func ClaimValue(claim document.Claim) string {
	switch c := claim.(type) {
	case *document.IdentifierClaim:
		return c.Value
	case *document.StringClaim:
		return c.String
	case *document.HTMLClaim:
		return c.HTML
	case *document.AmountClaim:
		return c.Amount.String()
	case *document.AmountIntervalClaim:
		return FormatAmountInterval(c)
	case *document.TimeClaim:
		return string(c.Time)
	case *document.TimeIntervalClaim:
		return FormatTimeInterval(c)
	case *document.LinkClaim:
		return c.IRI
	case *document.ReferenceClaim:
		return c.To.ID.String()
	case *document.NoneClaim:
		return NoneValue
	case *document.UnknownClaim:
		return UnknownValue
	default:
		errE := errors.New("claim type not supported")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", claim)
		panic(errE)
	}
}

// FormatAmountInterval formats an AmountIntervalClaim as an interval, e.g. "[1, 10)".
func FormatAmountInterval(c *document.AmountIntervalClaim) string {
	left := "["
	if c.FromIsOpen {
		left = "("
	}
	right := "]"
	if c.ToIsOpen {
		right = ")"
	}
	from := FormatAmountBound(c.From, c.FromIsUnknown, c.FromIsNone)
	to := FormatAmountBound(c.To, c.ToIsUnknown, c.ToIsNone)
	return left + from + ", " + to + right
}

// FormatAmountBound formats a single bound value of an amount interval.
func FormatAmountBound(val *document.Amount, isUnknown, isNone bool) string {
	if isUnknown {
		return UnknownValue
	}
	if isNone {
		return NoneValue
	}
	if val != nil {
		return val.String()
	}
	return ""
}

// FormatTimeInterval formats a TimeIntervalClaim as an interval, e.g. "[2020-01-01, 2025-01-01)".
func FormatTimeInterval(c *document.TimeIntervalClaim) string {
	left := "["
	if c.FromIsOpen {
		left = "("
	}
	right := "]"
	if c.ToIsOpen {
		right = ")"
	}
	from := FormatTimeBound(c.From, c.FromIsUnknown, c.FromIsNone)
	to := FormatTimeBound(c.To, c.ToIsUnknown, c.ToIsNone)
	return left + from + ", " + to + right
}

// FormatTimeBound formats a single bound value of a time interval.
func FormatTimeBound(val *document.Time, isUnknown, isNone bool) string {
	if isUnknown {
		return UnknownValue
	}
	if isNone {
		return NoneValue
	}
	if val != nil {
		return string(*val)
	}
	return ""
}

// Export exports documents from the given ES index to the writer.
func Export(ctx context.Context, w io.Writer, esClient *elasticsearch.TypedClient, index string, getDoc GetDocFunc, config Config) errors.E {
	// Step 1: Fetch property documents to build mnemonic map.
	propIDs, errE := internalSearch.FetchDocumentIDs(ctx, esClient, index, []identifier.Identifier{internalCore.PropertyClassID})
	if errE != nil {
		return errE
	}

	properties := make([]*document.D, 0, len(propIDs))
	for _, id := range propIDs {
		doc, errE := getDoc(ctx, id)
		if errE != nil {
			return errE
		}
		if doc != nil {
			properties = append(properties, doc)
		}
	}

	mnemonics := BuildMnemonicMap(properties)

	// Step 2: Resolve INSTANCE_OF class IDs.
	classIDs, errE := ResolveIDs(config.InstanceOf, mnemonics)
	if errE != nil {
		return errE
	}

	// Step 3: Parse property specs.
	specs, errE := ParsePropertySpecs(config.Properties, mnemonics)
	if errE != nil {
		return errE
	}

	// Step 4: Fetch document IDs to export.
	docIDs, errE := internalSearch.FetchDocumentIDs(ctx, esClient, index, classIDs)
	if errE != nil {
		return errE
	}

	// Step 5: Build display name cache.
	names := NewNameCache(getDoc)
	names.Preload(properties)

	// Step 6: Write output.
	switch config.Format {
	case "csv":
		return CSV(ctx, w, docIDs, specs, names, getDoc)
	case "json":
		return JSON(ctx, w, docIDs, specs, names, getDoc)
	default:
		errE := errors.New("unsupported format")
		errors.Details(errE)["format"] = config.Format
		return errE
	}
}
