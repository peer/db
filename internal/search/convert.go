package search

import (
	"bytes"
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalDocument "gitlab.com/peerdb/peerdb/internal/document"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

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
	Hooks []func(doc *document.D) (*document.D, errors.E)
	// LanguageCodes is a map that maps language document ID to primary language subtag (e.g., "en").
	LanguageCodes map[identifier.Identifier]string

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
	// Ensure all supported languages are keys in languagePriority, matching the
	// frontend pattern where languagePriority keys define the set of enabled
	// languages. On the backend, we enable all supported languages.
	// Languages without explicit fallbacks get default fallback behavior.
	fullPriority := make(map[string][]string, len(SupportedLanguages))
	for lang := range SupportedLanguages {
		if fallbacks, ok := languagePriority[lang]; ok {
			fullPriority[lang] = fallbacks
		} else if lang != document.UndeterminedLanguage {
			fullPriority[lang] = []string{document.UndeterminedLanguage}
		} else {
			fullPriority[lang] = nil
		}
	}
	c := &Converter{
		Hooks:                    nil,
		LanguageCodes:            nil,
		propertyDescendants:      nil,
		propertyAncestors:        nil,
		valueHierarchyProperties: nil,
		namingProperties:         nil,
		inverseProperties:        nil,
		fieldInverseProperties:   nil,
		languagePriority:         fullPriority,
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
		ids := document.GetClaimsOfTypeWithConfidence[document.IdentifierClaim](doc, internalCore.CodePropID, document.LowConfidence)
		for _, id := range ids {
			code, _, _ := strings.Cut(id.Value, "-")
			if SupportedLanguages[code] {
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
	for lang := range SupportedLanguages {
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

	for lang := range SupportedLanguages {
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

	// For each supported language, find the best template using the fallback chain.
	result := make(map[string]string, len(SupportedLanguages))
	for lang := range SupportedLanguages {
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
		if code, ok := c.LanguageCodes[ref.To.ID]; ok && SupportedLanguages[code] {
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

// propagateProp returns the property IDs to create claims for:
// the original property plus all its transitive super-properties.
// If X is a sub-property of Y, a claim for X also produces a claim for Y.
func (c *Converter) propagateProp(propID identifier.Identifier) []identifier.Identifier {
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

// VisitIdentifier converts an identifier claim to search identifier claims.
func (v *convertVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertIdentifier(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Identifier = append(v.result.Claims.Identifier, claims...)
	return document.Keep, nil
}

// VisitString converts a string claim to search string claims.
func (v *convertVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertString(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.String = append(v.result.Claims.String, claims...)
	return document.Keep, nil
}

// VisitHTML converts an HTML claim to search HTML claims.
func (v *convertVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertHTML(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.HTML = append(v.result.Claims.HTML, claims...)
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
	amountClaims, unknownClaims, errE := v.converter.convertAmountInterval(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Amount = append(v.result.Claims.Amount, amountClaims...)
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, unknownClaims...)
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
	timeClaims, unknownClaims, errE := v.converter.convertTimeInterval(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Time = append(v.result.Claims.Time, timeClaims...)
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, unknownClaims...)
	return document.Keep, nil
}

// VisitLink converts a link claim to search link claims.
func (v *convertVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertLink(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Link = append(v.result.Claims.Link, claims...)
	return document.Keep, nil
}

// VisitReference converts a reference claim to search reference claims.
func (v *convertVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertReference(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Reference = append(v.result.Claims.Reference, claims...)

	return document.Keep, nil
}

// VisitHas converts a has claim to search has claims.
func (v *convertVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertHas(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Has = append(v.result.Claims.Has, claims...)
	return document.Keep, nil
}

// VisitNone converts a none claim to search none claims.
func (v *convertVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertNone(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.None = append(v.result.Claims.None, claims...)
	return document.Keep, nil
}

// VisitUnknown converts an unknown claim to search unknown claims.
func (v *convertVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}
	claims, errE := v.converter.convertUnknown(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, claims...)
	return document.Keep, nil
}

// FromDocument converts a document.D to a search Document.
//
// inverseRelations contains reference claims from other documents that point to this document.
// For each inverse relation, a synthetic reverse reference claim is added to the search document.
func (c *Converter) FromDocument(
	ctx context.Context, doc *document.D, inverseRelations []internalStore.InverseRelation,
) (*Document, errors.E) {
	var errE errors.E
	for i, hook := range c.Hooks {
		doc, errE = hook(doc)
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
			ID:     doc.ID,
			Claims: ClaimTypes{},
		},
		docID: doc.ID,
	}

	errE = doc.Visit(v)
	if errE != nil {
		return nil, errE
	}

	// Process incoming inverse relations from metadata.
	for _, ir := range inverseRelations {
		claims, errE := c.convertReference(ctx, &document.ReferenceClaim{
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
	}

	return v.result, nil
}

// inverseReferenceClaimID computes an unique claim ID for a synthetic inverse reference claim.
//
// It uses InverseRelationKey to avoid collisions between claims from
// different source documents that might share the same claim ID.
func inverseReferenceClaimID(base []string, irKey internalStore.InverseRelationKey) identifier.Identifier {
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
	result map[identifier.Identifier][]internalStore.InverseRelation
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
		v.result[claim.To.ID] = append(v.result[claim.To.ID], internalStore.InverseRelation{
			InverseRelationKey: internalStore.InverseRelationKey{
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
			v.result[claim.To.ID] = append(v.result[claim.To.ID], internalStore.InverseRelation{
				InverseRelationKey: internalStore.InverseRelationKey{
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
func (c *Converter) OutgoingInverseRelations(doc *document.D) map[identifier.Identifier][]internalStore.InverseRelation {
	v := &inverseRelationsVisitor{
		converter: c,
		docID:     doc.ID,
		path:      nil,
		result:    map[identifier.Identifier][]internalStore.InverseRelation{},
	}
	// Visit cannot return an error from inverseRelationsVisitor.
	_ = doc.Visit(v)
	return v.result
}

func (c *Converter) convertIdentifier(ctx context.Context, claim *document.IdentifierClaim) ([]IdentifierClaim, errors.E) {
	props := c.propagateProp(claim.Prop.ID)
	result := make([]IdentifierClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, IdentifierClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Value:       claim.Value,
		})
	}
	return result, nil
}

func (c *Converter) convertString(ctx context.Context, claim *document.StringClaim) ([]StringClaim, errors.E) {
	str := map[string]string{}
	for _, lang := range c.extractInLanguages(claim.Sub) {
		str[lang] = claim.String
	}
	props := c.propagateProp(claim.Prop.ID)
	result := make([]StringClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, StringClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			String:      str,
		})
	}
	return result, nil
}

func (c *Converter) convertHTML(ctx context.Context, claim *document.HTMLClaim) ([]HTMLClaim, errors.E) {
	html := map[string]string{}
	for _, lang := range c.extractInLanguages(claim.Sub) {
		html[lang] = claim.HTML
	}
	props := c.propagateProp(claim.Prop.ID)
	result := make([]HTMLClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, HTMLClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			HTML:        html,
		})
	}
	return result, nil
}

func (c *Converter) convertAmount(ctx context.Context, claim *document.AmountClaim) ([]AmountClaim, errors.E) {
	// TODO: Normalize amounts of units of same measure to same base unit (e.g., cm and mm to m).
	unit := c.extractInUnit(claim.Sub)
	amount, errE := claim.Amount.Float64(claim.Precision)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	from := amount - claim.Precision/2 //nolint:mnd
	to := amount + claim.Precision/2   //nolint:mnd
	display := claim.Amount.String()

	// We use separate variables for the range bounds to avoid aliasing
	// with from/to (swapping from/to would mutate the range values).
	rangeFrom := from
	rangeTo := to
	rangeFloat := RangeFloat{ //nolint:exhaustruct
		GreaterThanOrEqual: &rangeFrom,
		LessThan:           &rangeTo,
	}

	// Sanity check.
	swapped, errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	if swapped {
		from, to = to, from
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

func (c *Converter) convertAmountInterval(ctx context.Context, claim *document.AmountIntervalClaim) ([]AmountClaim, []UnknownClaim, errors.E) { //nolint:cyclop
	var (
		rangeFloat  RangeFloat
		from, to    *float64
		fromDisplay string
		toDisplay   string
	)

	switch {
	case claim.From != nil:
		if claim.FromPrecision == nil {
			errE := errors.New("missing from precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// TODO: How to integrate precision besides validation?
		fromValue, errE := claim.From.Float64(*claim.FromPrecision)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		from = &fromValue
		fromDisplay = claim.From.String()
		if claim.FromIsOpen {
			rangeFloat.GreaterThan = &fromValue
		} else {
			rangeFloat.GreaterThanOrEqual = &fromValue
		}
	case claim.FromIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave from and fromDisplay empty.
		// But we want to find it always when we search by range.
		f := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &f
	case claim.FromIsUnknown && claim.To != nil:
		if claim.ToPrecision == nil {
			errE := errors.New("missing to precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// Unknown From with known To: treat as single point at To.
		claims, errE := c.convertAmount(ctx, &document.AmountClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
			Amount:    *claim.To,
			Precision: *claim.ToPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, errE
	default:
		// Unknown From with Unknown or None To. We cannot do much here,
		// so we convert it as an unknown claim. This also handles the case
		// of invalid claims (e.g., an empty claim without anything set).
		claims, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
		})
		return nil, claims, errE
	}

	switch {
	case claim.To != nil:
		if claim.ToPrecision == nil {
			errE := errors.New("missing to precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// TODO: How to integrate precision besides display?
		toValue, errE := claim.To.Float64(*claim.ToPrecision)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		to = &toValue
		toDisplay = claim.To.String()
		if claim.ToIsOpen {
			rangeFloat.LessThan = &toValue
		} else {
			rangeFloat.LessThanOrEqual = &toValue
		}
	case claim.ToIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave to and toDisplay empty.
		// But we want to find it always when we search by range.
		t := math.MaxFloat64
		rangeFloat.LessThanOrEqual = &t
	case claim.ToIsUnknown && claim.From != nil:
		if claim.FromPrecision == nil {
			errE := errors.New("missing from precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// Unknown To with known From: treat as single point at From.
		claims, errE := c.convertAmount(ctx, &document.AmountClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
			Amount:    *claim.From,
			Precision: *claim.FromPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, errE
	default:
		// Unknown To with None From. We cannot do much here,
		// so we convert it as an unknown claim.
		claims, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
		})
		return nil, claims, errE
	}

	// If To and From are the same, treat as single point.
	if from != nil && to != nil && *from == *to {
		claims, errE := c.convertAmount(ctx, &document.AmountClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
			Amount:    *claim.From,
			// Smaller float64 is more precise.
			Precision: min(*claim.FromPrecision, *claim.ToPrecision),
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, errE
	}

	// Sanity check.
	swapped, errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, errE
	}
	if swapped {
		from, to = to, from
		fromDisplay, toDisplay = toDisplay, fromDisplay
	}

	// TODO: Normalize amounts of units of same measure to same base unit (e.g., cm and mm to m).
	unit := c.extractInUnit(claim.Sub)
	props := c.propagateProp(claim.Prop.ID)
	result := make([]AmountClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
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
	return result, nil, nil
}

func (c *Converter) convertTime(ctx context.Context, claim *document.TimeClaim) ([]TimeClaim, errors.E) {
	t, errE := claim.Time.Time(claim.Precision, time.UTC)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	from := x.TimeToFloat64(t)
	to := x.TimeToFloat64(addPrecision(t, claim.Precision))
	display := claim.Time.String()

	// We use separate variables for the range bounds to avoid aliasing
	// with from/to (swapping from/to would mutate the range values).
	rangeFrom := from
	rangeTo := to
	rangeFloat := RangeFloat{ //nolint:exhaustruct
		GreaterThanOrEqual: &rangeFrom,
		LessThan:           &rangeTo,
	}

	// Sanity check.
	swapped, errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	if swapped {
		from, to = to, from
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

func (c *Converter) convertTimeInterval(ctx context.Context, claim *document.TimeIntervalClaim) ([]TimeClaim, []UnknownClaim, errors.E) { //nolint:cyclop
	var (
		rangeFloat  RangeFloat
		from, to    *float64
		fromDisplay string
		toDisplay   string
	)

	switch {
	case claim.From != nil:
		if claim.FromPrecision == nil {
			errE := errors.New("missing from precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		tm, errE := claim.From.Time(*claim.FromPrecision, time.UTC)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// FromIsOpen=true excludes the from-window, advancing the lower
		// bound to from_end; default closed-lower uses from_start.
		var f float64
		if claim.FromIsOpen {
			f = x.TimeToFloat64(addPrecision(tm, *claim.FromPrecision))
		} else {
			f = x.TimeToFloat64(tm)
		}
		from = &f
		fromDisplay = claim.From.String()
		rangeFloat.GreaterThanOrEqual = &f
	case claim.FromIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave from and fromDisplay empty.
		// But we want to find it always when we search by range.
		f := -math.MaxFloat64
		rangeFloat.GreaterThanOrEqual = &f
	case claim.FromIsUnknown && claim.To != nil:
		if claim.ToPrecision == nil {
			errE := errors.New("missing to precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// Unknown From with known To: treat as single point at To.
		claims, errE := c.convertTime(ctx, &document.TimeClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
			Time:      *claim.To,
			Precision: *claim.ToPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, errE
	default:
		// Unknown From with Unknown or None To. We cannot do much here,
		// so we convert it as an unknown claim. This also handles the case
		// of invalid claims (e.g., an empty claim without anything set).
		claims, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
		})
		return nil, claims, errE
	}

	switch {
	case claim.To != nil:
		if claim.ToPrecision == nil {
			errE := errors.New("missing to precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		tm, errE := claim.To.Time(*claim.ToPrecision, time.UTC)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// ToIsOpen=true excludes the to-window, pulling the upper bound
		// back to to_start; default closed-upper extends to to_end so the
		// to-window is fully included.
		var t float64
		if claim.ToIsOpen {
			t = x.TimeToFloat64(tm)
		} else {
			t = x.TimeToFloat64(addPrecision(tm, *claim.ToPrecision))
		}
		to = &t
		toDisplay = claim.To.String()
		rangeFloat.LessThan = &t
	case claim.ToIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave to and toDisplay empty.
		// But we want to find it always when we search by range.
		t := math.MaxFloat64
		rangeFloat.LessThanOrEqual = &t
	case claim.ToIsUnknown && claim.From != nil:
		if claim.FromPrecision == nil {
			errE := errors.New("missing from precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// Unknown To with known From: treat as single point at From.
		claims, errE := c.convertTime(ctx, &document.TimeClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
			Time:      *claim.From,
			Precision: *claim.FromPrecision,
		})
		if errE != nil {
			errors.Details(errE)["claim"] = claim
		}
		return claims, nil, errE
	default:
		// Unknown To with None From. We cannot do much here,
		// so we convert it as an unknown claim.
		claims, errE := c.convertUnknown(ctx, &document.UnknownClaim{
			CoreClaim: claim.CoreClaim,
			Prop:      claim.Prop,
		})
		return nil, claims, errE
	}

	// Sanity check.
	swapped, errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, errE
	}
	if swapped {
		from, to = to, from
		fromDisplay, toDisplay = toDisplay, fromDisplay
	}

	props := c.propagateProp(claim.Prop.ID)
	result := make([]TimeClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
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
	return result, nil, nil
}

func (c *Converter) convertLink(ctx context.Context, claim *document.LinkClaim) ([]LinkClaim, errors.E) {
	props := c.propagateProp(claim.Prop.ID)
	result := make([]LinkClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, LinkClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			IRI:         claim.IRI,
		})
	}
	return result, nil
}

func (c *Converter) convertReference(ctx context.Context, claim *document.ReferenceClaim) ([]ReferenceClaim, errors.E) {
	// Convert sub-claim references to nested search reference claims.
	subRelations := document.GetAllClaimsOfTypeWithConfidence[document.ReferenceClaim](claim.Sub, document.LowConfidence)
	nested := make([]NestedReferenceClaim, 0, len(subRelations))
	for _, mr := range subRelations {
		mrPropDisplay, errE := c.getDisplayStrings(ctx, mr.Prop.ID)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		mrToDisplay, errE := c.getDisplayStrings(ctx, mr.To.ID)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		nested = append(nested, NestedReferenceClaim{
			Prop:        mr.Prop.ID,
			PropDisplay: mrPropDisplay.Display,
			PropNaming:  mrPropDisplay.Naming,
			To:          mr.To.ID,
			ToDisplay:   mrToDisplay.Display,
			ToNaming:    mrToDisplay.Naming,
		})
	}

	// Cross product of propagated properties x (target + value hierarchy ancestors).
	propIDs := c.propagateProp(claim.Prop.ID)

	// Compute target IDs: the target itself plus ancestors from all value hierarchies.
	targetInfo, errE := c.getDocumentInfo(ctx, claim.To.ID)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}
	targetIDs := []identifier.Identifier{claim.To.ID}
	seen := map[identifier.Identifier]bool{claim.To.ID: true}
	for _, ancestors := range targetInfo.Ancestors {
		for _, aid := range ancestors {
			if !seen[aid] {
				seen[aid] = true
				targetIDs = append(targetIDs, aid)
			}
		}
	}

	result := make([]ReferenceClaim, 0, len(propIDs)*len(targetIDs))
	for _, pid := range propIDs {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		for _, tid := range targetIDs {
			var tidInfo documentInfo
			if tid == claim.To.ID {
				tidInfo = targetInfo
			} else {
				// Ancestors were already computed during the hierarchy walk, so this hits cache.
				tidInfo, errE = c.getDocumentInfo(ctx, tid)
				if errE != nil {
					errors.Details(errE)["claim"] = claim
					return nil, errE
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
				Reference:     nested,
			})
		}
	}
	return result, nil
}

func (c *Converter) convertHas(ctx context.Context, claim *document.HasClaim) ([]HasClaim, errors.E) {
	// Convert sub-claim references to nested search reference claims.
	subRelations := document.GetAllClaimsOfTypeWithConfidence[document.ReferenceClaim](claim.Sub, document.LowConfidence)
	nested := make([]NestedReferenceClaim, 0, len(subRelations))
	for _, mr := range subRelations {
		mrPropDisplay, errE := c.getDisplayStrings(ctx, mr.Prop.ID)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		mrToDisplay, errE := c.getDisplayStrings(ctx, mr.To.ID)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		nested = append(nested, NestedReferenceClaim{
			Prop:        mr.Prop.ID,
			PropDisplay: mrPropDisplay.Display,
			PropNaming:  mrPropDisplay.Naming,
			To:          mr.To.ID,
			ToDisplay:   mrToDisplay.Display,
			ToNaming:    mrToDisplay.Naming,
		})
	}

	props := c.propagateProp(claim.Prop.ID)
	result := make([]HasClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, HasClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			Reference:   nested,
		})
	}
	return result, nil
}

func (c *Converter) convertNone(ctx context.Context, claim *document.NoneClaim) ([]NoneClaim, errors.E) {
	props := c.propagateProp(claim.Prop.ID)
	result := make([]NoneClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, NoneClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
		})
	}
	return result, nil
}

func (c *Converter) convertUnknown(ctx context.Context, claim *document.UnknownClaim) ([]UnknownClaim, errors.E) {
	props := c.propagateProp(claim.Prop.ID)
	result := make([]UnknownClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, UnknownClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
		})
	}
	return result, nil
}

// addPrecision returns the time at the end of the precision window
// starting at t. For example, year precision returns the start of the next year.
func addPrecision(t time.Time, precision internalDocument.TimePrecision) time.Time {
	switch precision { //nolint:exhaustive
	case document.TimePrecisionGigaYears:
		return t.AddDate(1_000_000_000, 0, 0) //nolint:mnd
	case document.TimePrecisionHundredMegaYears:
		return t.AddDate(100_000_000, 0, 0) //nolint:mnd
	case document.TimePrecisionTenMegaYears:
		return t.AddDate(10_000_000, 0, 0) //nolint:mnd
	case document.TimePrecisionMegaYears:
		return t.AddDate(1_000_000, 0, 0) //nolint:mnd
	case document.TimePrecisionHundredKiloYears:
		return t.AddDate(100_000, 0, 0) //nolint:mnd
	case document.TimePrecisionTenKiloYears:
		return t.AddDate(10_000, 0, 0) //nolint:mnd
	case document.TimePrecisionKiloYears:
		return t.AddDate(1_000, 0, 0) //nolint:mnd
	case document.TimePrecisionHundredYears:
		return t.AddDate(100, 0, 0) //nolint:mnd
	case document.TimePrecisionTenYears:
		return t.AddDate(10, 0, 0) //nolint:mnd
	case document.TimePrecisionYear:
		return t.AddDate(1, 0, 0)
	case document.TimePrecisionMonth:
		return t.AddDate(0, 1, 0)
	case document.TimePrecisionDay:
		return t.AddDate(0, 0, 1)
	case document.TimePrecisionHour:
		return t.Add(time.Hour)
	case document.TimePrecisionMinute:
		return t.Add(time.Minute)
	default:
		// Second and all subsecond precisions: we ignore subseconds,
		// so the range is always at least one second.
		return t.Add(time.Second)
	}
}
