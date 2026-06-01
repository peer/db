package search

import (
	"bytes"
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"golang.org/x/net/html"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/store"
)

// inlineHTMLTags is the set of HTML tag names that do NOT produce a visible
// line/block break in rendered text. Text fragments separated only by these
// tags are concatenated directly when extracted to plain text. Every other
// tag (including unknown ones) inserts a single space between adjacent text
// fragments. The set tracks the inline elements the document sanitizer
// allows (document.SanitizeHTML).
var inlineHTMLTags = map[string]bool{ //nolint:gochecknoglobals
	"a":      true,
	"b":      true,
	"i":      true,
	"img":    true,
	"strike": true,
	"tt":     true,
	"u":      true,
}

// noBlockSpaceLanguages is the set of language codes whose writing system does
// not use spaces between words/blocks (CJK, Thai, Lao, ...). For these
// languages stripHTML concatenates text across non-inline tag boundaries
// instead of inserting a space, because a stray ASCII space would split a
// token that the language analyzer treats as a single run. Inline tags behave
// the same regardless of language.
var noBlockSpaceLanguages = map[string]bool{} //nolint:gochecknoglobals

// isHTMLWhitespace reports whether r is one of the ASCII whitespace characters
// the HTML spec treats as collapsible whitespace: SPACE, TAB, LF, FF, CR.
// Notably this excludes vertical tab (U+000B) and all non-ASCII whitespace
// (NBSP, U+2028, U+2029, ...), those are regular characters per the spec
// and must not be trimmed or collapsed.
func isHTMLWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

// stripHTML extracts plain text from HTML, inserting a single space between
// text fragments wherever a separator is implied and concatenating them
// directly otherwise. Separators are: any non-inline tag, any whitespace-only
// text token between tags, or leading/trailing whitespace within a text
// fragment. Only the known inline tags do not insert a separator on their own.
// Unknown tags default to inserting a space. Non-text tokens (Comment/Doctype)
// are dropped. Returns "" for input that contains no text.
//
// lang is the language code the resulting text will be indexed under. for
// languages listed in noBlockSpaceLanguages, non-inline tag boundaries do not
// insert a space (their scripts do not use word spaces).
func stripHTML(s, lang string) string {
	insertBlockSpace := !noBlockSpaceLanguages[lang]
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var buf bytes.Buffer
	needSpace := false
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			return string(bytes.TrimFunc(buf.Bytes(), isHTMLWhitespace))
		}
		switch tt { //nolint:exhaustive
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			if insertBlockSpace && !inlineHTMLTags[string(name)] {
				needSpace = true
			}
		case html.TextToken:
			raw := tokenizer.Text()
			if len(raw) == 0 {
				continue
			}
			text := bytes.TrimFunc(raw, isHTMLWhitespace)
			if len(text) == 0 {
				// Whitespace-only text between tags signals a separator
				// without emitting anything.
				needSpace = true
				continue
			}
			// HTML whitespace is single-byte ASCII, so testing the boundary
			// bytes is sufficient. For multi-byte UTF-8 sequences the lead /
			// trail byte is in 0xC0-0xFF or 0x80-0xBF, neither overlapping
			// with the whitespace set, so rune(b) gives the right answer.
			hasLeading := isHTMLWhitespace(rune(raw[0]))
			hasTrailing := isHTMLWhitespace(rune(raw[len(raw)-1]))
			if buf.Len() > 0 && (needSpace || hasLeading) {
				buf.WriteByte(' ')
			}
			buf.Write(text)
			needSpace = hasTrailing
		}
	}
}

type displayStrings struct {
	Display map[string]string
	Naming  map[string][]string
}

// hierarchyPathSeparator is the null byte used as separator in display hierarchy paths.
// It sorts before all printable characters, ensuring correct hierarchical ordering.
const hierarchyPathSeparator = "\x00"

// documentInfo holds information about a document: display strings,
// transitive ancestors, and hierarchy paths for each value hierarchy type.
type documentInfo struct {
	Display displayStrings
	// Ancestors maps a hierarchy property ID (e.g., SUBCLASS_OF) to transitive ancestor IDs.
	Ancestors map[identifier.Identifier][]identifier.Identifier
	// IDPaths maps a hierarchy property ID to ID-based hierarchy paths from root to this document.
	// Each path is a string of IDs joined by "/".
	IDPaths map[identifier.Identifier][]string
	// DisplayPaths maps a hierarchy property ID to per-language display hierarchy paths
	// from root to this document. Each path is a string of display labels joined by null bytes.
	DisplayPaths map[identifier.Identifier]map[string][]string
}

// CollectHierarchyPaths collects all hierarchy paths, combining paths from all
// value hierarchy types into single slices. ID paths are prefixed with the hierarchy
// property ID and ":" separator (e.g., "<SUBCLASS_OF_ID>:<root_ID>/<parent_ID>/<this_ID>")
// to identify which hierarchy each path belongs to.
func (d documentInfo) CollectHierarchyPaths() ([]string, map[string][]string) {
	var toPath []string
	for hierProp, paths := range d.IDPaths {
		prefix := hierProp.String() + ":"
		for _, p := range paths {
			toPath = append(toPath, prefix+p)
		}
	}
	var toDisplayPath map[string][]string
	for _, dpaths := range d.DisplayPaths {
		for lang, paths := range dpaths {
			if toDisplayPath == nil {
				toDisplayPath = map[string][]string{}
			}
			toDisplayPath[lang] = append(toDisplayPath[lang], paths...)
		}
	}
	return toPath, toDisplayPath
}

// fieldInverseKey identifies a position within a class's field hierarchy
// for field-level inverse property lookup. Path is the encoded sequence of
// parent HasClaim property IDs and SourceProp is the property at this position.
type fieldInverseKey struct {
	// Path is the encoded path of parent HasClaim property IDs leading to this
	// field position, joined by "/". Empty string for top-level fields.
	Path string
	// SourceProp is the property ID of the claim at this field position.
	SourceProp identifier.Identifier
}

// Converter holds preprocessed data for converting document.D to search Document.
type Converter struct {
	// Hooks are called in order to allow for modification of documents before they are converted.
	Hooks []func(ctx context.Context, doc *document.D) (*document.D, errors.E)
	// LanguageCodes is a map that maps language document ID to primary language subtag (e.g., "en").
	LanguageCodes map[identifier.Identifier]string
	// IndexAncestorProperties enables claim propagation to transitive super-properties:
	// when set, a claim for property X is also indexed for every ancestor of X via
	// SUBPROPERTY_OF. Disabled by default; only the original property is indexed.
	IndexAncestorProperties bool

	// propertyDescendants maps a property ID to all its transitive sub-property IDs.
	propertyDescendants map[identifier.Identifier][]identifier.Identifier
	// propertyAncestors maps a property ID to all its transitive super-property IDs.
	propertyAncestors map[identifier.Identifier][]identifier.Identifier
	// valueHierarchyProperties lists hierarchy-defining property IDs for value expansion
	// (sub-properties of SUBENTITY_OF, excluding INSTANCE_OF and SUBPROPERTY_OF).
	valueHierarchyProperties []identifier.Identifier
	// namingProperties is the set of property IDs that are NAMING or sub-properties of NAMING.
	namingProperties []identifier.Identifier
	// inverseProperties maps a property ID to all its inverse property IDs.
	// Both directions are stored: if X has INVERSE_PROPERTY_OF -> Y, then
	// Y is in inverseProperties[X] and X is in inverseProperties[Y].
	// Multiple properties can be inverses of the same property.
	inverseProperties map[identifier.Identifier][]identifier.Identifier
	// fieldInverseProperties maps a (field path, source property ID) pair to the
	// target inverse property ID defined on class field definitions. Built from all
	// class documents that define fields with INVERSE_PROPERTY. Field-level inverse
	// properties take precedence over property-level INVERSE_PROPERTY_OF.
	fieldInverseProperties map[fieldInverseKey]identifier.Identifier
	// languagePriority defines per-language fallback order for display label resolution.
	// It maps a language to its ordered fallback languages for display label resolution.
	// If a language is not a key, fallback is only the undetermined language.
	// If a language has an empty slice, no fallback is attempted at all.
	languagePriority map[string][]string
	// enabledLanguages is the set of languages this site indexes. Derived from
	// the languagePriority keys (plus "und") when set, otherwise the package-level
	// SupportedLanguages default. The converter populates per-language indexed
	// content only for these languages.
	enabledLanguages map[string]bool
	// recognizedLanguages is the set of languages whose content the converter
	// identifies: the enabled languages plus any language that appears only as a
	// fallback target in languagePriority. A fallback-target-only language is
	// recognized (its content is grouped under its own code so it can serve as a
	// display fallback) but not enabled (it gets no index field, and its content
	// is dropped from the text buckets).
	recognizedLanguages map[string]bool
	// getDocument fetches a document by ID from the store.
	getDocument func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E)
	// documentInfoMu protects documentInfoCache for concurrent access.
	documentInfoMu sync.RWMutex
	// documentInfoCache caches computed document info (display strings and hierarchy ancestors) per document ID.
	documentInfoCache map[identifier.Identifier]documentInfo
}

// NewConverter creates a Converter that preprocesses property hierarchies and discovers
// value hierarchy types. properties contains all property documents (used for SUBPROPERTY_OF
// hierarchy and discovering other hierarchy types), languages contains language documents
// needed for language code extraction, classes contains class documents with field definitions
// (used for field-level INVERSE_PROPERTY), languagePriority defines per-language fallback
// order for display label resolution, and getDocument is a callback to fetch documents by ID.
//
// Value hierarchies (e.g., SUBCLASS_OF) are computed lazily during conversion.
func NewConverter(
	properties, languages, classes []*document.D,
	languagePriority map[string][]string,
	getDocument func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E),
) (*Converter, errors.E) {
	errE := validateLanguagePriority(languagePriority)
	if errE != nil {
		return nil, errE
	}
	enabledLanguages, languagePriority := enabledLanguagesFromLanguagePriority(languagePriority)
	// Recognized languages are the enabled ones plus any language that appears
	// only as a fallback target. Those are identified (so they can serve as a
	// display fallback) but not indexed.
	recognizedLanguages := make(map[string]bool, len(enabledLanguages))
	for lang := range enabledLanguages {
		recognizedLanguages[lang] = true
	}
	for _, fallbacks := range languagePriority {
		for _, fb := range fallbacks {
			recognizedLanguages[fb] = true
		}
	}
	c := &Converter{
		Hooks:                    nil,
		LanguageCodes:            nil,
		IndexAncestorProperties:  false,
		propertyDescendants:      nil,
		propertyAncestors:        nil,
		valueHierarchyProperties: nil,
		namingProperties:         nil,
		inverseProperties:        nil,
		fieldInverseProperties:   nil,
		languagePriority:         languagePriority,
		enabledLanguages:         enabledLanguages,
		recognizedLanguages:      recognizedLanguages,
		getDocument:              getDocument,
		documentInfoCache:        map[identifier.Identifier]documentInfo{},
		documentInfoMu:           sync.RWMutex{},
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()
	c.buildNamingProperties()
	c.buildLanguageCodes(languages)
	c.buildInverseProperties(properties)
	c.buildFieldInverseProperties(classes)
	return c, nil
}

// validateLanguagePriority checks that all languages in priority are supported.
func validateLanguagePriority(priority map[string][]string) errors.E {
	for lang, fallbacks := range priority {
		if !SupportedLanguages[lang] {
			errE := errors.New("unsupported language in priority key")
			errors.Details(errE)["language"] = lang
			return errE
		}
		for _, fb := range fallbacks {
			if fb == lang {
				errE := errors.New("language cannot be its own fallback")
				errors.Details(errE)["language"] = lang
				return errE
			}
			if !SupportedLanguages[fb] {
				errE := errors.New("unsupported language in priority fallback")
				errors.Details(errE)["language"] = lang
				errors.Details(errE)["fallback"] = fb
				return errE
			}
		}
	}
	return nil
}

// isInstanceOf returns true if the document has an INSTANCE_OF reference claim
// pointing to the given class ID.
func isInstanceOf(doc *document.D, classID identifier.Identifier) bool {
	for _, rel := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, internalCore.InstanceOfPropID, document.LowConfidence) {
		if rel.To.ID == classID {
			return true
		}
	}
	return false
}

// buildPropertyHierarchy computes transitive descendants and ancestors for each property
// based on SUBPROPERTY_OF reference claims. Only documents that are instances of PROPERTY
// are considered.
func (c *Converter) buildPropertyHierarchy(properties []*document.D) {
	// Build parent -> children and child -> parents maps.
	// A property X with SUBPROPERTY_OF -> Y means X is a child (sub-property) of Y.
	parentChildren := map[identifier.Identifier][]identifier.Identifier{}
	childParents := map[identifier.Identifier][]identifier.Identifier{}
	for _, prop := range properties {
		if !isInstanceOf(prop, internalCore.PropertyClassID) {
			continue
		}
		for _, rel := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](prop, internalCore.SubpropertyOfPropID, document.LowConfidence) {
			parentChildren[rel.To.ID] = append(parentChildren[rel.To.ID], prop.ID)
			childParents[prop.ID] = append(childParents[prop.ID], rel.To.ID)
		}
	}

	// Compute transitive descendants for each property (used for naming properties).
	c.propertyDescendants = map[identifier.Identifier][]identifier.Identifier{}
	for _, prop := range properties {
		visited := map[identifier.Identifier]bool{}
		var walk func(identifier.Identifier)
		walk = func(propID identifier.Identifier) {
			for _, child := range parentChildren[propID] {
				if !visited[child] {
					visited[child] = true
					walk(child)
				}
			}
		}
		walk(prop.ID)
		// Exclude the property itself to avoid duplicates when consuming code
		// prepends the property (e.g., propagateProp).
		delete(visited, prop.ID)
		if len(visited) > 0 {
			result := make([]identifier.Identifier, 0, len(visited))
			for d := range visited {
				result = append(result, d)
			}
			c.propertyDescendants[prop.ID] = result
		}
	}

	// Compute transitive ancestors for each property (used for claim propagation).
	c.propertyAncestors = map[identifier.Identifier][]identifier.Identifier{}
	for _, prop := range properties {
		visited := map[identifier.Identifier]bool{}
		var walk func(identifier.Identifier)
		walk = func(propID identifier.Identifier) {
			for _, parent := range childParents[propID] {
				if !visited[parent] {
					visited[parent] = true
					walk(parent)
				}
			}
		}
		walk(prop.ID)
		// Exclude the property itself to avoid duplicates when consuming code
		// prepends the property (e.g., propagateProp).
		delete(visited, prop.ID)
		if len(visited) > 0 {
			result := make([]identifier.Identifier, 0, len(visited))
			for a := range visited {
				result = append(result, a)
			}
			c.propertyAncestors[prop.ID] = result
		}
	}
}

// discoverValueHierarchyProperties finds all sub-properties of SUBENTITY_OF
// that define value hierarchies. INSTANCE_OF and SUBPROPERTY_OF are excluded:
// INSTANCE_OF because there is no sub-instance-of concept, and SUBPROPERTY_OF
// because it is used for property propagation, not value expansion.
func (c *Converter) discoverValueHierarchyProperties() {
	c.valueHierarchyProperties = nil
	for _, desc := range c.propertyDescendants[internalCore.SubentityOfPropID] {
		if desc == internalCore.InstanceOfPropID || desc == internalCore.SubpropertyOfPropID {
			continue
		}
		c.valueHierarchyProperties = append(c.valueHierarchyProperties, desc)
	}
}

// buildNamingProperties computes the set of all properties that are
// NAMING or transitive sub-properties of NAMING.
func (c *Converter) buildNamingProperties() {
	c.namingProperties = []identifier.Identifier{internalCore.NamingPropID}
	c.namingProperties = append(c.namingProperties, c.propertyDescendants[internalCore.NamingPropID]...)
}

// buildLanguageCodes extracts language codes from language documents.
//
// Only language documents (those with INSTANCE_OF -> LANGUAGE) need to be passed,
// but the method still filters by INSTANCE_OF for safety.
func (c *Converter) buildLanguageCodes(allDocuments []*document.D) {
	c.LanguageCodes = map[identifier.Identifier]string{}
	for _, doc := range allDocuments {
		if !isInstanceOf(doc, internalCore.LanguageClassID) {
			continue
		}
		// Extract the CODE identifier claim and use the primary language subtag.
		// Register codes that are recognized (enabled or a fallback target) so a
		// fallback-only language is identified for display resolution; an
		// unrecognized language quietly falls back to "und".
		ids := document.GetClaimsOfTypeWithConfidence[document.IdentifierClaim](doc, internalCore.CodePropID, document.LowConfidence)
		for _, id := range ids {
			code, _, _ := strings.Cut(id.Value, "-")
			if c.recognizedLanguages[code] {
				c.LanguageCodes[doc.ID] = code
			}
		}
	}
}

// buildInverseProperties computes the bidirectional inverse property mapping.
// If property X has INVERSE_PROPERTY_OF -> Y, then Y is added to inverseProperties[X]
// and X is added to inverseProperties[Y]. Multiple properties can be inverses of
// the same property (e.g., both X and Z can have INVERSE_PROPERTY_OF -> Y).
func (c *Converter) buildInverseProperties(properties []*document.D) {
	c.inverseProperties = map[identifier.Identifier][]identifier.Identifier{}
	for _, prop := range properties {
		if !isInstanceOf(prop, internalCore.PropertyClassID) {
			continue
		}
		for _, rel := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](prop, internalCore.InversePropertyOfPropID, document.LowConfidence) {
			if !slices.Contains(c.inverseProperties[prop.ID], rel.To.ID) {
				c.inverseProperties[prop.ID] = append(c.inverseProperties[prop.ID], rel.To.ID)
			}
			if !slices.Contains(c.inverseProperties[rel.To.ID], prop.ID) {
				c.inverseProperties[rel.To.ID] = append(c.inverseProperties[rel.To.ID], prop.ID)
			}
		}
	}
}

// encodeFieldPath encodes a slice of property IDs into a string for use as a
// fieldInverseKey path. IDs are joined by "/". Returns empty string for empty path.
func encodeFieldPath(path []identifier.Identifier) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(path))
	for _, id := range path {
		parts = append(parts, id.String())
	}
	return strings.Join(parts, "/")
}

// buildFieldInverseProperties extracts inverse property definitions from class
// document field hierarchies. For each class document that has FIELD or SECTION
// claims, it walks the field tree (FIELD -> SUB_FIELD) and records any
// INVERSE_PROPERTY settings as (field path, source property) -> target property.
func (c *Converter) buildFieldInverseProperties(classes []*document.D) {
	c.fieldInverseProperties = map[fieldInverseKey]identifier.Identifier{}
	for _, cls := range classes {
		// Process top-level FIELD HasClaims.
		for _, field := range document.GetClaimsOfTypeWithConfidence[document.HasClaim](cls, internalCore.FieldPropID, document.LowConfidence) {
			c.processFieldInverse(nil, field)
		}
		// Process SECTION HasClaims which contain FIELD HasClaims.
		for _, section := range document.GetClaimsOfTypeWithConfidence[document.HasClaim](cls, internalCore.SectionPropID, document.LowConfidence) {
			for _, field := range document.GetClaimsOfTypeWithConfidence[document.HasClaim](section, internalCore.FieldPropID, document.LowConfidence) {
				c.processFieldInverse(nil, field)
			}
		}
	}
}

// processFieldInverse extracts inverse property from a single field HasClaim
// and recurses into SUB_FIELD HasClaims. parentPath tracks the accumulated
// property IDs from parent fields.
func (c *Converter) processFieldInverse(parentPath []identifier.Identifier, field *document.HasClaim) {
	// Extract the field's property (HAS_PROPERTY reference).
	hasPropRef := document.GetBestClaimOfType[document.ReferenceClaim](field, internalCore.HasPropertyPropID)
	if hasPropRef == nil {
		return
	}
	propID := hasPropRef.To.ID

	// Check for INVERSE_PROPERTY reference.
	inversePropRef := document.GetBestClaimOfType[document.ReferenceClaim](field, internalCore.InversePropertyPropID)
	if inversePropRef != nil {
		key := fieldInverseKey{
			Path:       encodeFieldPath(parentPath),
			SourceProp: propID,
		}
		c.fieldInverseProperties[key] = inversePropRef.To.ID
	}

	// Recurse into SUB_FIELD HasClaims.
	childPath := append(slices.Clone(parentPath), propID)
	for _, subField := range document.GetClaimsOfTypeWithConfidence[document.HasClaim](field, internalCore.SubFieldPropID, document.LowConfidence) {
		c.processFieldInverse(childPath, subField)
	}
}

// getDocumentInfo returns the document info for a document, computing and
// caching it on first access. It computes display strings and lazily walks
// value hierarchy ancestors (e.g., SUBCLASS_OF). It is safe for concurrent use.
func (c *Converter) getDocumentInfo(ctx context.Context, id identifier.Identifier) (documentInfo, errors.E) {
	return c.computeDocumentInfo(ctx, id, map[identifier.Identifier]bool{})
}

// computeDocumentInfo fetches a document, computes its display strings, and lazily
// walks value hierarchy ancestors. The computing set prevents infinite recursion
// when cycles exist in hierarchy data. Results are cached for reuse.
func (c *Converter) computeDocumentInfo(ctx context.Context, id identifier.Identifier, computing map[identifier.Identifier]bool) (documentInfo, errors.E) {
	// Check cache.
	c.documentInfoMu.RLock()
	if info, ok := c.documentInfoCache[id]; ok {
		c.documentInfoMu.RUnlock()
		return info, nil
	}
	c.documentInfoMu.RUnlock()

	// Cycle protection.
	if computing[id] {
		return documentInfo{}, nil
	}
	computing[id] = true

	doc, errE := c.getDocument(ctx, id)
	if errE != nil {
		if errors.Is(errE, store.ErrValueNotFound) {
			// Referenced document does not exist (or was deleted). Return empty
			// info without caching so that a later re-index can pick it up.
			zerolog.Ctx(ctx).Warn().Err(errE).
				Str("id", id.String()).
				Msg("referenced document not found, using empty info")
			return documentInfo{}, nil
		}
		return documentInfo{}, errE
	}
	display, errE := c.makeDisplayStrings(ctx, doc)
	if errE != nil {
		return documentInfo{}, errE
	}

	// Compute ancestors and hierarchy paths for each value hierarchy property.
	var ancestors map[identifier.Identifier][]identifier.Identifier
	var idPaths map[identifier.Identifier][]string
	var displayPaths map[identifier.Identifier]map[string][]string
	idStr := id.String()
	for _, hierProp := range c.valueHierarchyProperties {
		refs := document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, hierProp, document.LowConfidence)
		if len(refs) == 0 {
			continue
		}
		seen := map[identifier.Identifier]bool{id: true} // Exclude self to avoid duplicates.
		var hierAncestors []identifier.Identifier
		var hierIDPaths []string
		hierDisplayPaths := map[string][]string{}
		for _, ref := range refs {
			parentID := ref.To.ID
			if seen[parentID] {
				continue
			}
			seen[parentID] = true
			hierAncestors = append(hierAncestors, parentID)
			// Recursively get parent info to collect transitive ancestors and paths.
			parentInfo, errE := c.computeDocumentInfo(ctx, parentID, computing)
			if errE != nil {
				return documentInfo{}, errE
			}
			for _, grandparent := range parentInfo.Ancestors[hierProp] {
				if !seen[grandparent] {
					seen[grandparent] = true
					hierAncestors = append(hierAncestors, grandparent)
				}
			}
			// Extend parent's hierarchy paths with this document.
			if parentPaths := parentInfo.IDPaths[hierProp]; len(parentPaths) > 0 {
				for _, pp := range parentPaths {
					hierIDPaths = append(hierIDPaths, pp+"/"+idStr)
				}
			} else {
				// Parent has no paths (e.g., root or cycle break), create a two-level path.
				hierIDPaths = append(hierIDPaths, parentID.String()+"/"+idStr)
			}
			// Extend parent's display paths with this document's display.
			c.extendDisplayPaths(hierDisplayPaths, parentInfo, hierProp, display)
		}
		if len(hierAncestors) > 0 {
			if ancestors == nil {
				ancestors = map[identifier.Identifier][]identifier.Identifier{}
			}
			ancestors[hierProp] = hierAncestors
		}
		if len(hierIDPaths) > 0 {
			if idPaths == nil {
				idPaths = map[identifier.Identifier][]string{}
			}
			idPaths[hierProp] = hierIDPaths
		}
		if len(hierDisplayPaths) > 0 {
			if displayPaths == nil {
				displayPaths = map[identifier.Identifier]map[string][]string{}
			}
			displayPaths[hierProp] = hierDisplayPaths
		}
	}

	info := documentInfo{
		Display:      display,
		Ancestors:    ancestors,
		IDPaths:      idPaths,
		DisplayPaths: displayPaths,
	}

	c.documentInfoMu.Lock()
	c.documentInfoCache[id] = info
	c.documentInfoMu.Unlock()

	return info, nil
}

// extendDisplayPaths extends hierDisplayPaths by appending this document's display to
// each of the parent's display paths for all supported languages. Every level adds a
// separator, even when display labels are empty strings.
func (c *Converter) extendDisplayPaths(
	hierDisplayPaths map[string][]string,
	parentInfo documentInfo, hierProp identifier.Identifier,
	display displayStrings,
) {
	for lang := range c.enabledLanguages {
		// If lang does not exist in Display, this just means it is an empty string and we have not stored
		// it in the map. So reading a zero value from the map makes the right thing and we get an empty string back.
		thisDisplay := display.Display[lang]
		paths := parentInfo.DisplayPaths[hierProp][lang]

		if len(paths) > 0 {
			for _, pp := range paths {
				hierDisplayPaths[lang] = append(hierDisplayPaths[lang], pp+hierarchyPathSeparator+thisDisplay)
			}
		} else {
			// Parent is a root (no paths yet), create a two-level path.
			// Parent display might be an empty string and this is OK.
			parentDisplay := parentInfo.Display.Display[lang]
			hierDisplayPaths[lang] = append(hierDisplayPaths[lang], parentDisplay+hierarchyPathSeparator+thisDisplay)
		}
	}
}

// makeDisplayStrings returns the display strings for a document for every supported language.
//
// For every supported language this should match what is shown in the UI to users when they
// configure UI to show them data in that language.
//
// For each supported language (with its fallback chain):
//  1. If the document's class defines a display label template for the language
//     (resolved via IN_LANGUAGE sub-claims with the language fallback chain),
//     render it with the target language (template functions use that language's
//     fallback chain internally). The result is the display label, even if empty.
//  2. If no template resolves for the language, search naming strings through the
//     fallback chain. The first (highest confidence) naming string from the first
//     language in the chain that has naming strings becomes the display label.
//
// Naming contains all naming strings per language as extracted from claims, without
// modifications. It is independent of Display.
func (c *Converter) makeDisplayStrings(ctx context.Context, doc *document.D) (displayStrings, errors.E) {
	templates, errE := c.displayLabelTemplate(ctx, doc)
	if errE != nil {
		return displayStrings{}, errE
	}

	result := displayStrings{
		Display: map[string]string{},
		Naming:  c.namingStrings(doc),
	}

	for lang := range c.enabledLanguages {
		if tmplStr, ok := templates[lang]; ok {
			// Template exists for this language: render it with the target language so that
			// template functions (e.g., bestString) use that language's fallback chain.
			rendered, errE := c.renderDisplayTemplate(ctx, doc, lang, tmplStr)
			if errE != nil {
				return displayStrings{}, errE
			}
			rendered = sanitizeDisplayString(strings.TrimSpace(rendered))
			if rendered != "" {
				// We do not store an empty string into the map. But we still read it out as
				// an empty string when needed (reading a zero value from the map).
				result.Display[lang] = rendered
			}
			// Even if rendered is an empty string, we are done for this language.
			// This is also what happens in UI.
			continue
		}

		// No template for this language. Search naming strings in the fallback chain.
		// Both here and in namingProperties we traverse naming claims, so we traverse it twice, but we do that
		// so that code here matches the implementation on the frontend and that it is easier to compare with it.
		selected := document.SelectClaimsByLanguage[document.StringClaim](
			doc, c.namingProperties, lang,
			func(claims []*document.StringClaim) bool {
				// Here we want only a non-empty string (after sanitization).
				// So if we got an empty string here, we ignore it and continue searching.
				return len(claims) > 0 && sanitizeDisplayString(claims[0].String) != ""
			},
			document.LowConfidence, c.LanguageCodes, c.languagePriority,
		)
		if len(selected) > 0 {
			// This cannot be an empty string here because we already checked for it above.
			// We do not store an empty string into the map. But we still read it out as
			// an empty string when needed (reading a zero value from the map).
			result.Display[lang] = sanitizeDisplayString(selected[0].String)
		}
	}

	return result, nil
}

// sanitizeDisplayString removes the hierarchy path separator from a display string to
// prevent any issues with potential conflicts between display strings and hierarchy path separators.
func sanitizeDisplayString(s string) string {
	return strings.ReplaceAll(s, hierarchyPathSeparator, "")
}

// getDisplayStrings is a convenience wrapper around getDocumentInfo that
// returns only the display strings.
func (c *Converter) getDisplayStrings(ctx context.Context, id identifier.Identifier) (displayStrings, errors.E) {
	info, errE := c.getDocumentInfo(ctx, id)
	if errE != nil {
		return displayStrings{}, errE
	}
	return info.Display, nil
}

// displayLabelTemplate returns the best display label template for each supported
// language by looking at the document's INSTANCE_OF class documents. Templates can
// have IN_LANGUAGE sub-claims to specify which language they apply to; templates
// without IN_LANGUAGE are treated as undetermined language and can be reached via
// fallback.
//
// For each supported language, the fallback chain is walked to find the best template.
// Because templates are reached indirectly - through the document's INSTANCE_OF claims
// to classes, then through the class's template claim - each template carries an
// effective confidence: the product of the INSTANCE_OF claim's confidence and the
// template claim's confidence. Within the first language in the chain that has
// templates, the one with the highest effective confidence wins.
//
// Returns nil if no templates are found for any language.
func (c *Converter) displayLabelTemplate(ctx context.Context, doc *document.D) (map[string]string, errors.E) {
	// Collect templates grouped by their language with effective confidences.
	type templateEntry struct {
		template   string
		confidence document.Confidence
	}
	byLang := map[string][]templateEntry{}

	for _, rel := range document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, internalCore.InstanceOfPropID, document.LowConfidence) {
		classDoc, errE := c.getDocument(ctx, rel.To.ID)
		if errE != nil {
			if errors.Is(errE, store.ErrValueNotFound) {
				// Class document does not exist, skip it.
				zerolog.Ctx(ctx).Warn().Err(errE).
					Str("id", doc.ID.String()).
					Str("classId", rel.To.ID.String()).
					Msg("class document for the document not found, skipping it for display label template")
				continue
			}
			return nil, errE
		}
		instanceConfidence := rel.GetConfidence()
		for _, sc := range document.GetClaimsOfTypeWithConfidence[document.StringClaim](classDoc, internalCore.DisplayLabelTemplatePropID, document.LowConfidence) {
			effective := instanceConfidence * sc.GetConfidence()
			for _, lang := range c.extractInLanguages(sc.Sub) {
				byLang[lang] = append(byLang[lang], templateEntry{template: sc.String, confidence: effective})
			}
		}
	}

	if len(byLang) == 0 {
		return nil, nil //nolint:nilnil
	}

	// For each enabled language, find the best template using the fallback chain.
	result := make(map[string]string, len(c.enabledLanguages))
	for lang := range c.enabledLanguages {
		chain := []string{lang}
		if fallbacks, ok := c.languagePriority[lang]; ok {
			chain = append(chain, fallbacks...)
		} else if lang != document.UndeterminedLanguage {
			chain = append(chain, document.UndeterminedLanguage)
		}
		for _, tryLang := range chain {
			entries, ok := byLang[tryLang]
			if !ok || len(entries) == 0 {
				continue
			}
			// Found templates for this fallback language. Pick the one with highest confidence.
			bestIdx := 0
			for i := 1; i < len(entries); i++ {
				if entries[i].confidence > entries[bestIdx].confidence {
					bestIdx = i
				}
			}
			result[lang] = entries[bestIdx].template
			break
		}
	}

	if len(result) == 0 {
		return nil, nil //nolint:nilnil
	}

	return result, nil
}

// renderDisplayTemplate parses and executes a display label template string.
func (c *Converter) renderDisplayTemplate(ctx context.Context, doc *document.D, lang, tmplStr string) (string, errors.E) {
	tmpl, err := template.New("display").Funcs(c.templateFuncs(ctx, lang)).Parse(tmplStr)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["template"] = tmplStr
		return "", errE
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, doc)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["template"] = tmplStr
		return "", errE
	}
	return buf.String(), nil
}

// templateFuncs returns template functions for rendering display label templates.
// Functions accept mnemonic as the first argument and *document.D as the last,
// making them composable with Go template pipelines.
//
// Sprig functions (https://masterminds.github.io/sprig/) are included as a base.
func (c *Converter) templateFuncs(ctx context.Context, lang string) template.FuncMap {
	funcs := sprig.HermeticTxtFuncMap()
	// identifierString returns an identifier.Identifier from the string version of the identifier.
	funcs["identifierString"] = func(s string) (identifier.Identifier, error) {
		return identifier.MaybeString(s)
	}
	// identifier returns an identifier.Identifier from the given values.
	funcs["identifier"] = identifier.From
	// bestString returns the best string claim value for a property ID in the current language.
	// Falls back using the language priority chain.
	funcs["bestString"] = func(propID identifier.Identifier, doc *document.D) (string, error) {
		if doc == nil {
			return "", nil
		}
		selected := document.SelectClaimsByLanguage[document.StringClaim](
			doc, []identifier.Identifier{propID}, lang,
			func(claims []*document.StringClaim) bool {
				// Here we are fine with empty strings.
				return len(claims) > 0
			},
			document.LowConfidence, c.LanguageCodes, c.languagePriority,
		)
		if len(selected) > 0 {
			return selected[0].String, nil
		}
		return "", nil
	}
	// bestAmountString returns the string of the best amount claim for a property ID.
	funcs["bestAmountString"] = func(propID identifier.Identifier, doc *document.D) (string, error) {
		if doc == nil {
			return "", nil
		}
		ac := document.GetBestClaimOfType[document.AmountClaim](doc, propID)
		if ac == nil {
			return "", nil
		}
		return ac.Amount.String(), nil
	}
	// bestReferenceDoc follows the best reference claim for a property ID and returns the target document.
	funcs["bestReferenceDoc"] = func(propID identifier.Identifier, doc *document.D) (*document.D, error) {
		if doc == nil {
			return nil, nil //nolint:nilnil
		}
		rc := document.GetBestClaimOfType[document.ReferenceClaim](doc, propID)
		if rc == nil {
			return nil, nil //nolint:nilnil
		}
		d, errE := c.getDocument(ctx, rc.To.ID)
		if errE != nil && errors.Is(errE, store.ErrValueNotFound) {
			zerolog.Ctx(ctx).Warn().Err(errE).
				Str("id", doc.ID.String()).
				Str("propId", propID.String()).
				Str("referenceId", rc.To.ID.String()).
				Msg("bestReferenceDoc: reference not found, returning nil")
			return nil, nil //nolint:nilnil
		}
		return d, errE
	}
	// getDocument returns the document for a document ID.
	funcs["getDocument"] = func(docID identifier.Identifier) (*document.D, error) {
		d, errE := c.getDocument(ctx, docID)
		if errE != nil && errors.Is(errE, store.ErrValueNotFound) {
			return nil, nil //nolint:nilnil
		}
		return d, errE
	}
	// bestIdentifier returns the best identifier claim value for a property ID.
	funcs["bestIdentifier"] = func(propID identifier.Identifier, doc *document.D) (string, error) {
		if doc == nil {
			return "", nil
		}
		ic := document.GetBestClaimOfType[document.IdentifierClaim](doc, propID)
		if ic == nil {
			return "", nil
		}
		return ic.Value, nil
	}
	// bestTimeString returns the display string of the best time claim for a property ID.
	funcs["bestTimeString"] = func(propID identifier.Identifier, doc *document.D) (string, error) {
		if doc == nil {
			return "", nil
		}
		tc := document.GetBestClaimOfType[document.TimeClaim](doc, propID)
		if tc == nil {
			return "", nil
		}
		return tc.Time.String(), nil
	}
	// referenceClaimDoc resolves the target document of a ReferenceClaim.
	funcs["referenceClaimDoc"] = func(claim *document.ReferenceClaim) (*document.D, error) {
		if claim == nil {
			return nil, nil //nolint:nilnil
		}
		d, errE := c.getDocument(ctx, claim.To.ID)
		if errE != nil && errors.Is(errE, store.ErrValueNotFound) {
			zerolog.Ctx(ctx).Warn().Err(errE).
				Str("referenceId", claim.To.ID.String()).
				Msg("referenceClaimDoc: reference not found, returning nil")
			return nil, nil //nolint:nilnil
		}
		return d, errE
	}
	// bestSubTimeInterval returns the best TimeIntervalClaim from the sub-claims of a
	// ReferenceClaim for a given sub-property ID.
	funcs["bestSubTimeInterval"] = func(subPropID identifier.Identifier, claim *document.ReferenceClaim) *document.TimeIntervalClaim {
		if claim == nil {
			return nil
		}
		return document.GetBestClaimOfType[document.TimeIntervalClaim](claim, subPropID)
	}
	// pickReferenceByEarliestSubInterval picks the ReferenceClaim for propID whose
	// sub-claim TimeIntervalClaim for subPropID has the earliest From time. Time strings
	// follow a format where lexicographic comparison is monotonic, so direct string
	// comparison gives the correct ordering. If no claim has a defined sub-interval From,
	// returns the first ReferenceClaim by confidence. Returns nil if no ReferenceClaims
	// exist for propID.
	funcs["pickReferenceByEarliestSubInterval"] = func(propID, subPropID identifier.Identifier, doc *document.D) *document.ReferenceClaim {
		if doc == nil {
			return nil
		}
		refClaims := document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](doc, propID, document.LowConfidence)
		if len(refClaims) == 0 {
			return nil
		}
		var best *document.ReferenceClaim
		var bestFrom *document.Time
		for _, rc := range refClaims {
			ti := document.GetBestClaimOfType[document.TimeIntervalClaim](rc, subPropID)
			if ti == nil || ti.From == nil {
				continue
			}
			if bestFrom == nil || string(*ti.From) < string(*bestFrom) {
				best = rc
				bestFrom = ti.From
			}
		}
		if best != nil {
			return best
		}
		return refClaims[0]
	}
	// formatTimeInterval renders a TimeIntervalClaim's starting date as a display string.
	// Returns "" if the claim is nil or has no defined From.
	funcs["formatTimeInterval"] = func(claim *document.TimeIntervalClaim) string {
		if claim == nil || claim.From == nil {
			return ""
		}
		return string(*claim.From)
	}
	return funcs
}

// namingStrings returns all naming display strings per language for a document.
// It collects string claims whose property is NAMING or any sub-property of NAMING
// (NAME, SHORT_NAME, ALTERNATIVE_NAME, TITLE, CODE, MNEMONIC, etc.).
func (c *Converter) namingStrings(doc *document.D) map[string][]string {
	claimsByLang := document.GetClaimsAndLanguageOfTypeWithConfidence[document.StringClaim](
		doc, c.namingProperties, document.LowConfidence, c.LanguageCodes, c.languagePriority,
	)
	if len(claimsByLang) == 0 {
		return nil
	}
	result := make(map[string][]string, len(claimsByLang))
	for lang, claims := range claimsByLang {
		result[lang] = make([]string, 0, len(claims))
		for _, sc := range claims {
			result[lang] = append(result[lang], sc.String)
		}
	}
	return result
}

// extractInLanguages extracts language codes from a claim's IN_LANGUAGE sub-claim references.
// A claim can be in multiple languages, so all matching codes are returned.
// Returns ["und"] if no languages are specified or none can be resolved to
// supported languages.
func (c *Converter) extractInLanguages(claims document.Claims) []string {
	if claims == nil {
		return []string{document.UndeterminedLanguage}
	}
	refs := document.GetClaimsOfTypeWithConfidence[document.ReferenceClaim](claims, internalCore.InLanguagePropID, document.LowConfidence)
	var codes []string
	for _, ref := range refs {
		if code, ok := c.LanguageCodes[ref.To.ID]; ok && c.recognizedLanguages[code] {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return []string{document.UndeterminedLanguage}
	}
	return codes
}

// extractInUnit extracts the unit identifier from a claim's IN_UNIT sub-claim reference.
func (c *Converter) extractInUnit(sub *document.ClaimTypes) *identifier.Identifier {
	if rel := document.GetBestClaimOfType[document.ReferenceClaim](sub, internalCore.InUnitPropID); rel != nil {
		return &rel.To.ID
	}
	return nil
}

// propagateProp returns the property IDs to create claims for. When
// IndexAncestorProperties is set, this is the original property plus all its
// transitive super-properties (so a claim for sub-property X also produces a
// claim for Y); otherwise only the original property is returned.
func (c *Converter) propagateProp(propID identifier.Identifier) []identifier.Identifier {
	if !c.IndexAncestorProperties {
		return []identifier.Identifier{propID}
	}
	result := make([]identifier.Identifier, 0, 1+len(c.propertyAncestors[propID]))
	result = append(result, propID)
	result = append(result, c.propertyAncestors[propID]...)
	return result
}

// convertVisitor implements document.Visitor to convert claims to search claims.
type convertVisitor struct {
	ctx       context.Context //nolint:containedctx
	converter *Converter
	result    *Document
	// docID is the ID of the document being converted.
	docID identifier.Identifier
}

var _ document.Visitor = (*convertVisitor)(nil)

// addText appends value to the document's top-level text bucket for the given
// language, skipping empty/whitespace-only values and lazily initializing the
// map. Content in a recognized-but-not-enabled language (a fallback-target-only
// language) is dropped: it has no index field, and it reaches search only
// through the display labels it resolves to.
func (v *convertVisitor) addText(lang, value string) {
	if !v.converter.enabledLanguages[lang] {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if v.result.Text == nil {
		v.result.Text = map[string][]string{}
	}
	v.result.Text[lang] = append(v.result.Text[lang], value)
}

// appendClaimDisplaysToText folds claim values into the document's top-level
// text bucket so the text-search query can match against referenced-document
// names, numeric/temporal boundary strings, and has-claim property labels.
//
// For value claims (Amount, Time, Reference, and their Sub* counterparts) the
// property label (PropDisplay/PropNaming) is deliberately not folded: a property
// name like "date of birth" is structural, not document content, and folding it
// would make every document with such a claim match a search for the property
// name. Only the values fold: referenced-document labels (ToDisplay/ToNaming)
// and numeric/temporal bounds (FromDisplay/ToDisplay).
//
// Has claims (and their SubHas counterparts) are property-only: the assertion
// that the document "has" the property is itself the content, so their property
// labels do fold. None and Unknown claims, which assert the absence or
// ignorance of a value, are not folded.
//
// Display labels (ToDisplay, and the language-neutral FromDisplay/ToDisplay) are
// fallback-resolved and may mix languages, so they fold into the "und" bucket
// only (und_text analyzer), never into a language-specific bucket where a
// foreign stemmer would mangle them. Naming strings (ToNaming, PropNaming) are
// extracted per their own language, so they fold into that language's bucket
// where the matching analyzer applies.
func (v *convertVisitor) appendClaimDisplaysToText() {
	// Display values across the per-language map collapse into "und"; we drop
	// duplicates that arise when fallback resolves multiple languages to the
	// same rendered string.
	addDisplay := func(m map[string]string) {
		seen := map[string]bool{}
		for _, val := range m {
			if seen[val] {
				continue
			}
			seen[val] = true
			v.addText(document.UndeterminedLanguage, val)
		}
	}
	addNaming := func(m map[string][]string) {
		for lang, vals := range m {
			for _, val := range vals {
				v.addText(lang, val)
			}
		}
	}
	for _, c := range v.result.Claims.Amount {
		v.addText(document.UndeterminedLanguage, c.FromDisplay)
		v.addText(document.UndeterminedLanguage, c.ToDisplay)
	}
	for _, c := range v.result.Claims.Time {
		v.addText(document.UndeterminedLanguage, c.FromDisplay)
		v.addText(document.UndeterminedLanguage, c.ToDisplay)
	}
	for _, c := range v.result.Claims.Reference {
		addDisplay(c.ToDisplay)
		addNaming(c.ToNaming)
	}
	for _, c := range v.result.Claims.Has {
		addDisplay(c.PropDisplay)
		addNaming(c.PropNaming)
	}
	for _, c := range v.result.Claims.SubRef {
		addDisplay(c.ToDisplay)
		addNaming(c.ToNaming)
	}
	for _, c := range v.result.Claims.SubHas {
		addDisplay(c.PropDisplay)
		addNaming(c.PropNaming)
	}
	for _, c := range v.result.Claims.SubAmount {
		v.addText(document.UndeterminedLanguage, c.FromDisplay)
		v.addText(document.UndeterminedLanguage, c.ToDisplay)
	}
	for _, c := range v.result.Claims.SubTime {
		v.addText(document.UndeterminedLanguage, c.FromDisplay)
		v.addText(document.UndeterminedLanguage, c.ToDisplay)
	}
}

// VisitIdentifier folds the identifier value into the document's top-level
// text field under the undetermined-language bucket so the text-search query
// can score it together with other textual content.
func (v *convertVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	v.addText(document.UndeterminedLanguage, claim.Value)
	return document.Keep, nil
}

// VisitString folds the string value into the document's top-level text field
// under every language the claim's IN_LANGUAGE sub-claims resolve to.
func (v *convertVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	for _, lang := range v.converter.extractInLanguages(claim.Sub) {
		v.addText(lang, claim.String)
	}
	return document.Keep, nil
}

// VisitHTML strips HTML tags from the claim's value and folds the plain-text
// result into the document's top-level text field under every language the
// claim's IN_LANGUAGE sub-claims resolve to. stripHTML is called per-language
// because the language controls whether block-tag boundaries become spaces.
func (v *convertVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	for _, lang := range v.converter.extractInLanguages(claim.Sub) {
		v.addText(lang, stripHTML(claim.HTML, lang))
	}
	return document.Keep, nil
}

// VisitAmount converts an amount claim to search amount claims.
func (v *convertVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertAmount(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Amount = append(v.result.Claims.Amount, claims...)
	return document.Keep, nil
}

// VisitAmountInterval converts an amount interval claim to search amount claims.
func (v *convertVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	amountClaims, unknownClaims, subs, errE := v.converter.convertAmountInterval(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Amount = append(v.result.Claims.Amount, amountClaims...)
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, unknownClaims...)
	v.appendSubClaims(subs)
	return document.Keep, nil
}

// VisitTime converts a time claim to search time claims.
func (v *convertVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertTime(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Time = append(v.result.Claims.Time, claims...)
	return document.Keep, nil
}

// VisitTimeInterval converts a time interval claim to search time claims.
func (v *convertVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	timeClaims, unknownClaims, subs, errE := v.converter.convertTimeInterval(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Time = append(v.result.Claims.Time, timeClaims...)
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, unknownClaims...)
	v.appendSubClaims(subs)
	return document.Keep, nil
}

// VisitLink folds the link IRI into the document's top-level text field under
// the undetermined-language bucket so the URL components are searchable.
func (v *convertVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	v.addText(document.UndeterminedLanguage, claim.IRI)
	return document.Keep, nil
}

// VisitReference converts a reference claim to search reference claims.
func (v *convertVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, subs, errE := v.converter.convertReference(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Reference = append(v.result.Claims.Reference, claims...)
	v.appendSubClaims(subs)

	return document.Keep, nil
}

// VisitHas converts a has claim to search has claims.
func (v *convertVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, subs, errE := v.converter.convertHas(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Has = append(v.result.Claims.Has, claims...)
	v.appendSubClaims(subs)
	return document.Keep, nil
}

// VisitNone converts a none claim to search none claims.
func (v *convertVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, subs, errE := v.converter.convertNone(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.None = append(v.result.Claims.None, claims...)
	v.appendSubClaims(subs)
	return document.Keep, nil
}

// VisitUnknown converts an unknown claim to search unknown claims.
func (v *convertVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, subs, errE := v.converter.convertUnknown(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, claims...)
	v.appendSubClaims(subs)
	return document.Keep, nil
}

// appendSubClaims appends every Sub* slice from subs into the matching
// claims.sub* slice on the visitor's result document.
func (v *convertVisitor) appendSubClaims(subs subClaims) {
	v.result.Claims.SubRef = append(v.result.Claims.SubRef, subs.Refs...)
	v.result.Claims.SubAmount = append(v.result.Claims.SubAmount, subs.Amounts...)
	v.result.Claims.SubTime = append(v.result.Claims.SubTime, subs.Times...)
	v.result.Claims.SubHas = append(v.result.Claims.SubHas, subs.Has...)
}

// FromDocument converts a document.D to a search Document.
//
// inverseRelations contains reference claims from other documents that point to this document.
// For each inverse relation, a synthetic reverse reference claim is added to the search document.
func (c *Converter) FromDocument(
	ctx context.Context, doc *document.D, inverseRelations []store.InverseRelation,
) (*Document, errors.E) {
	var errE errors.E
	for i, hook := range c.Hooks {
		doc, errE = hook(ctx, doc)
		if errE != nil {
			errors.Details(errE)["hook"] = i
			return nil, errE
		}
		if doc == nil {
			errE = errors.New("hook returned nil document")
			errors.Details(errE)["hook"] = i
			return nil, errE
		}
	}

	v := &convertVisitor{
		ctx:       ctx,
		converter: c,
		result: &Document{
			ID:      doc.ID,
			Display: nil,
			Text:    nil,
			Claims:  ClaimTypes{},
		},
		docID: doc.ID,
	}

	// Render the document's display label per supported language and store
	// the non-empty results in the top-level "display" field.
	displayStrings, errE := c.makeDisplayStrings(ctx, doc)
	if errE != nil {
		return nil, errE
	}
	if len(displayStrings.Display) > 0 {
		v.result.Display = displayStrings.Display
	}

	// Index the document's own ID into the "und" bucket so a user typing the ID
	// (or a URL containing it) can locate the document via text search. The
	// query searches "und" alongside every per-language field, so it is reachable
	// from any language.
	v.addText(document.UndeterminedLanguage, doc.ID.String())

	errE = doc.Visit(v)
	if errE != nil {
		return nil, errE
	}

	// Process incoming inverse relations from metadata.
	for _, ir := range inverseRelations {
		claims, subs, errE := c.convertReference(ctx, &document.ReferenceClaim{
			CoreClaim: document.CoreClaim{
				ID:         inverseReferenceClaimID(doc.Base, ir.InverseRelationKey),
				Confidence: ir.Confidence,
			},
			Prop: document.Reference{ID: ir.TargetProp},
			To:   document.Reference{ID: ir.Source},
		})
		if errE != nil {
			return nil, errE
		}
		v.result.Claims.Reference = append(v.result.Claims.Reference, claims...)
		v.appendSubClaims(subs)
	}

	// Fold every non-text-claim display label into the top-level text bucket
	// so the text-search query can match against property names, referenced-
	// document names, and amount/time boundary strings without depending only
	// on the per-claim-type nested queries.
	v.appendClaimDisplaysToText()

	return v.result, nil
}

// inverseReferenceClaimID computes an unique claim ID for a synthetic inverse reference claim.
//
// It uses InverseRelationKey to avoid collisions between claims from
// different source documents that might share the same claim ID.
func inverseReferenceClaimID(base []string, irKey store.InverseRelationKey) identifier.Identifier {
	base = slices.Clone(base)
	base = append(base, "INVERSE_RELATION", irKey.Source.String(), irKey.Claim.String(), irKey.TargetProp.String())
	return identifier.From(base...)
}

// inverseRelationsVisitor implements document.Visitor to collect outgoing inverse
// relations from a document. It tracks the current field path (via sub-claims)
// and for each reference claim, resolves the inverse property from field-level
// definitions (taking precedence) or property-level INVERSE_PROPERTY_OF.
type inverseRelationsVisitor struct {
	converter *Converter
	docID     identifier.Identifier
	// path tracks the current nesting of claim property IDs.
	path []identifier.Identifier
	// result maps target document ID to collected inverse relations.
	result map[identifier.Identifier][]store.InverseRelation
}

var _ document.Visitor = (*inverseRelationsVisitor)(nil)

// recurse pushes propID onto the path, visits sub-claims, then pops.
func (v *inverseRelationsVisitor) recurse(propID identifier.Identifier, claim document.Claim) errors.E {
	v.path = append(v.path, propID)
	errE := claim.Visit(v)
	v.path = v.path[:len(v.path)-1]
	return errE
}

// VisitIdentifier recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitString recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitHTML recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitAmount recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitAmountInterval recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitTime recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitTimeInterval recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitLink recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitReference checks for inverse properties and creates InverseRelation entries,
// then recurses into sub-claims to find further nested references.
// It first checks field-level inverse properties (based on the current path), then
// falls back to property-level inverse properties.
func (v *inverseRelationsVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}

	// Check field-level inverse property first (takes precedence).
	key := fieldInverseKey{
		Path:       encodeFieldPath(v.path),
		SourceProp: claim.Prop.ID,
	}
	if targetProp, ok := v.converter.fieldInverseProperties[key]; ok {
		v.result[claim.To.ID] = append(v.result[claim.To.ID], store.InverseRelation{
			InverseRelationKey: store.InverseRelationKey{
				Claim:      claim.ID,
				Source:     v.docID,
				TargetProp: targetProp,
			},
			SourceProp: claim.Prop.ID,
			Target:     claim.To.ID,
			Confidence: claim.GetConfidence(),
		})
	} else {
		// Fall back to property-level inverse properties.
		for _, inversePropID := range v.converter.inverseProperties[claim.Prop.ID] {
			v.result[claim.To.ID] = append(v.result[claim.To.ID], store.InverseRelation{
				InverseRelationKey: store.InverseRelationKey{
					Claim:      claim.ID,
					Source:     v.docID,
					TargetProp: inversePropID,
				},
				SourceProp: claim.Prop.ID,
				Target:     claim.To.ID,
				Confidence: claim.GetConfidence(),
			})
		}
	}

	// Recurse into sub-claims (reference claims can also have sub-fields).
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitHas recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitNone recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// VisitUnknown recurses into sub-claims to find nested references.
func (v *inverseRelationsVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	return document.Keep, v.recurse(claim.Prop.ID, claim)
}

// OutgoingInverseRelations extracts the outgoing inverse relations from a document.
//
// It walks the document's claims using an inverseRelationsVisitor that tracks the
// current field path (via HasClaim nesting). For each reference claim, it resolves
// the inverse property from field-level INVERSE_PROPERTY (taking precedence) or
// property-level INVERSE_PROPERTY_OF. Only reference claims with a resolved inverse
// property produce InverseRelation entries. Returns a map keyed by target document ID.
func (c *Converter) OutgoingInverseRelations(doc *document.D) map[identifier.Identifier][]store.InverseRelation {
	v := &inverseRelationsVisitor{
		converter: c,
		docID:     doc.ID,
		path:      nil,
		result:    map[identifier.Identifier][]store.InverseRelation{},
	}
	// Visit cannot return an error from inverseRelationsVisitor.
	_ = doc.Visit(v)
	return v.result
}

func (c *Converter) convertAmount(ctx context.Context, claim *document.AmountClaim) ([]AmountClaim, errors.E) {
	// TODO: Normalize amounts of units of same measure to same base unit (e.g., cm and mm to m).
	unit := c.extractInUnit(claim.Sub)
	from, errE := claim.Amount.WindowStartFloat64(claim.Precision, false)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	to, errE := claim.Amount.WindowEndFloat64(claim.Precision, false)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	display := claim.Amount.String()

	rangeFrom := from
	rangeTo := to
	rangeFloat := RangeFloat{ //nolint:exhaustruct
		GreaterThanOrEqual: &rangeFrom,
		LessThan:           &rangeTo,
	}

	// Sanity check. Validate is strict and never swaps; for a single-point
	// amount with positive precision the bounds are always well-formed.
	errE = rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	props := c.propagateProp(claim.Prop.ID)
	result := make([]AmountClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, AmountClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Unit:        unit,
			Range:       rangeFloat,
			From:        &from,
			FromDisplay: display,
			To:          &to,
			ToDisplay:   display,
		})
	}
	return result, nil
}

//nolint:cyclop
func (c *Converter) convertAmountInterval(
	ctx context.Context, claim *document.AmountIntervalClaim,
) ([]AmountClaim, []UnknownClaim, subClaims, errors.E) {
	if claim.From != nil && claim.FromPrecision == nil {
		errE := errors.New("missing from precision in claim")
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}
	if claim.To != nil && claim.ToPrecision == nil {
		errE := errors.New("missing to precision in claim")
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}

	// Swap directed-decreasing input to ascending order, before computing
	// rangeFloat bounds. Dual criterion (matches document.AmountIntervalClaim.Validate):
	//   - When both bounds share the same precision, the directed-decreasing
	//     interpretation is unambiguous, so we use the simpler value-based
	//     criterion: swap iff fromValue > toValue.
	//   - When precisions differ, fall back to the un-swapped-empty
	//     criterion: swap iff start(from) >= end(to). This preserves
	//     precision-coarsening patterns (e.g., from=2025-10-21 day, to=2025
	//     year, where the to-window engulfs from).
	workClaim := *claim
	if workClaim.From != nil && workClaim.To != nil {
		var swap bool
		if *workClaim.FromPrecision == *workClaim.ToPrecision {
			fromValue, errE := workClaim.From.Float64(*workClaim.FromPrecision)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			toValue, errE := workClaim.To.Float64(*workClaim.ToPrecision)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			swap = fromValue > toValue
		} else {
			start, errE := workClaim.From.WindowStartFloat64(*workClaim.FromPrecision, workClaim.FromIsOpen)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			end, errE := workClaim.To.WindowEndFloat64(*workClaim.ToPrecision, workClaim.ToIsOpen)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			swap = start >= end
		}
		if swap {
			workClaim.From, workClaim.To = workClaim.To, workClaim.From
			workClaim.FromPrecision, workClaim.ToPrecision = workClaim.ToPrecision, workClaim.FromPrecision
			workClaim.FromIsOpen, workClaim.ToIsOpen = workClaim.ToIsOpen, workClaim.FromIsOpen
		}
	}

	var (
		rangeFloat  RangeFloat
		from, to    *float64
		fromDisplay string
		toDisplay   string
	)

	//nolint:dupl
	switch {
	case workClaim.From != nil:
		// FromIsOpen=true excludes the from-window: lower advances past the
		// window's end. Default closed-lower uses the window's start.
		f, errE := workClaim.From.WindowStartFloat64(*workClaim.FromPrecision, workClaim.FromIsOpen)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		from = &f
		fromDisplay = workClaim.From.String()
		rangeFloat.GreaterThanOrEqual = &f
	case workClaim.FromIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave from and fromDisplay empty.
		// But we want to find it always when we search by range.
		f := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &f
	case workClaim.FromIsUnknown && workClaim.To != nil:
		// Unknown From with known To: treat as single point at To.
		claims, errE := c.convertAmount(ctx, &document.AmountClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
			Amount:    *workClaim.To,
			Precision: *workClaim.ToPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, subClaims{}, errE
	default:
		// Unknown From with Unknown or None To. We cannot do much here,
		// so we convert it as an unknown claim. This also handles the case
		// of invalid claims (e.g., an empty claim without anything set).
		claims, subs, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
		})
		return nil, claims, subs, errE
	}

	switch {
	case workClaim.To != nil:
		// ToIsOpen=true excludes the to-window: upper retreats to the
		// window's start. Default closed-upper extends to the window's end.
		t, errE := workClaim.To.WindowEndFloat64(*workClaim.ToPrecision, workClaim.ToIsOpen)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		to = &t
		toDisplay = workClaim.To.String()
		rangeFloat.LessThan = &t
	case workClaim.ToIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave to and toDisplay empty.
		// But we want to find it always when we search by range.
		t := math.MaxFloat64
		rangeFloat.LessThanOrEqual = &t
	case workClaim.ToIsUnknown && workClaim.From != nil:
		// Unknown To with known From: treat as single point at From.
		claims, errE := c.convertAmount(ctx, &document.AmountClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
			Amount:    *workClaim.From,
			Precision: *workClaim.FromPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, subClaims{}, errE
	default:
		// Unknown To with None From. We cannot do much here,
		// so we convert it as an unknown claim.
		claims, subs, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
		})
		return nil, claims, subs, errE
	}

	errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}

	// TODO: Normalize amounts of units of same measure to same base unit (e.g., cm and mm to m).
	unit := c.extractInUnit(workClaim.Sub)
	props := c.propagateProp(workClaim.Prop.ID)
	result := make([]AmountClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		result = append(result, AmountClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Unit:        unit,
			Range:       rangeFloat,
			From:        from,
			FromDisplay: fromDisplay,
			To:          to,
			ToDisplay:   toDisplay,
		})
	}
	return result, nil, subClaims{}, nil
}

func (c *Converter) convertTime(ctx context.Context, claim *document.TimeClaim) ([]TimeClaim, errors.E) {
	from, errE := claim.Time.WindowStartFloat64(claim.Precision, false)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	to, errE := claim.Time.WindowEndFloat64(claim.Precision, false)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	display := claim.Time.String()

	rangeFrom := from
	rangeTo := to
	rangeFloat := RangeFloat{ //nolint:exhaustruct
		GreaterThanOrEqual: &rangeFrom,
		LessThan:           &rangeTo,
	}

	// Sanity check. Validate is strict and never swaps; for a single-point
	// time with non-zero precision the bounds are always well-formed.
	errE = rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	props := c.propagateProp(claim.Prop.ID)
	result := make([]TimeClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, TimeClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Range:       rangeFloat,
			From:        &from,
			FromDisplay: display,
			To:          &to,
			ToDisplay:   display,
		})
	}
	return result, nil
}

//nolint:cyclop
func (c *Converter) convertTimeInterval(ctx context.Context, claim *document.TimeIntervalClaim) ([]TimeClaim, []UnknownClaim, subClaims, errors.E) {
	if claim.From != nil && claim.FromPrecision == nil {
		errE := errors.New("missing from precision in claim")
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}
	if claim.To != nil && claim.ToPrecision == nil {
		errE := errors.New("missing to precision in claim")
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}

	// Swap directed-decreasing input to ascending order, before computing
	// rangeFloat bounds. Dual criterion (matches document.TimeIntervalClaim.Validate):
	// same precision -> swap on value, different precision -> swap iff the
	// un-swapped form is empty. See convertAmountInterval for the full
	// rationale.
	workClaim := *claim
	if workClaim.From != nil && workClaim.To != nil {
		var swap bool
		if *workClaim.FromPrecision == *workClaim.ToPrecision {
			fromValue, errE := workClaim.From.Float64(*workClaim.FromPrecision, nil)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			toValue, errE := workClaim.To.Float64(*workClaim.ToPrecision, nil)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			swap = fromValue > toValue
		} else {
			start, errE := workClaim.From.WindowStartFloat64(*workClaim.FromPrecision, workClaim.FromIsOpen)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			end, errE := workClaim.To.WindowEndFloat64(*workClaim.ToPrecision, workClaim.ToIsOpen)
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, nil, subClaims{}, errE
			}
			swap = start >= end
		}
		if swap {
			workClaim.From, workClaim.To = workClaim.To, workClaim.From
			workClaim.FromPrecision, workClaim.ToPrecision = workClaim.ToPrecision, workClaim.FromPrecision
			workClaim.FromIsOpen, workClaim.ToIsOpen = workClaim.ToIsOpen, workClaim.FromIsOpen
		}
	}

	var (
		rangeFloat  RangeFloat
		from, to    *float64
		fromDisplay string
		toDisplay   string
	)

	//nolint:dupl
	switch {
	case workClaim.From != nil:
		// FromIsOpen=true excludes the from-window: lower advances past the
		// window's end. Default closed-lower uses the window's start.
		f, errE := workClaim.From.WindowStartFloat64(*workClaim.FromPrecision, workClaim.FromIsOpen)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		from = &f
		fromDisplay = workClaim.From.String()
		rangeFloat.GreaterThanOrEqual = &f
	case workClaim.FromIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave from and fromDisplay empty.
		// But we want to find it always when we search by range.
		f := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &f
	case workClaim.FromIsUnknown && workClaim.To != nil:
		// Unknown From with known To: treat as single point at To.
		claims, errE := c.convertTime(ctx, &document.TimeClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
			Time:      *workClaim.To,
			Precision: *workClaim.ToPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, subClaims{}, errE
	default:
		// Unknown From with Unknown or None To. We cannot do much here,
		// so we convert it as an unknown claim. This also handles the case
		// of invalid claims (e.g., an empty claim without anything set).
		claims, subs, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
		})
		return nil, claims, subs, errE
	}

	switch {
	case workClaim.To != nil:
		// ToIsOpen=true excludes the to-window: upper retreats to the
		// window's start. Default closed-upper extends to the window's end.
		t, errE := workClaim.To.WindowEndFloat64(*workClaim.ToPrecision, workClaim.ToIsOpen)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		to = &t
		toDisplay = workClaim.To.String()
		rangeFloat.LessThan = &t
	case workClaim.ToIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave to and toDisplay empty.
		// But we want to find it always when we search by range.
		t := math.MaxFloat64
		rangeFloat.LessThanOrEqual = &t
	case workClaim.ToIsUnknown && workClaim.From != nil:
		// Unknown To with known From: treat as single point at From.
		claims, errE := c.convertTime(ctx, &document.TimeClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
			Time:      *workClaim.From,
			Precision: *workClaim.FromPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, subClaims{}, errE
	default:
		// Unknown To with None From. We cannot do much here,
		// so we convert it as an unknown claim.
		claims, subs, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: workClaim.CoreClaim,
			Prop:      workClaim.Prop,
		})
		return nil, claims, subs, errE
	}

	errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, subClaims{}, errE
	}

	props := c.propagateProp(workClaim.Prop.ID)
	result := make([]TimeClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, subClaims{}, errE
		}
		result = append(result, TimeClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Range:       rangeFloat,
			From:        from,
			FromDisplay: fromDisplay,
			To:          to,
			ToDisplay:   toDisplay,
		})
	}
	return result, nil, subClaims{}, nil
}

// Sentinel values for sub-claim ParentTo when the parent claim is not a reference claim.
const (
	ParentToHas     = "__HAS__"
	ParentToNone    = "__NONE__"
	ParentToUnknown = "__UNKNOWN__"
)

// subClaims aggregates every per-document Sub* record produced by extracting
// sub-claims from a parent claim's sub-tree.
type subClaims struct {
	Refs    []SubRefClaim
	Amounts []SubAmountClaim
	Times   []SubTimeClaim
	Has     []SubHasClaim
}

func (s *subClaims) append(other subClaims) {
	s.Refs = append(s.Refs, other.Refs...)
	s.Amounts = append(s.Amounts, other.Amounts...)
	s.Times = append(s.Times, other.Times...)
	s.Has = append(s.Has, other.Has...)
}

// extractSubClaims walks a parent claim's sub-tree and produces every Sub*
// record (reference, amount, time, has) keyed by the given parentProps and
// parentTo. Each parent prop in parentProps yields its own copy of the
// records, mirroring the property propagation behaviour the parent claim
// already applied.
func (c *Converter) extractSubClaims(
	ctx context.Context, sub *document.ClaimTypes, parentProps []identifier.Identifier, parentTo string,
) (subClaims, errors.E) {
	var result subClaims
	refs, errE := c.convertSubRefs(ctx, sub, parentProps, parentTo)
	if errE != nil {
		return subClaims{}, errE
	}
	result.Refs = refs
	amounts, errE := c.convertSubAmounts(ctx, sub, parentProps, parentTo)
	if errE != nil {
		return subClaims{}, errE
	}
	result.Amounts = amounts
	times, errE := c.convertSubTimes(ctx, sub, parentProps, parentTo)
	if errE != nil {
		return subClaims{}, errE
	}
	result.Times = times
	has, errE := c.convertSubHas(ctx, sub, parentProps, parentTo)
	if errE != nil {
		return subClaims{}, errE
	}
	result.Has = has
	return result, nil
}

// convertSubRefs extracts reference sub-claims from a claim's sub-claims and
// produces SubRefClaim entries for each (parentProp, parentTo, subRef)
// combination.
func (c *Converter) convertSubRefs(
	ctx context.Context, sub *document.ClaimTypes, parentProps []identifier.Identifier, parentTo string,
) ([]SubRefClaim, errors.E) {
	subRelations := document.GetAllClaimsOfTypeWithConfidence[document.ReferenceClaim](sub, document.LowConfidence)
	if len(subRelations) == 0 {
		return nil, nil
	}

	// Pre-resolve display strings and hierarchy paths for each sub-relation.
	type resolvedSubRef struct {
		Prop          identifier.Identifier
		PropDisplay   map[string]string
		PropNaming    map[string][]string
		To            identifier.Identifier
		ToDisplay     map[string]string
		ToNaming      map[string][]string
		ToPath        []string
		ToDisplayPath map[string][]string
	}
	resolved := make([]resolvedSubRef, 0, len(subRelations))
	for _, mr := range subRelations {
		mrPropDisplay, errE := c.getDisplayStrings(ctx, mr.Prop.ID)
		if errE != nil {
			return nil, errE
		}
		mrToInfo, errE := c.getDocumentInfo(ctx, mr.To.ID)
		if errE != nil {
			return nil, errE
		}
		toPath, toDisplayPath := mrToInfo.CollectHierarchyPaths()
		resolved = append(resolved, resolvedSubRef{
			Prop:          mr.Prop.ID,
			PropDisplay:   mrPropDisplay.Display,
			PropNaming:    mrPropDisplay.Naming,
			To:            mr.To.ID,
			ToDisplay:     mrToInfo.Display.Display,
			ToNaming:      mrToInfo.Display.Naming,
			ToPath:        toPath,
			ToDisplayPath: toDisplayPath,
		})
	}

	// Cross product of parentProps x resolved sub-refs.
	result := make([]SubRefClaim, 0, len(parentProps)*len(resolved))
	for _, pp := range parentProps {
		for _, r := range resolved {
			result = append(result, SubRefClaim{
				ParentProp:    pp,
				ParentTo:      parentTo,
				Prop:          r.Prop,
				PropDisplay:   r.PropDisplay,
				PropNaming:    r.PropNaming,
				To:            r.To,
				ToDisplay:     r.ToDisplay,
				ToNaming:      r.ToNaming,
				ToPath:        r.ToPath,
				ToDisplayPath: r.ToDisplayPath,
			})
		}
	}

	return result, nil
}

// convertSubAmounts extracts amount and amount-interval sub-claims from a
// parent claim's sub-tree and produces SubAmountClaim entries for each
// (parentProp, parentTo, sub-claim) combination. Source claims are passed
// through convertAmount/convertAmountInterval so single-point and interval
// values use the same indexed range shape as the top-level amounts.
func (c *Converter) convertSubAmounts(
	ctx context.Context, sub *document.ClaimTypes, parentProps []identifier.Identifier, parentTo string,
) ([]SubAmountClaim, errors.E) {
	var indexed []AmountClaim
	for _, ac := range document.GetAllClaimsOfTypeWithConfidence[document.AmountClaim](sub, document.LowConfidence) {
		ic, errE := c.convertAmount(ctx, ac)
		if errE != nil {
			return nil, errE
		}
		indexed = append(indexed, ic...)
	}
	for _, aic := range document.GetAllClaimsOfTypeWithConfidence[document.AmountIntervalClaim](sub, document.LowConfidence) {
		// Sub-claims do not get a fallback UnknownClaim representation, and any
		// nested sub-refs inside a sub-claim are not flattened a second level
		// up. Discard the second and third return values.
		ic, _, _, errE := c.convertAmountInterval(ctx, aic)
		if errE != nil {
			return nil, errE
		}
		indexed = append(indexed, ic...)
	}
	if len(indexed) == 0 {
		return nil, nil
	}

	result := make([]SubAmountClaim, 0, len(parentProps)*len(indexed))
	for _, pp := range parentProps {
		for _, a := range indexed {
			result = append(result, SubAmountClaim{
				ParentProp:  pp,
				ParentTo:    parentTo,
				Prop:        a.Prop,
				PropDisplay: a.PropDisplay,
				PropNaming:  a.PropNaming,
				Unit:        a.Unit,
				Range:       a.Range,
				From:        a.From,
				FromDisplay: a.FromDisplay,
				To:          a.To,
				ToDisplay:   a.ToDisplay,
			})
		}
	}
	return result, nil
}

// convertSubTimes extracts time and time-interval sub-claims from a parent
// claim's sub-tree and produces SubTimeClaim entries for each (parentProp,
// parentTo, sub-claim) combination. Source claims are passed through
// convertTime/convertTimeInterval for the same single-point-or-interval
// range mapping the top-level times use.
func (c *Converter) convertSubTimes(
	ctx context.Context, sub *document.ClaimTypes, parentProps []identifier.Identifier, parentTo string,
) ([]SubTimeClaim, errors.E) {
	var indexed []TimeClaim
	for _, tc := range document.GetAllClaimsOfTypeWithConfidence[document.TimeClaim](sub, document.LowConfidence) {
		ic, errE := c.convertTime(ctx, tc)
		if errE != nil {
			return nil, errE
		}
		indexed = append(indexed, ic...)
	}
	for _, tic := range document.GetAllClaimsOfTypeWithConfidence[document.TimeIntervalClaim](sub, document.LowConfidence) {
		ic, _, _, errE := c.convertTimeInterval(ctx, tic)
		if errE != nil {
			return nil, errE
		}
		indexed = append(indexed, ic...)
	}
	if len(indexed) == 0 {
		return nil, nil
	}

	result := make([]SubTimeClaim, 0, len(parentProps)*len(indexed))
	for _, pp := range parentProps {
		for _, t := range indexed {
			result = append(result, SubTimeClaim{
				ParentProp:  pp,
				ParentTo:    parentTo,
				Prop:        t.Prop,
				PropDisplay: t.PropDisplay,
				PropNaming:  t.PropNaming,
				Range:       t.Range,
				From:        t.From,
				FromDisplay: t.FromDisplay,
				To:          t.To,
				ToDisplay:   t.ToDisplay,
			})
		}
	}
	return result, nil
}

// convertSubHas extracts simple has sub-claims (those with no further
// sub-claims) from a parent claim's sub-tree and produces SubHasClaim entries
// for each (parentProp, parentTo, hasProp) combination. Has sub-claims that
// have their own sub-claims contribute to the Sub* records of their content
// types but do not themselves appear here.
func (c *Converter) convertSubHas(
	ctx context.Context, sub *document.ClaimTypes, parentProps []identifier.Identifier, parentTo string,
) ([]SubHasClaim, errors.E) {
	subHas := document.GetAllClaimsOfTypeWithConfidence[document.HasClaim](sub, document.LowConfidence)
	if len(subHas) == 0 {
		return nil, nil
	}

	type resolvedSubHas struct {
		Prop        identifier.Identifier
		PropDisplay map[string]string
		PropNaming  map[string][]string
	}
	resolved := make([]resolvedSubHas, 0, len(subHas))
	for _, hc := range subHas {
		if hc.Sub != nil && hc.Sub.Size() > 0 {
			continue
		}
		propIDs := c.propagateProp(hc.Prop.ID)
		for _, pid := range propIDs {
			propDisplay, errE := c.getDisplayStrings(ctx, pid)
			if errE != nil {
				return nil, errE
			}
			resolved = append(resolved, resolvedSubHas{
				Prop:        pid,
				PropDisplay: propDisplay.Display,
				PropNaming:  propDisplay.Naming,
			})
		}
	}
	if len(resolved) == 0 {
		return nil, nil
	}

	result := make([]SubHasClaim, 0, len(parentProps)*len(resolved))
	for _, pp := range parentProps {
		for _, r := range resolved {
			result = append(result, SubHasClaim{
				ParentProp:  pp,
				ParentTo:    parentTo,
				Prop:        r.Prop,
				PropDisplay: r.PropDisplay,
				PropNaming:  r.PropNaming,
			})
		}
	}
	return result, nil
}

func (c *Converter) convertReference(ctx context.Context, claim *document.ReferenceClaim) ([]ReferenceClaim, subClaims, errors.E) {
	// Cross product of propagated properties x (target + value hierarchy ancestors).
	props := c.propagateProp(claim.Prop.ID)

	// Compute target IDs: the target itself plus ancestors from all value hierarchies.
	targetInfo, errE := c.getDocumentInfo(ctx, claim.To.ID)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, subClaims{}, errE
	}
	targets := []identifier.Identifier{claim.To.ID}
	seen := map[identifier.Identifier]bool{claim.To.ID: true}
	for _, ancestors := range targetInfo.Ancestors {
		for _, aid := range ancestors {
			if !seen[aid] {
				seen[aid] = true
				targets = append(targets, aid)
			}
		}
	}

	result := make([]ReferenceClaim, 0, len(props)*len(targets))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, subClaims{}, errE
		}
		for _, tid := range targets {
			var tidInfo documentInfo
			if tid == claim.To.ID {
				tidInfo = targetInfo
			} else {
				// Ancestors were already computed during the hierarchy walk, so this hits cache.
				tidInfo, errE = c.getDocumentInfo(ctx, tid)
				if errE != nil {
					errors.Details(errE)["claim"] = claim
					return nil, subClaims{}, errE
				}
			}
			// Collect hierarchy paths across all value hierarchy types.
			toPath, toDisplayPath := tidInfo.CollectHierarchyPaths()
			result = append(result, ReferenceClaim{
				Prop:          pid,
				PropDisplay:   propDisplay.Display,
				PropNaming:    propDisplay.Naming,
				To:            tid,
				ToDisplay:     tidInfo.Display.Display,
				ToNaming:      tidInfo.Display.Naming,
				ToPath:        toPath,
				ToDisplayPath: toDisplayPath,
			})
		}
	}

	// Generate Sub* entries for each (expandedProp, expandedTarget) x sub-claim combination.
	var allSubs subClaims
	for _, pid := range props {
		for _, tid := range targets {
			subs, errE := c.extractSubClaims(ctx, claim.Sub, []identifier.Identifier{pid}, tid.String())
			if errE != nil {
				errors.Details(errE)["claim"] = claim
				return nil, subClaims{}, errE
			}
			allSubs.append(subs)
		}
	}

	return result, allSubs, nil
}

func (c *Converter) convertHas(ctx context.Context, claim *document.HasClaim) ([]HasClaim, subClaims, errors.E) {
	props := c.propagateProp(claim.Prop.ID)

	subs, errE := c.extractSubClaims(ctx, claim.Sub, props, ParentToHas)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, subClaims{}, errE
	}

	// A has claim that carries any sub-claims is not indexed in claims.has;
	// its content is already reachable through the Sub* records produced
	// above with ParentTo=ParentToHas. The has filter that queries claims.has
	// therefore naturally sees only simple has claims.
	if claim.Sub != nil && claim.Sub.Size() > 0 {
		return nil, subs, nil
	}

	result := make([]HasClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, subClaims{}, errE
		}
		result = append(result, HasClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
		})
	}
	return result, subs, nil
}

func (c *Converter) convertNone(ctx context.Context, claim *document.NoneClaim) ([]NoneClaim, subClaims, errors.E) {
	props := c.propagateProp(claim.Prop.ID)

	subs, errE := c.extractSubClaims(ctx, claim.Sub, props, ParentToNone)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, subClaims{}, errE
	}

	result := make([]NoneClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, subClaims{}, errE
		}
		result = append(result, NoneClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
		})
	}
	return result, subs, nil
}

func (c *Converter) convertUnknown(ctx context.Context, claim *document.UnknownClaim) ([]UnknownClaim, subClaims, errors.E) {
	props := c.propagateProp(claim.Prop.ID)

	subs, errE := c.extractSubClaims(ctx, claim.Sub, props, ParentToUnknown)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, subClaims{}, errE
	}

	result := make([]UnknownClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, subClaims{}, errE
		}
		result = append(result, UnknownClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
		})
	}
	return result, subs, nil
}
