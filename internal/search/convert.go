package search

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

const undeterminedLanguage = "und"

// Well-known property and class IDs computed from the core namespace.
//
//nolint:gochecknoglobals
var (
	subpropertyOfPropID = identifier.From("core.peerdb.org", "SUBPROPERTY_OF")
	subclassOfPropID    = identifier.From("core.peerdb.org", "SUBCLASS_OF")
	namingPropID        = identifier.From("core.peerdb.org", "NAMING")
	inLanguagePropID    = identifier.From("core.peerdb.org", "IN_LANGUAGE")
	inUnitPropID        = identifier.From("core.peerdb.org", "IN_UNIT")
	codePropID          = identifier.From("core.peerdb.org", "CODE")
	instanceOfPropID    = identifier.From("core.peerdb.org", "INSTANCE_OF")
	propertyClassID     = identifier.From("core.peerdb.org", "PROPERTY")
	classClassID        = identifier.From("core.peerdb.org", "CLASS")
	languageClassID     = identifier.From("core.peerdb.org", "LANGUAGE")
)

type displayStrings struct {
	Display map[string]string
	Naming  map[string][]string
}

// Converter holds preprocessed data for converting document.D to search Document.
type Converter struct {
	// propertyDescendants maps a property ID to all its transitive sub-property IDs.
	propertyDescendants map[identifier.Identifier][]identifier.Identifier
	// propertyAncestors maps a property ID to all its transitive super-property IDs.
	propertyAncestors map[identifier.Identifier][]identifier.Identifier
	// classAncestors maps a class ID to all its transitive super-class IDs.
	classAncestors map[identifier.Identifier][]identifier.Identifier
	// namingProperties is the set of property IDs that are NAMING or sub-properties of NAMING.
	namingProperties map[identifier.Identifier]bool
	// languageCodes maps language document ID to primary language subtag (e.g., "en").
	languageCodes map[identifier.Identifier]string
	// mnemonics maps mnemonic to property ID.
	mnemonics map[string]identifier.Identifier
	// getDocument fetches a document by ID from the store.
	getDocument func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E)
	// displayCache caches computed display strings per language per document ID.
	displayCache map[identifier.Identifier]displayStrings
}

// NewConverter creates a Converter that preprocesses property and class hierarchies.
// properties contains all property documents, classes contains all class documents,
// vocabularies contains vocabulary documents (including language documents) needed
// for language code extraction, and getDocument is a callback to fetch documents by ID.
func NewConverter(
	properties, classes, vocabularies []*document.D,
	mnemonics map[string]identifier.Identifier,
	getDocument func(ctx context.Context, id identifier.Identifier) (*document.D, errors.E),
) *Converter {
	c := &Converter{
		propertyDescendants: nil,
		propertyAncestors:   nil,
		classAncestors:      nil,
		namingProperties:    nil,
		languageCodes:       nil,
		mnemonics:           mnemonics,
		getDocument:         getDocument,
		displayCache:        make(map[identifier.Identifier]displayStrings),
	}
	c.buildPropertyHierarchy(properties)
	c.buildClassHierarchy(classes)
	c.buildNamingProperties()
	c.buildLanguageCodes(vocabularies)
	return c
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
		if len(visited) > 0 {
			result := make([]identifier.Identifier, 0, len(visited))
			for a := range visited {
				result = append(result, a)
			}
			c.propertyAncestors[prop.ID] = result
		}
	}
}

// buildClassHierarchy computes transitive ancestors for each class
// based on SUBCLASS_OF relation claims. Only documents that are instances of CLASS
// are considered.
func (c *Converter) buildClassHierarchy(classes []*document.D) {
	// Build child -> parents map. A class X with SUBCLASS_OF -> Y
	// means Y is a parent (super-class) of X.
	childParents := make(map[identifier.Identifier][]identifier.Identifier)
	for _, cls := range classes {
		if !isInstanceOf(cls, classClassID) {
			continue
		}
		for _, rel := range document.GetClaimsOfTypeWithConfidence[*document.RelationClaim](cls, subclassOfPropID, document.LowConfidence) {
			childParents[cls.ID] = append(childParents[cls.ID], rel.To.ID)
		}
	}

	// Compute transitive ancestors for each class.
	c.classAncestors = make(map[identifier.Identifier][]identifier.Identifier)
	for _, cls := range classes {
		visited := make(map[identifier.Identifier]bool)
		var walk func(identifier.Identifier)
		walk = func(classID identifier.Identifier) {
			for _, parent := range childParents[classID] {
				if !visited[parent] {
					visited[parent] = true
					walk(parent)
				}
			}
		}
		walk(cls.ID)
		if len(visited) > 0 {
			result := make([]identifier.Identifier, 0, len(visited))
			for a := range visited {
				result = append(result, a)
			}
			c.classAncestors[cls.ID] = result
		}
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

// buildLanguageCodes extracts language codes from language vocabulary documents.
// It identifies language documents by their INSTANCE_OF -> LANGUAGE class relation
// and extracts the CODE identifier claim value.
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

// getDisplayStrings returns the display strings for a document, making and
// caching them on first access.
func (c *Converter) getDisplayStrings(ctx context.Context, id identifier.Identifier) (displayStrings, errors.E) {
	if display, ok := c.displayCache[id]; ok {
		return display, nil
	}
	doc, errE := c.getDocument(ctx, id)
	if errE != nil {
		return displayStrings{}, errE
	}
	display, errE := c.makeDisplayStrings(ctx, doc)
	if errE != nil {
		return displayStrings{}, errE
	}
	c.displayCache[id] = display
	return display, nil
}

// makeDisplayStrings returns the display strings for a document.
func (c *Converter) makeDisplayStrings(_ context.Context, doc *document.D) (displayStrings, errors.E) { //nolint:unparam
	namingStrings := c.namingStrings(doc)

	display := displayStrings{
		Display: make(map[string]string, len(namingStrings)),
		Naming:  make(map[string][]string, len(namingStrings)),
	}

	for lang, strs := range namingStrings {
		// There should always be at least one.
		display.Display[lang] = strs[0]
		display.Naming[lang] = strs[1:]
	}

	return display, nil
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
		// so we lave from and fromDisplay empty.
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
		// so we lave to and toDisplay empty.
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
		// so we lave from and fromDisplay empty.
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
		// so we lave to and toDisplay empty.
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
			return nil, errE
		}
		mrToDisplay, errE := c.getDisplayStrings(ctx, mr.To.ID)
		if errE != nil {
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

	// Cross product of propagated properties x (target + ancestor classes).
	propIDs := c.propagateProp(claim.Prop.ID)
	targetIDs := []identifier.Identifier{claim.To.ID}
	targetIDs = append(targetIDs, c.classAncestors[claim.To.ID]...)

	result := make([]RelationClaim, 0, len(propIDs)*len(targetIDs))
	for _, pid := range propIDs {
		propDisplay, errE := c.getDisplayStrings(ctx, pid)
		if errE != nil {
			return nil, errE
		}
		for _, tid := range targetIDs {
			toDisplay, errE := c.getDisplayStrings(ctx, tid)
			if errE != nil {
				return nil, errE
			}
			result = append(result, RelationClaim{
				Prop:        pid,
				PropDisplay: propDisplay.Display,
				PropNaming:  propDisplay.Naming,
				To:          tid,
				ToDisplay:   toDisplay.Display,
				ToNaming:    toDisplay.Naming,
				Relation:    nested,
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
			return nil, errE
		}
		mrToDisplay, errE := c.getDisplayStrings(ctx, mr.To.ID)
		if errE != nil {
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
