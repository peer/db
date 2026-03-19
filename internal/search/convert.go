package search

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// LanguagePriority maps a language to its ordered fallback languages for display label resolution.
// If a language is not a key, fallback is only the undetermined language.
// If a language has an empty slice, no fallback is attempted at all.
type LanguagePriority map[string][]string

const undeterminedLanguage = "und"

// Well-known property and class IDs computed from the core namespace.
//
//nolint:gochecknoglobals
var (
	subentityOfPropID          = identifier.From("core.peerdb.org", "SUBENTITY_OF")
	subpropertyOfPropID        = identifier.From("core.peerdb.org", "SUBPROPERTY_OF")
	namingPropID               = identifier.From("core.peerdb.org", "NAMING")
	inLanguagePropID           = identifier.From("core.peerdb.org", "IN_LANGUAGE")
	inUnitPropID               = identifier.From("core.peerdb.org", "IN_UNIT")
	codePropID                 = identifier.From("core.peerdb.org", "CODE")
	instanceOfPropID           = identifier.From("core.peerdb.org", "INSTANCE_OF")
	propertyClassID            = identifier.From("core.peerdb.org", "PROPERTY")
	languageClassID            = identifier.From("core.peerdb.org", "LANGUAGE")
	displayLabelTemplatePropID = identifier.From("core.peerdb.org", "DISPLAY_LABEL_TEMPLATE")
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
				toDisplayPath = make(map[string][]string)
			}
			toDisplayPath[lang] = append(toDisplayPath[lang], paths...)
		}
	}
	return toPath, toDisplayPath
}

// Converter holds preprocessed data for converting document.D to search Document.
type Converter struct {
	// propertyDescendants maps a property ID to all its transitive sub-property IDs.
	propertyDescendants map[identifier.Identifier][]identifier.Identifier
	// propertyAncestors maps a property ID to all its transitive super-property IDs.
	propertyAncestors map[identifier.Identifier][]identifier.Identifier
	// valueHierarchyProperties lists hierarchy-defining property IDs for value expansion
	// (sub-properties of SUBENTITY_OF, excluding INSTANCE_OF and SUBPROPERTY_OF).
	valueHierarchyProperties []identifier.Identifier
	// namingProperties is the set of property IDs that are NAMING or sub-properties of NAMING.
	namingProperties map[identifier.Identifier]bool
	// languageCodes maps language document ID to primary language subtag (e.g., "en").
	languageCodes map[identifier.Identifier]string
	// mnemonics maps mnemonic to property ID.
	mnemonics map[string]identifier.Identifier
	// languagePriority defines per-language fallback order for display label resolution.
	languagePriority LanguagePriority
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
// needed for language code extraction, languagePriority defines per-language fallback order
// for display label resolution, and getDocument is a callback to fetch documents by ID.
// Value hierarchies (e.g., SUBCLASS_OF) are computed lazily during conversion.
func NewConverter(
	properties, languages []*document.D,
	mnemonics map[string]identifier.Identifier,
	languagePriority LanguagePriority,
	getDocument func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E),
) (*Converter, errors.E) {
	errE := validateLanguagePriority(languagePriority)
	if errE != nil {
		return nil, errE
	}
	c := &Converter{
		propertyDescendants:      nil,
		propertyAncestors:        nil,
		valueHierarchyProperties: nil,
		namingProperties:         nil,
		languageCodes:            nil,
		mnemonics:                mnemonics,
		languagePriority:         languagePriority,
		getDocument:              getDocument,
		documentInfoCache:        make(map[identifier.Identifier]documentInfo),
		documentInfoMu:           sync.RWMutex{},
	}
	c.buildPropertyHierarchy(properties)
	c.discoverValueHierarchyProperties()
	c.buildNamingProperties()
	c.buildLanguageCodes(languages)
	return c, nil
}

// validateLanguagePriority checks that all languages in priority are supported.
func validateLanguagePriority(priority LanguagePriority) errors.E {
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

// getFallbackLanguages returns the fallback language chain for a given language.
// If the language has an entry in languagePriority, that entry is used.
// If the language has no entry, the fallback is just the undetermined language
// (unless the language is itself undetermined).
func (c *Converter) getFallbackLanguages(lang string) []string {
	if c.languagePriority != nil {
		fallbacks, ok := c.languagePriority[lang]
		if ok {
			return fallbacks
		}
	}
	// Default: try undetermined language, unless lang is already undetermined.
	if lang != undeterminedLanguage {
		return []string{undeterminedLanguage}
	}
	return nil
}

// isInstanceOf returns true if the document has an INSTANCE_OF relation claim
// pointing to the given class ID.
func isInstanceOf(doc *document.D, classID identifier.Identifier) bool {
	for _, rel := range document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](doc, instanceOfPropID, document.LowConfidence) {
		if rel.To.ID == classID {
			return true
		}
	}
	return false
}

// buildPropertyHierarchy computes transitive descendants and ancestors for each property
// based on SUBPROPERTY_OF relation claims. Only documents that are instances of PROPERTY
// are considered.
func (c *Converter) buildPropertyHierarchy(properties []*document.D) {
	// Build parent -> children and child -> parents maps.
	// A property X with SUBPROPERTY_OF -> Y means X is a child (sub-property) of Y.
	parentChildren := make(map[identifier.Identifier][]identifier.Identifier)
	childParents := make(map[identifier.Identifier][]identifier.Identifier)
	for _, prop := range properties {
		if !isInstanceOf(prop, propertyClassID) {
			continue
		}
		for _, rel := range document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](prop, subpropertyOfPropID, document.LowConfidence) {
			parentChildren[rel.To.ID] = append(parentChildren[rel.To.ID], prop.ID)
			childParents[prop.ID] = append(childParents[prop.ID], rel.To.ID)
		}
	}

	// Compute transitive descendants for each property (used for naming properties).
	c.propertyDescendants = make(map[identifier.Identifier][]identifier.Identifier)
	for _, prop := range properties {
		visited := make(map[identifier.Identifier]bool)
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
	c.propertyAncestors = make(map[identifier.Identifier][]identifier.Identifier)
	for _, prop := range properties {
		visited := make(map[identifier.Identifier]bool)
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
	for _, desc := range c.propertyDescendants[subentityOfPropID] {
		if desc == instanceOfPropID || desc == subpropertyOfPropID {
			continue
		}
		c.valueHierarchyProperties = append(c.valueHierarchyProperties, desc)
	}
}

// buildNamingProperties computes the set of all properties that are
// NAMING or transitive sub-properties of NAMING.
func (c *Converter) buildNamingProperties() {
	c.namingProperties = make(map[identifier.Identifier]bool)
	c.namingProperties[namingPropID] = true
	for _, desc := range c.propertyDescendants[namingPropID] {
		c.namingProperties[desc] = true
	}
}

// buildLanguageCodes extracts language codes from language documents.
// Only language documents (those with INSTANCE_OF -> LANGUAGE) need to be passed,
// but the method still filters by INSTANCE_OF for safety.
func (c *Converter) buildLanguageCodes(allDocuments []*document.D) {
	c.languageCodes = make(map[identifier.Identifier]string)
	for _, doc := range allDocuments {
		if !isInstanceOf(doc, languageClassID) {
			continue
		}
		// Extract the CODE identifier claim and use the primary language subtag.
		ids := document.GetClaimsOfTypeWithConfidence[*document.IdentifierClaim](doc, codePropID, document.LowConfidence)
		if len(ids) > 0 {
			code, _, _ := strings.Cut(ids[0].Value, "-")
			c.languageCodes[doc.ID] = code
		}
	}
}

// getDocumentInfo returns the document info for a document, computing and
// caching it on first access. It computes display strings and lazily walks
// value hierarchy ancestors (e.g., SUBCLASS_OF). It is safe for concurrent use.
func (c *Converter) getDocumentInfo(ctx context.Context, id identifier.Identifier) (documentInfo, errors.E) {
	return c.computeDocumentInfo(ctx, id, make(map[identifier.Identifier]bool))
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
		rels := document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](doc, hierProp, document.LowConfidence)
		if len(rels) == 0 {
			continue
		}
		seen := map[identifier.Identifier]bool{id: true} // Exclude self to avoid duplicates.
		var hierAncestors []identifier.Identifier
		var hierIDPaths []string
		hierDisplayPaths := map[string][]string{}
		for _, rel := range rels {
			parentID := rel.To.ID
			if seen[parentID] {
				continue
			}
			seen[parentID] = true
			hierAncestors = append(hierAncestors, parentID)
			// Recursively get parent info to collect transitive ancestors and paths.
			parentInfo, parentErr := c.computeDocumentInfo(ctx, parentID, computing)
			if parentErr != nil {
				continue
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
				ancestors = make(map[identifier.Identifier][]identifier.Identifier)
			}
			ancestors[hierProp] = hierAncestors
		}
		if len(hierIDPaths) > 0 {
			if idPaths == nil {
				idPaths = make(map[identifier.Identifier][]string)
			}
			idPaths[hierProp] = hierIDPaths
		}
		if len(hierDisplayPaths) > 0 {
			if displayPaths == nil {
				displayPaths = make(map[identifier.Identifier]map[string][]string)
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
		// If lang does not exist in Display, this just means it is an empty string and we have not store
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
//  1. If the document's class defines a display label template, render it with
//     the target language (template functions use that language's fallback chain
//     internally). The result is the display label, even if empty.
//  2. If no template exists, search naming strings through the fallback chain.
//     The first (highest confidence) naming string from the first language in the
//     chain that has naming strings becomes the display label.
//
// Naming contains all naming strings per language as extracted from claims, without
// modifications. It is independent of Display.
func (c *Converter) makeDisplayStrings(ctx context.Context, doc *document.D) (displayStrings, errors.E) {
	tmplStr, errE := c.displayLabelTemplate(ctx, doc)
	if errE != nil {
		return displayStrings{}, errE
	}

	result := displayStrings{
		Display: make(map[string]string),
		Naming:  c.namingStrings(doc),
	}

	for lang := range SupportedLanguages {
		if tmplStr != "" {
			// Template exists: render it with the target language so that template
			// functions (e.g., bestString) use that language's fallback chain.
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

		// No template. Search naming strings in the fallback chain.
		chain := append([]string{lang}, c.getFallbackLanguages(lang)...)
		for _, tryLang := range chain {
			strs := result.Naming[tryLang]
			if len(strs) == 0 {
				continue
			}
			str := sanitizeDisplayString(strs[0])
			if str != "" {
				// We do not store an empty string into the map. But we still read it out as
				// an empty string when needed (reading a zero value from the map).
				result.Display[lang] = str
				// Here we are done only if we found a non-empty string.
				// There should be no empty strings in Naming, and none with null bytes either.
				// So if we got an empty string here, we ignore it and continue searching.
				break
			}
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

// displayLabelTemplate returns the best display label template for a document
// by looking at the document's INSTANCE_OF class documents. The template is
// not per-language, language fallback happens inside template functions.
//
// If multiple class documents define templates, the one with the highest
// effective confidence wins. Effective confidence is the product of the
// INSTANCE_OF claim's confidence and the template claim's confidence.
func (c *Converter) displayLabelTemplate(ctx context.Context, doc *document.D) (string, errors.E) {
	var bestTemplate string
	var bestConfidence document.Confidence

	for _, rel := range document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](doc, instanceOfPropID, document.LowConfidence) {
		classDoc, errE := c.getDocument(ctx, rel.To.ID)
		if errE != nil {
			return "", errE
		}
		instanceConfidence := rel.GetConfidence()
		for _, sc := range document.GetClaimsOfTypeWithConfidence[*document.StringClaim](classDoc, displayLabelTemplatePropID, document.LowConfidence) {
			effective := instanceConfidence * sc.GetConfidence()
			if effective > bestConfidence {
				bestConfidence = effective
				bestTemplate = sc.String
			}
		}
	}

	return bestTemplate, nil
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
func (c *Converter) templateFuncs(ctx context.Context, lang string) template.FuncMap {
	return template.FuncMap{
		// bestString returns the best string claim value for a mnemonic in the current language.
		// Falls back using the language priority chain.
		"bestString": func(mnemonic string, doc *document.D) (string, error) {
			if doc == nil {
				return "", nil
			}
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return "", errE
			}
			claims := document.GetClaimsOfTypeWithConfidence[*document.StringClaim](doc, propID, document.LowConfidence)
			// First pass: look for the requested language.
			for _, sc := range claims {
				if slices.Contains(c.extractInLanguages(sc.Meta), lang) {
					return sc.String, nil
				}
			}
			// Fallback passes using language priority chain.
			for _, fb := range c.getFallbackLanguages(lang) {
				for _, sc := range claims {
					if slices.Contains(c.extractInLanguages(sc.Meta), fb) {
						return sc.String, nil
					}
				}
			}
			return "", nil
		},
		// bestAmountString returns the display string of the best amount claim for a mnemonic.
		"bestAmountString": func(mnemonic string, doc *document.D) (string, error) {
			if doc == nil {
				return "", nil
			}
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return "", errE
			}
			ac := document.GetBestClaimOfType[*document.AmountClaim](doc, propID)
			if ac == nil {
				return "", nil
			}
			return ac.Amount.String(), nil
		},
		// bestRelationDoc follows the best relation claim for a mnemonic and returns the target document.
		"bestRelationDoc": func(mnemonic string, doc *document.D) (*document.D, error) {
			if doc == nil {
				return nil, nil
			}
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return nil, errE
			}
			rc := document.GetBestClaimOfType[*document.RelationClaim](doc, propID)
			if rc == nil {
				return nil, nil
			}
			return c.getDocument(ctx, rc.To.ID)
		},
		// getDocumentByMnemonic looks up the document ID from the mnemonics map and returns the document.
		"getDocumentByMnemonic": func(mnemonic string) (*document.D, error) {
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return nil, errE
			}
			return c.getDocument(ctx, propID)
		},
		// bestIdentifier returns the best identifier claim value for a mnemonic.
		"bestIdentifier": func(mnemonic string, doc *document.D) (string, error) {
			if doc == nil {
				return "", nil
			}
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return "", errE
			}
			ic := document.GetBestClaimOfType[*document.IdentifierClaim](doc, propID)
			if ic == nil {
				return "", nil
			}
			return ic.Value, nil
		},
		// bestTimeString returns the display string of the best time claim for a mnemonic.
		"bestTimeString": func(mnemonic string, doc *document.D) (string, error) {
			if doc == nil {
				return "", nil
			}
			propID, ok := c.mnemonics[mnemonic]
			if !ok {
				errE := errors.New("mnemonic not found")
				errors.Details(errE)["mnemonic"] = mnemonic
				return "", errE
			}
			tc := document.GetBestClaimOfType[*document.TimeClaim](doc, propID)
			if tc == nil {
				return "", nil
			}
			return tc.Timestamp.String(), nil
		},
	}
}

// namingStrings returns all naming display strings per language for a document.
// It collects string claims whose property is NAMING or any sub-property of NAMING
// (NAME, SHORT_NAME, ALTERNATIVE_NAME, TITLE, CODE, MNEMONIC, etc.).
func (c *Converter) namingStrings(doc *document.D) map[string][]string {
	claims := make(map[string][]*document.StringClaim)
	for propID := range c.namingProperties {
		for _, sc := range document.GetClaimsOfTypeWithConfidence[*document.StringClaim](doc, propID, document.LowConfidence) {
			for _, lang := range c.extractInLanguages(sc.Meta) {
				claims[lang] = append(claims[lang], sc)
			}
		}
	}
	if len(claims) == 0 {
		return nil
	}
	result := make(map[string][]string)
	for lang := range claims {
		slices.SortFunc(claims[lang], func(a, b *document.StringClaim) int {
			// Reverse order: higher confidence first.
			return cmp.Compare(b.GetConfidence(), a.GetConfidence())
		})
		result[lang] = make([]string, 0, len(claims[lang]))
		for _, sc := range claims[lang] {
			result[lang] = append(result[lang], sc.String)
		}
	}
	return result
}

// extractInLanguages extracts language codes from a claim's meta IN_LANGUAGE relations.
// A claim can be in multiple languages, so all matching codes are returned.
// Returns ["und"] if no languages are specified or none can be resolved to
// supported languages.
func (c *Converter) extractInLanguages(meta *document.ClaimTypes) []string {
	rels := document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](meta, inLanguagePropID, document.LowConfidence)
	var codes []string
	for _, rel := range rels {
		if code, ok := c.languageCodes[rel.To.ID]; ok && SupportedLanguages[code] {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return []string{undeterminedLanguage}
	}
	return codes
}

// extractInUnit extracts the unit identifier from a claim's meta IN_UNIT relation.
func (c *Converter) extractInUnit(meta *document.ClaimTypes) *identifier.Identifier {
	if rel := document.GetBestClaimOfType[*document.RelationClaim](meta, inUnitPropID); rel != nil {
		return &rel.To.ID
	}
	return nil
}

// propagateProp returns the property IDs to create claims for:
// the original property plus all its transitive super-properties.
// If X is a sub-property of Y, a claim for X also produces a claim for Y.
func (c *Converter) propagateProp(propID identifier.Identifier) []identifier.Identifier {
	result := []identifier.Identifier{propID}
	result = append(result, c.propertyAncestors[propID]...)
	return result
}

// convertVisitor implements document.Visitor to convert claims to search claims.
type convertVisitor struct {
	ctx       context.Context //nolint:containedctx
	converter *Converter
	result    *Document
}

var _ document.Visitor = (*convertVisitor)(nil)

// VisitIdentifier converts an identifier claim to search identifier claims.
func (v *convertVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertIdentifier(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Identifier = append(v.result.Claims.Identifier, claims...)
	return document.Keep, nil
}

// VisitString converts a string claim to search string claims.
func (v *convertVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertString(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.String = append(v.result.Claims.String, claims...)
	return document.Keep, nil
}

// VisitHTML converts an HTML claim to search HTML claims.
func (v *convertVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertHTML(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.HTML = append(v.result.Claims.HTML, claims...)
	return document.Keep, nil
}

// VisitAmount converts an amount claim to search amount claims.
func (v *convertVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertAmount(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Amount = append(v.result.Claims.Amount, claims...)
	return document.Keep, nil
}

// VisitAmountInterval converts an amount interval claim to search amount claims.
func (v *convertVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
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
	claims, errE := v.converter.convertTime(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Time = append(v.result.Claims.Time, claims...)
	return document.Keep, nil
}

// VisitTimeInterval converts a time interval claim to search time claims.
func (v *convertVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	timeClaims, unknownClaims, errE := v.converter.convertTimeInterval(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Time = append(v.result.Claims.Time, timeClaims...)
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, unknownClaims...)
	return document.Keep, nil
}

// VisitReference converts a reference claim to search reference claims.
func (v *convertVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertReference(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Reference = append(v.result.Claims.Reference, claims...)
	return document.Keep, nil
}

// VisitRelation converts a relation claim to search relation claims.
func (v *convertVisitor) VisitRelation(claim *document.RelationClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertRelation(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Relation = append(v.result.Claims.Relation, claims...)
	return document.Keep, nil
}

// VisitHas converts a has claim to search has claims.
func (v *convertVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertHas(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Has = append(v.result.Claims.Has, claims...)
	return document.Keep, nil
}

// VisitNone converts a none claim to search none claims.
func (v *convertVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertNone(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.None = append(v.result.Claims.None, claims...)
	return document.Keep, nil
}

// VisitUnknown converts an unknown claim to search unknown claims.
func (v *convertVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	claims, errE := v.converter.convertUnknown(v.ctx, claim)
	if errE != nil {
		return document.Keep, errE
	}
	v.result.Claims.Unknown = append(v.result.Claims.Unknown, claims...)
	return document.Keep, nil
}

// FromDocument converts a document.D to a search Document.
func (c *Converter) FromDocument(ctx context.Context, doc *document.D) (*Document, errors.E) {
	v := &convertVisitor{
		ctx:       ctx,
		converter: c,
		result: &Document{
			ID:     doc.ID,
			Claims: ClaimTypes{},
		},
	}
	errE := doc.Visit(v)
	if errE != nil {
		return nil, errE
	}
	return v.result, nil
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
	str := make(map[string]string)
	for _, lang := range c.extractInLanguages(claim.Meta) {
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
	html := make(map[string]string)
	for _, lang := range c.extractInLanguages(claim.Meta) {
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
	unit := c.extractInUnit(claim.Meta)
	amount, errE := claim.Amount.Float64(claim.Precision)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	from := amount - claim.Precision/2 //nolint:mnd
	to := amount + claim.Precision/2   //nolint:mnd
	display := claim.Amount.String()

	rangeFloat := RangeFloat{ //nolint:exhaustruct
		GreaterThanOrEqual: &from,
		LessThan:           &to,
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
		if claim.ToIsClosed {
			rangeFloat.LessThanOrEqual = &toValue
		} else {
			rangeFloat.LessThan = &toValue
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

	// Sanity check.
	errE := rangeFloat.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, errE
	}

	// TODO: Normalize amounts of units of same measure to same base unit (e.g., cm and mm to m).
	unit := c.extractInUnit(claim.Meta)
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
	t, errE := claim.Timestamp.Time(claim.Precision, time.UTC)
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, errE
	}

	from := t.Unix()
	to := addPrecision(t, claim.Precision).Unix()
	display := claim.Timestamp.String()

	rangeInt := RangeInt{ //nolint:exhaustruct
		GreaterThanOrEqual: &from,
		LessThan:           &to,
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
			Range:       rangeInt,
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
		rangeInt    RangeInt
		from, to    *int64
		fromDisplay string
		toDisplay   string
	)

	switch { //nolint:dupl
	case claim.From != nil:
		if claim.FromPrecision == nil {
			errE := errors.New("missing from precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// TODO: How to integrate precision besides validation?
		tm, errE := claim.From.Time(*claim.FromPrecision, time.UTC)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		f := tm.Unix()
		from = &f
		fromDisplay = claim.From.String()
		if claim.FromIsOpen {
			rangeInt.GreaterThan = &f
		} else {
			rangeInt.GreaterThanOrEqual = &f
		}
	case claim.FromIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave from and fromDisplay empty.
		// But we want to find it always when we search by range.
		f := int64(math.MinInt64)
		rangeInt.GreaterThanOrEqual = &f
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
			Timestamp: *claim.To,
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

	switch { //nolint:dupl
	case claim.To != nil:
		if claim.ToPrecision == nil {
			errE := errors.New("missing to precision in claim")
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		// TODO: How to integrate precision besides validation?
		tm, errE := claim.To.Time(*claim.ToPrecision, time.UTC)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, nil, errE
		}
		t := tm.Unix()
		to = &t
		toDisplay = claim.To.String()
		if claim.ToIsClosed {
			rangeInt.LessThanOrEqual = &t
		} else {
			rangeInt.LessThan = &t
		}
	case claim.ToIsNone:
		// We cannot search by the exact bound (we know that it does not exist),
		// so we leave to and toDisplay empty.
		// But we want to find it always when we search by range.
		t := int64(math.MaxInt64)
		rangeInt.LessThanOrEqual = &t
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
			Timestamp: *claim.From,
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
	errE := rangeInt.Validate()
	if errE != nil {
		errors.Details(errE)["claim"] = claim
		return nil, nil, errE
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
			Range:       rangeInt,
			From:        from,
			FromDisplay: fromDisplay,
			To:          to,
			ToDisplay:   toDisplay,
		})
	}
	return result, nil, nil
}

func (c *Converter) convertReference(ctx context.Context, claim *document.ReferenceClaim) ([]ReferenceClaim, errors.E) {
	props := c.propagateProp(claim.Prop.ID)
	result := make([]ReferenceClaim, 0, len(props))
	for _, pid := range props {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			errors.Details(errE)["claim"] = claim
			return nil, errE
		}
		result = append(result, ReferenceClaim{
			Prop:        pid,
			PropDisplay: propDisplay.Display,
			PropNaming:  propDisplay.Naming,
			IRI:         claim.IRI,
		})
	}
	return result, nil
}

func (c *Converter) convertRelation(ctx context.Context, claim *document.RelationClaim) ([]RelationClaim, errors.E) {
	// Convert meta relation claims to nested search relation claims.
	var nested RelationClaims
	for _, mr := range document.GetAllClaimsOfTypeWithConfidence[*document.RelationClaim](claim.Meta, document.LowConfidence) {
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
		nested = append(nested, RelationClaim{
			Prop:        mr.Prop.ID,
			PropDisplay: mrPropDisplay.Display,
			PropNaming:  mrPropDisplay.Naming,
			To:          mr.To.ID,
			ToDisplay:   mrToDisplay.Display,
			ToNaming:    mrToDisplay.Naming,
			Relation:    nil,
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

	result := make([]RelationClaim, 0, len(propIDs)*len(targetIDs))
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
			result = append(result, RelationClaim{
				Prop:          pid,
				PropDisplay:   propDisplay.Display,
				PropNaming:    propDisplay.Naming,
				To:            tid,
				ToDisplay:     tidInfo.Display.Display,
				ToNaming:      tidInfo.Display.Naming,
				ToPath:        toPath,
				ToDisplayPath: toDisplayPath,
				Relation:      nested,
			})
		}
	}
	return result, nil
}

func (c *Converter) convertHas(ctx context.Context, claim *document.HasClaim) ([]HasClaim, errors.E) {
	// Convert meta relation claims to nested search relation claims.
	var nested RelationClaims
	for _, mr := range document.GetAllClaimsOfTypeWithConfidence[*document.RelationClaim](claim.Meta, document.LowConfidence) {
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
		nested = append(nested, RelationClaim{
			Prop:        mr.Prop.ID,
			PropDisplay: mrPropDisplay.Display,
			PropNaming:  mrPropDisplay.Naming,
			To:          mr.To.ID,
			ToDisplay:   mrToDisplay.Display,
			ToNaming:    mrToDisplay.Naming,
			Relation:    nil,
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
			Relation:    nested,
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
func addPrecision(t time.Time, precision document.TimePrecision) time.Time {
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
