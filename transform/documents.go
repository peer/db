// Package transform provides functionality to transform Go structs into PeerDB documents.
//
// # Overview
//
// This package uses reflection to inspect struct fields and their tags to automatically
// generate PeerDB documents and their claims. It supports various claim types (string, numeric, time,
// relation, etc.) and provides fine-grained control over field processing through struct tags.
//
// # Basic Usage
//
//	type Person struct {
//		ID   []string `documentid:""`
//		Name string   `property:"NAME"`
//		Age  int      `property:"AGE" unit:"1"`
//	}
//
//	mnemonics := map[string]identifier.Identifier{
//		"NAME": identifier.New(),
//		"AGE":  identifier.New(),
//	}
//
//	docs := []any{
//		&Person{
//			ID:   []string{"people", "person1"},
//			Name: "John Doe",
//			Age:  30,
//		},
//	}
//
//	documents, err := transform.Documents(mnemonics, docs)
//
// # Supported Struct Tags
//
// ## documentid
//
// Marks a field as containing the document ID. Must be a []string slice.
// Only one field per struct (or embedded structs) can have this tag.
//
//	ID []string `documentid:""`
//
// ## property
//
// Maps a field to a property using its mnemonic. The mnemonic must exist
// in the mnemonics map passed to Documents().
//
//	Name string `property:"NAME"`
//
// Use property:"-" to explicitly skip a field:
//
//	Internal string `property:"-"`
//
// ## value
//
// Marks a field as the value field within a nested struct. This field's value
// becomes the main claim, while other fields in the struct become meta claims.
//
//	type PersonName struct {
//		Value  string        `value:""`
//		Period core.Interval `property:"PERIOD"`
//	}
//
// Cannot be combined with property, cardinality, and default tags.
// Those tags belong to the field which uses the nested struct the value field is in.
//
// ## type
//
// Specifies how to interpret fields. Supported types for string fields:
//
//   - "id": create an identifier claim,
//   - "iri": create a reference claim,
//   - "html": create a text claim with HTML (content will be escaped),
//   - "rawhtml": create a text claim with raw HTML (content will be sanitized but not escaped).
//
// Supported types for boolean fields:
//
//   - "none": create a none-value claim when true,
//   - "unknown": create an unknown-value claim when true.
//
// Example:
//
//	Code      string `property:"CODE" type:"id"`
//	Homepage  string `property:"HOMEPAGE" type:"iri"`
//	Bio       string `property:"BIO" type:"html"`
//	IsAbsent  bool   `property:"NAME" type:"none"`
//	IsUnknown bool   `property:"AGE" type:"unknown"`
//
// ## unit
//
// Required for numeric types (int, float, etc.). Specifies the unit of measurement.
// Must be a valid PeerDB AmountUnit (e.g., "m", "kg", "1" for unitless).
//
//	Height float64 `property:"HEIGHT" unit:"m"`
//	Count  int     `property:"COUNT" unit:"1"`
//
// ## cardinality
//
// Specifies minimum and maximum number of values for a field. Format: "min..max".
//
// Formats:
//   - "1": exactly one value (min=1, max=1),
//   - "1..": one or more values (min=1, max=unbounded),
//   - "0..1": zero or one value (min=0, max=1),
//   - "2..5": between 2 and 5 values (min=2, max=5).
//
// Rules:
//   - slice fields: any cardinality range allowed,
//   - pointer fields: max must be ≤ 1 (can be 0 or 1),
//   - single value fields: max must be ≤ 1 (can be 0 or 1),
//   - max cardinality cannot be 0.
//
// Default: Fields without cardinality are optional (min=0, max=unbounded).
//
//	Required   []string  `property:"NAME" cardinality:"1.."` // At least one required.
//	Optional   *string   `property:"NOTE" cardinality:"0..1"` // Zero or one.
//	Exactly2   []string  `property:"IDS" cardinality:"2"`     // Exactly two.
//
// ## default
//
// Specifies default claim to create when field doesn't satisfy minimum cardinality,
// instead of returning an error.
// Can only be used with cardinality tag when min > 0.
// The number of claims added equals (min - actual count).
//
// Supported values:
//
//   - "none": add none-value claim(s),
//   - "unknown": add unknown-value claim(s)..
//
// Example:
//
//	Name []string `property:"NAME" cardinality:"1.." default:"none"`
//
// Semantic difference between values:
//   - none: it is known that the value does not exist,
//   - unknown: it is known that the value exists but is unknown or cannot be determined.
//
// # Field Types
//
// Supported field types:
//   - string: string claim (or identifier claim/reference claim/text claim with type tag),
//   - int, int8, int16, int32, int64: amount claim (requires unit tag),
//   - uint, uint8, uint16, uint32, uint64: amount claim (requires unit tag),
//   - float32, float64: amount claim (requires unit tag),
//   - bool: none-value claim when true (or none-value/unknown-value claim with type tag) (TODO: Change to has claim),
//   - core.Ref: relation claim,
//   - core.Time: time claim,
//   - core.Interval: time range claim,
//   - core.Identifier: identifier claim,
//   - core.IRI: reference claim,
//   - core.HTML: text claim (with escaping),
//   - core.RawHTML: text claim (without escaping),
//   - core.None: none-value claim when true,
//   - core.Unknown: unknown-value claim when true,
//   - struct: nested claims (value field + meta claims),
//   - []T: slice of any supported type,
//   - *T: pointer to any supported type.
//
// # Empty Values
//
// Empty values (zero values) do not produce claims unless:
//   - field has cardinality with min > 0 and default:"none" tag: creates none-value claim(s),
//   - Field has cardinality with min > 0 and default:"unknown" tag: creates unknown-value claim(s),
//   - Field has cardinality with min > 0 without default tag: returns error,
//   - Nested struct with no value but has meta claims: creates none-value claim with meta claims (TODO: Change to has claim).
//
// # Examples
//
// ## Basic Document
//
//	type Article struct {
//		ID    []string `documentid:""`
//		Title string   `property:"TITLE"`
//		Views int      `property:"VIEWS" unit:"1"`
//	}
//
// ## Required Fields
//
//	type Person struct {
//		ID       []string     `documentid:""`
//		LastName []PersonName `property:"LAST_NAME" cardinality:"1.." default:"none"`
//		Name     []PersonName `property:"NAME" cardinality:"1.." default:"none"`
//	}
//
// ## Nested Structures with Value Field
//
//	type PersonName struct {
//		Value  string        `value:""`
//		Period core.Interval `property:"PERIOD"`
//		Note   string        `property:"NOTE"`
//	}
//
//	type Person struct {
//		ID   []string   `documentid:""`
//		Name PersonName `property:"NAME"`
//	}
//
// ## Cardinality Constraints
//
//	type Document struct {
//		ID       []string `documentid:""`
//		Authors  []string `property:"AUTHORS" cardinality:"1.."` // At least 1 required.
//		Keywords []string `property:"KEYWORDS" cardinality:"1..5"` // Between 1 and 5.
//		Note     *string  `property:"NOTE" cardinality:"0..1"` // Optional, max 1.
//	}
//
// ## HTML Content
//
//	type Article struct {
//		ID          []string  `documentid:""`
//		Description string    `property:"DESCRIPTION" type:"html"` // Escaped HTML.
//		RawContent  string    `property:"CONTENT" type:"rawhtml"` // Sanitized raw HTML.
//		PlainHTML   core.HTML `property:"PLAIN"` // Escaped HTML.
//	}
//
// ## None and Unknown Values
//
//	type Person struct {
//		ID               []string  `documentid:""`
//		Name             string    `property:"NAME"`
//		Age              *int      `property:"AGE"` // Age is optional.
//		AgeIsUnknown     bool      `property:"AGE" type:"unknown"` // Creates unknown-value claim when true.
//		LastNameIsAbsent core.None `property:"LAST_NAME"` // Creates none-value claim when true.
//	}
package transform

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"

	"gitlab.com/peerdb/peerdb/core"
)

//nolint:gochecknoglobals
var (
	coreRef        = reflect.TypeFor[core.Ref]()
	coreTime       = reflect.TypeFor[core.Time]()
	coreInterval   = reflect.TypeFor[core.Interval]()
	coreIdentifier = reflect.TypeFor[core.Identifier]()
	coreIRI        = reflect.TypeFor[core.IRI]()
	coreHTML       = reflect.TypeFor[core.HTML]()
	coreRawHTML    = reflect.TypeFor[core.RawHTML]()
	coreNone       = reflect.TypeFor[core.None]()
	coreUnknown    = reflect.TypeFor[core.Unknown]()
)

var ErrDocumentIDNotFound = errors.Base("document ID not found")

var (
	errClaimNotMade       = errors.Base("claim not made")
	errValueClaimNotFound = errors.Base("value claim not found")
)

const (
	defaultNone    = "none"
	defaultUnknown = "unknown"

	typeID      = "id"
	typeIRI     = "iri"
	typeHTML    = "html"
	typeRawHTML = "rawhtml"
	typeNone    = "none"
	typeUnknown = "unknown"
)

// Documents transforms structs into PeerDB document.D documents.
//
// It takes a map between property mnemonics and identifiers, and a slice of documents
// which can be various struct types. It uses reflection to inspect structs and their
// struct tags to determine how to map struct fields to document claims.
func Documents(ctx context.Context, mnemonics map[string]identifier.Identifier, documents []any) ([]document.D, errors.E) {
	result := []document.D{}

	for _, doc := range documents {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		d, errE := transformDocument(mnemonics, doc)
		if errE != nil {
			errors.Details(errE)["doc"] = doc
			errors.Details(errE)["type"] = fmt.Sprintf("%T", doc)
			return nil, errE
		}
		result = append(result, d)
	}

	return result, nil
}

type transformer struct {
	Mnemonics map[string]identifier.Identifier
	Claims    *document.ClaimTypes
}

// transformDocument transforms a struct to a document.
func transformDocument(mnemonics map[string]identifier.Identifier, doc any) (document.D, errors.E) {
	v := reflect.ValueOf(doc)
	// Handle pointer to struct.
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		errE := errors.New("expected struct")
		errors.Details(errE)["got"] = v.Kind().String()
		return document.D{}, errE
	}

	t := v.Type()

	// Extract document ID.
	docID, errE := extractDocumentID(v, t, []string{})
	if errE != nil {
		return document.D{}, errE
	}

	result := document.D{
		CoreDocument: document.CoreDocument{
			ID:    identifier.From(docID...),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{},
	}

	// Create transformer with the document.
	tr := transformer{
		Mnemonics: mnemonics,
		Claims:    result.Claims,
	}

	// Process all fields. Start with document ID as the base for claim ID paths.
	errE = tr.processStructFields(v, t, docID, []string{}, map[identifier.Identifier]int{})
	if errE != nil {
		return document.D{}, errE
	}

	return result, nil
}

// processStructFields processes all fields of a struct.
func (tr *transformer) processStructFields(
	structValue reflect.Value,
	structType reflect.Type,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) errors.E {
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)
		fieldType := fieldValue.Type()

		newFieldPath := append(slices.Clone(fieldPath), field.Name)

		// Skip documentid field.
		if _, ok := field.Tag.Lookup("documentid"); ok {
			continue
		}

		// Skip value field.
		if _, ok := field.Tag.Lookup("value"); ok {
			continue
		}

		propertyMnemonic := field.Tag.Get("property")
		if propertyMnemonic == "-" {
			// Skip fields with property:"-".
			continue
		}

		// If this is an embedded struct, recursively process its fields.
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			errE := tr.processStructFields(fieldValue, fieldType, idPath, newFieldPath, claims)
			if errE != nil {
				return errE
			}
			continue
		}

		if propertyMnemonic == "" {
			// Skip fields without property tag.
			continue
		}

		// Get property ID from mnemonic.
		propertyID, ok := tr.Mnemonics[propertyMnemonic]
		if !ok {
			errE := errors.New("mnemonic not found")
			errors.Details(errE)["name"] = propertyMnemonic
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}

		newIDPath := append(slices.Clone(idPath), propertyMnemonic)

		// Get tags.
		typeTag := field.Tag.Get("type")
		unit := field.Tag.Get("unit")
		cardinality := field.Tag.Get("cardinality")
		defaultTag := field.Tag.Get("default")

		hasDefaultTag := defaultTag != ""

		minCardinality, maxCardinality, errE := parseCardinality(cardinality, fieldValue, hasDefaultTag, newFieldPath)
		if errE != nil {
			return errE
		}

		// Despite passing newIDPath here, we still pass existing claims. In fact, this is exactly the situation why
		// we have claims map in the first place: because if multiple fields add claims for the same property, we have
		// to track this to make sure claim IDs do not collide.
		errE = tr.processField(
			fieldValue, fieldType, propertyID, typeTag, defaultTag, minCardinality, maxCardinality,
			unit, newIDPath, newFieldPath, claims,
		)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// processField processes one field of a struct.
//
// It supports simple types of values and slices, pointers and non-core structs.
func (tr *transformer) processField(
	fieldValue reflect.Value,
	fieldType reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	defaultTag string,
	minCardinality int,
	maxCardinality int,
	unit string,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) errors.E {
	count := 0

	// Handle slices.
	if fieldValue.Kind() == reflect.Slice {
		for i := range fieldValue.Len() {
			elem := fieldValue.Index(i)
			errE := tr.processSingleValue(elem, elem.Type(), propertyID, typeTag, unit, idPath, fieldPath, claims)
			if errors.Is(errE, errClaimNotMade) {
				continue
			} else if errE != nil {
				return errE
			}

			count++
		}

		// Handle pointers.
	} else if fieldValue.Kind() == reflect.Ptr {
		// We do not use errors.E here because we do not really need a stack trace.
		err := errClaimNotMade
		count = 0

		if !fieldValue.IsNil() {
			elem := fieldValue.Elem()
			err = tr.processSingleValue(elem, elem.Type(), propertyID, typeTag, unit, idPath, fieldPath, claims)
		}

		if errors.Is(err, errClaimNotMade) { //nolint:revive
			// Do nothing.
		} else if err != nil {
			// errors.WithStack will not really add a stack trace here because err is in fact already errors.E.
			return errors.WithStack(err)
		} else {
			count++
		}

		// Handle single value.
	} else {
		errE := tr.processSingleValue(fieldValue, fieldType, propertyID, typeTag, unit, idPath, fieldPath, claims)

		if errors.Is(errE, errClaimNotMade) { //nolint:revive
			// Do nothing.
		} else if errE != nil {
			return errE
		} else {
			count++
		}
	}

	// Check cardinality constraints.
	if count < minCardinality {
		switch defaultTag {
		case defaultNone:
			// Add (minCardinality - count) NoValueClaims.
			for range minCardinality - count {
				claimID := newClaimID(idPath, propertyID, claims)
				noValueClaim := &document.NoValueClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: document.HighConfidence,
					},
					Prop: document.Reference{ID: &propertyID},
				}
				errE := tr.Claims.Add(noValueClaim)
				if errE != nil {
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return errE
				}
			}
		case defaultUnknown:
			// Add (minCardinality - count) UnknownValueClaims.
			for range minCardinality - count {
				claimID := newClaimID(idPath, propertyID, claims)
				unknownValueClaim := &document.UnknownValueClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: document.HighConfidence,
					},
					Prop: document.Reference{ID: &propertyID},
				}
				errE := tr.Claims.Add(unknownValueClaim)
				if errE != nil {
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return errE
				}
			}
		default:
			errE := errors.New("field value does not satisfy minimum cardinality")
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			errors.Details(errE)["cardinality"] = []int{minCardinality, maxCardinality}
			errors.Details(errE)["count"] = count
			return errE
		}
	}

	if maxCardinality != -1 && count > maxCardinality {
		errE := errors.New("field value exceeds maximum cardinality")
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		errors.Details(errE)["cardinality"] = []int{minCardinality, maxCardinality}
		errors.Details(errE)["count"] = count
		return errE
	}

	return nil
}

// processSingleValue processes a single value of a field.
//
// It supports simple types of values and non-core structs, but not slices and pointers
// (for the latter, processSingleValue should be called on their elements).
//
// It returns an errClaimNotMade error if no claim has been made (e.g., the value vas empty).
func (tr *transformer) processSingleValue(
	fieldValue reflect.Value,
	fieldType reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	unit string,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) errors.E {
	claim, errE := makeClaim(fieldValue, fieldType, propertyID, typeTag, unit, idPath, claims)
	if errors.Is(errE, errClaimNotMade) {
		return errE
	} else if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return errE
	}

	// Handle structs.
	if claim == nil && fieldValue.Kind() == reflect.Struct {
		// We reconstruct the newIDPath under current propertyID-based claim count.
		// This is the same count which is used below to construct the value claim.
		newIDPath := append(slices.Clone(idPath), strconv.Itoa(claims[propertyID]))

		// Create transformer for meta claims.
		metaTr := transformer{
			Mnemonics: tr.Mnemonics,
			Claims:    &document.ClaimTypes{},
		}

		// Here we use newIDPath because all other claims are nested under the value claim as its meta claims.
		// Because we use newIDPath, we can use an empty claims map because all claim IDs made under newIDPath
		// cannot collide with claim IDs made under idPath.
		errE = metaTr.processStructFields(fieldValue, fieldType, newIDPath, fieldPath, map[identifier.Identifier]int{})
		if errE != nil {
			return errE
		}

		// Here we use idPath because this claim really belongs at the level above this struct.
		// It is only inside the struct so that we can list also its meta claims next to it.
		claim, errE = extractValueClaim(fieldValue, fieldType, propertyID, idPath, fieldPath, claims)
		if errors.Is(errE, errClaimNotMade) {
			if metaTr.Claims.Size() == 0 {
				// There are no meta claims nor a value claim, so we just return errClaimNotMade here.
				return errE
			}

			// There is a value claim defined, but in this particular instance it has empty value,
			// but there are meta claims for it, so we use NoValueClaim.
			// TODO: What is all meta claims are "no value" claims?
			claimID := newClaimID(idPath, propertyID, claims)
			claim = &document.NoValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
			}
		} else if errors.Is(errE, errValueClaimNotFound) {
			if metaTr.Claims.Size() == 0 {
				// There are no meta claims nor a value claim, which makes nested claims be an empty value of sorts,
				// so we just return errClaimNotMade here.
				return errors.WithStack(errClaimNotMade)
			}

			// Value claim is not defined, so these are nested claims.
			claimID := newClaimID(idPath, propertyID, claims)
			// TODO: Make this better. We currently map nested claims to NoValueClaim, but we should to HasClaim.
			claim = &document.NoValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
			}
		} else if errE != nil {
			return errE
		}

		// We copy all claims to the value claim as its meta claims.
		for _, c := range metaTr.Claims.AllClaims() {
			errE = claim.Add(c)
			if errE != nil {
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return errE
			}
		}
	}

	if claim != nil {
		errE = tr.Claims.Add(claim)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			return errE
		}
		return nil
	}

	errE = errors.New("field has unsupported or unexpected value type")
	errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
	errors.Details(errE)["type"] = fieldType.String()
	return errE
}

// extractValueClaim returns a claim for the field with tag "value".
//
// If no such field is found, it returns an errValueClaimNotFound error.
// If such field is found, but it has no value, it returns an errClaimNotMade error.
//
//nolint:ireturn
func extractValueClaim(
	structValue reflect.Value,
	structType reflect.Type,
	propertyID identifier.Identifier,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) (document.Claim, errors.E) {
	valueClaimsAndErrors := []any{}

	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)
		fieldType := fieldValue.Type()

		newFieldPath := append(slices.Clone(fieldPath), field.Name)

		// We use Lookup because the tag has empty value.
		if _, ok := field.Tag.Lookup("value"); ok {
			if _, hasProperty := field.Tag.Lookup("property"); hasProperty {
				errE := errors.New("property tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			if _, hasProperty := field.Tag.Lookup("cardinality"); hasProperty {
				errE := errors.New("cardinality tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			if _, hasProperty := field.Tag.Lookup("default"); hasProperty {
				errE := errors.New("default tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			// Get tags.
			typeTag := field.Tag.Get("type")
			unit := field.Tag.Get("unit")

			claim, errE := processValueClaimField(fieldValue, fieldType, propertyID, typeTag, unit, idPath, newFieldPath, claims)
			if errE != nil {
				return nil, errE
			}

			valueClaimsAndErrors = append(valueClaimsAndErrors, claim)
			continue
		}

		// If this is an embedded struct, recursively check its fields.
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			vc, errE := extractValueClaim(fieldValue, fieldType, propertyID, idPath, newFieldPath, claims)
			if errors.Is(errE, errValueClaimNotFound) {
				continue
			} else if errors.Is(errE, errClaimNotMade) {
				valueClaimsAndErrors = append(valueClaimsAndErrors, errE)
			} else if errE != nil {
				return nil, errE
			} else {
				valueClaimsAndErrors = append(valueClaimsAndErrors, vc)
			}
		}
	}

	if len(valueClaimsAndErrors) > 1 {
		errE := errors.New("multiple value claims found")
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return nil, errE
	} else if len(valueClaimsAndErrors) == 1 {
		switch vc := valueClaimsAndErrors[0].(type) {
		case errors.E:
			return nil, vc
		case document.Claim:
			return vc, nil
		default:
			errE := errors.New("unexpected value claim type")
			errors.Details(errE)["type"] = fmt.Sprintf("%T", vc)
			panic(errE)
		}
	}

	return nil, errors.WithStack(errValueClaimNotFound)
}

// processValueClaimField returns a value claim for the field.
//
// If the field has no value, it returns an errClaimNotMade error.
//
//nolint:ireturn
func processValueClaimField(
	fieldValue reflect.Value,
	fieldType reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	unit string,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) (document.Claim, errors.E) {
	// Handle pointers.
	if fieldValue.Kind() == reflect.Ptr {
		if fieldValue.IsNil() {
			return nil, errors.WithStack(errClaimNotMade)
		}

		fieldValue = fieldValue.Elem()
		fieldType = fieldValue.Type()
	}

	claim, errE := makeClaim(fieldValue, fieldType, propertyID, typeTag, unit, idPath, claims)
	if errors.Is(errE, errClaimNotMade) {
		return nil, errE
	} else if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return nil, errE
	} else if claim != nil {
		return claim, nil
	}

	errE = errors.New("field has unsupported or unexpected value type")
	errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
	errors.Details(errE)["type"] = fieldType.String()
	return nil, errE
}

// makeClaim creates a claim for the field for simple types of values (not slices, pointers, nor non-core structs).
//
// If the field is supported but has no value, it returns an errClaimNotMade error.
// If the field is not supported, it returns nil.
//
//nolint:ireturn,maintidx
func makeClaim(
	fieldValue reflect.Value,
	t reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	unit string,
	idPath []string,
	claims map[identifier.Identifier]int,
) (document.Claim, errors.E) {
	// Handle core.Ref.
	if t == coreRef {
		ref := fieldValue.Interface().(core.Ref) //nolint:errcheck,forcetypeassert
		if len(ref.ID) == 0 {
			return nil, errors.WithStack(errClaimNotMade)
		}
		refID := identifier.From(ref.ID...)

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.RelationClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
			To:   document.Reference{ID: &refID},
		}, nil
	}

	// Handle core.Time.
	if t == coreTime {
		coreTime := fieldValue.Interface().(core.Time) //nolint:errcheck,forcetypeassert
		if coreTime.Timestamp.IsZero() {
			return nil, errors.WithStack(errClaimNotMade)
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:      document.Reference{ID: &propertyID},
			Timestamp: document.Timestamp(coreTime.Timestamp),
			Precision: coreTime.Precision,
		}, nil
	}

	// Handle core.Interval.
	if t == coreInterval {
		interval := fieldValue.Interface().(core.Interval) //nolint:errcheck,forcetypeassert

		if interval.From == nil && interval.To == nil && interval.FromIsUnknown && interval.ToIsUnknown {
			return nil, errors.WithStack(errClaimNotMade)
		}

		// TODO: This is just temporary. Support unknown interval bounds.
		if interval.From == nil || interval.To == nil || interval.FromIsUnknown || interval.ToIsUnknown {
			claimID := newClaimID(idPath, propertyID, claims)
			return &document.UnknownValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
			}, nil
		}

		// TODO: Support different precisions for each bound.
		precision := interval.From.Precision
		if interval.To.Precision > precision {
			precision = interval.To.Precision
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TimeRangeClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:      document.Reference{ID: &propertyID},
			Lower:     document.Timestamp(interval.From.Timestamp),
			Upper:     document.Timestamp(interval.To.Timestamp),
			Precision: precision,
		}, nil
	}

	// Handle core.Identifier.
	if t == coreIdentifier {
		identifier := fieldValue.Interface().(core.Identifier) //nolint:errcheck,forcetypeassert
		if identifier == "" {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeID {
			return nil, errors.New("identifier field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)

		return &document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:  document.Reference{ID: &propertyID},
			Value: string(identifier),
		}, nil
	}

	// Handle core.IRI.
	if t == coreIRI {
		iri := fieldValue.Interface().(core.IRI) //nolint:errcheck,forcetypeassert
		if iri == "" {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeIRI {
			return nil, errors.New("IRI field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.ReferenceClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
			IRI:  string(iri),
		}, nil
	}

	// Handle core.HTML.
	if t == coreHTML {
		h := fieldValue.Interface().(core.HTML) //nolint:errcheck,forcetypeassert
		if h == "" {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeHTML {
			return nil, errors.New("HTML field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
			HTML: document.TranslatableHTMLString{
				// We still sanitize HTML, so that our user HTML is consistent.
				"en": sanitizeHTML(escapeHTML(string(h))),
			},
		}, nil
	}

	// Handle core.RawHTML.
	if t == coreRawHTML {
		rawHTML := fieldValue.Interface().(core.RawHTML) //nolint:errcheck,forcetypeassert
		if rawHTML == "" {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeRawHTML {
			return nil, errors.New("raw HTML field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
			HTML: document.TranslatableHTMLString{
				// No escaping for raw HTML, but we do sanitize it.
				"en": sanitizeHTML(string(rawHTML)),
			},
		}, nil
	}

	// Handle core.None.
	if t == coreNone {
		none := fieldValue.Interface().(core.None) //nolint:errcheck,forcetypeassert
		if !bool(none) {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeNone {
			return nil, errors.New("none field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.NoValueClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
		}, nil
	}

	// Handle core.Unknown.
	if t == coreUnknown {
		unknown := fieldValue.Interface().(core.Unknown) //nolint:errcheck,forcetypeassert
		if !bool(unknown) {
			return nil, errors.WithStack(errClaimNotMade)
		}

		if typeTag != "" && typeTag != typeUnknown {
			return nil, errors.New("unknown field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.UnknownValueClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
		}, nil
	}

	// Handle string types.
	if fieldValue.Kind() == reflect.String {
		str := fieldValue.String()
		if str == "" {
			return nil, errors.WithStack(errClaimNotMade)
		}

		claimID := newClaimID(idPath, propertyID, claims)

		if typeTag == typeID {
			return &document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop:  document.Reference{ID: &propertyID},
				Value: str,
			}, nil
		}

		if typeTag == typeHTML {
			return &document.TextClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
				HTML: document.TranslatableHTMLString{
					// We still sanitize HTML, so that our user HTML is consistent.
					"en": sanitizeHTML(escapeHTML(str)),
				},
			}, nil
		}

		if typeTag == typeRawHTML {
			return &document.TextClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
				HTML: document.TranslatableHTMLString{
					// No escaping for raw HTML, but we do sanitize it.
					"en": sanitizeHTML(str),
				},
			}, nil
		}

		if typeTag == typeIRI {
			return &document.ReferenceClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
				IRI:  str,
			}, nil
		}

		return &document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:   document.Reference{ID: &propertyID},
			String: str,
		}, nil
	}

	// Handle bool.
	if fieldValue.Kind() == reflect.Bool {
		if !fieldValue.Bool() {
			return nil, errors.WithStack(errClaimNotMade)
		}

		claimID := newClaimID(idPath, propertyID, claims)

		if typeTag == typeNone {
			return &document.NoValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
			}, nil
		}

		if typeTag == typeUnknown {
			return &document.UnknownValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: document.HighConfidence,
				},
				Prop: document.Reference{ID: &propertyID},
			}, nil
		}

		// TODO: Make this better. We currently map true to NoValueClaim, but we should to HasClaim.
		return &document.NoValueClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop: document.Reference{ID: &propertyID},
		}, nil
	}

	// Handle int types.
	if fieldValue.Kind() >= reflect.Int && fieldValue.Kind() <= reflect.Int64 {
		if unit == "" {
			return nil, errors.New(`field has numeric type but is missing required "unit" tag`)
		}

		u, errE := parseAmountUnit(unit)
		if errE != nil {
			return nil, errE
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:   document.Reference{ID: &propertyID},
			Amount: float64(fieldValue.Int()),
			Unit:   u,
		}, nil
	}

	// Handle uint types.
	if fieldValue.Kind() >= reflect.Uint && fieldValue.Kind() <= reflect.Uint64 {
		if unit == "" {
			return nil, errors.New(`field has numeric type but is missing required "unit" tag`)
		}

		u, errE := parseAmountUnit(unit)
		if errE != nil {
			return nil, errE
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:   document.Reference{ID: &propertyID},
			Amount: float64(fieldValue.Uint()),
			Unit:   u,
		}, nil
	}

	// Handle float types.
	if fieldValue.Kind() == reflect.Float32 || fieldValue.Kind() == reflect.Float64 {
		if unit == "" {
			return nil, errors.New(`field has numeric type but is missing required "unit" tag`)
		}

		u, errE := parseAmountUnit(unit)
		if errE != nil {
			return nil, errE
		}

		amount := fieldValue.Float()
		if math.IsInf(amount, 0) || math.IsNaN(amount) {
			errE := errors.New("value is infinity or not a number")
			errors.Details(errE)["value"] = amount
			return nil, errE
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: document.HighConfidence,
			},
			Prop:   document.Reference{ID: &propertyID},
			Amount: amount,
			Unit:   u,
		}, nil
	}

	return nil, nil //nolint:nilnil
}

// newClaimID returns a new claim ID based on property and existing claims.
func newClaimID(idPath []string, propertyID identifier.Identifier, claims map[identifier.Identifier]int) identifier.Identifier {
	i := claims[propertyID]
	claims[propertyID] = i + 1
	newIDPath := append(slices.Clone(idPath), strconv.Itoa(i))
	claimID := identifier.From(newIDPath...)
	return claimID
}

// parseAmountUnit parses a unit tag string and returns the corresponding AmountUnit.
func parseAmountUnit(unit string) (document.AmountUnit, errors.E) {
	var u document.AmountUnit
	jsonBytes := []byte(`"` + unit + `"`)
	err := u.UnmarshalJSON(jsonBytes)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return u, nil
}

// parseCardinality parses a cardinality tag string and returns min and max values.
//
// Supported formats:
//   - "1" - exactly one (min=1, max=1)
//   - "1.." - one or more (min=1, max=-1 for unbounded)
//   - "0..1" - zero or one (min=0, max=1)
//   - "0.." - zero or more (min=0, max=-1 for unbounded)
//   - "2..5" - between 2 and 5 (min=2, max=5)
//
// Default cardinality, if not specified, is min=0, max=-1.
//
// Returns (minCardinality, maxCardinality) where maxCardinality=-1 means unbounded.
func parseCardinality(cardinality string, fieldValue reflect.Value, hasDefault bool, fieldPath []string) (int, int, errors.E) {
	minCardinality := 0
	maxCardinality := -1

	isPointer := fieldValue.Kind() == reflect.Ptr
	isSlice := fieldValue.Kind() == reflect.Slice
	isSingleValue := !isPointer && !isSlice
	isBooleanField := fieldValue.Kind() == reflect.Bool

	if cardinality != "" { //nolint:nestif
		if strings.Contains(cardinality, "..") {
			parts := strings.Split(cardinality, "..")
			if len(parts) != 2 { //nolint:mnd
				errE := errors.New("invalid cardinality format")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errE
			}

			minStr := strings.TrimSpace(parts[0])
			if minStr == "" {
				errE := errors.New("cardinality min value is empty")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errE
			}
			var err error
			minCardinality, err = strconv.Atoi(minStr)
			if err != nil {
				errE := errors.New("cardinality min value is not a valid integer")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errors.WrapWith(err, errE)
			}
			if minCardinality < 0 {
				errE := errors.New("cardinality min value cannot be negative")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errE
			}

			maxStr := strings.TrimSpace(parts[1])
			// Unbounded max is default.
			if maxStr != "" {
				maxCardinality, err = strconv.Atoi(maxStr)
				if err != nil {
					errE := errors.New("cardinality max value is not a valid integer")
					errors.Details(errE)["cardinality"] = cardinality
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return 0, 0, errors.WrapWith(err, errE)
				}
				if maxCardinality <= 0 {
					errE := errors.New("cardinality max value cannot be negative or zero")
					errors.Details(errE)["cardinality"] = cardinality
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return 0, 0, errE
				}
				if maxCardinality < minCardinality {
					errE := errors.New("cardinality max value cannot be less than min")
					errors.Details(errE)["cardinality"] = cardinality
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return 0, 0, errE
				}
			}
		} else {
			val, err := strconv.Atoi(strings.TrimSpace(cardinality))
			if err != nil {
				errE := errors.New("cardinality value is not a valid integer")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errors.WrapWith(err, errE)
			}
			if val <= 0 {
				errE := errors.New("cardinality value cannot be negative or zero")
				errors.Details(errE)["cardinality"] = cardinality
				errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
				return 0, 0, errE
			}
			minCardinality = val
			maxCardinality = val
		}
	}

	if (isPointer || isSingleValue || isBooleanField) && maxCardinality == -1 && cardinality == "" {
		maxCardinality = 1
	}

	// Pointer fields: max must be <= 1 (can be 0 or 1).
	if isPointer && (maxCardinality > 1 || maxCardinality == -1) {
		errE := errors.New("pointer field cannot have max cardinality greater than 1")
		errors.Details(errE)["cardinality"] = cardinality
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return 0, 0, errE
	}

	// Single value fields: max must be <= 1 (can be 0 or 1).
	if isSingleValue && (maxCardinality > 1 || maxCardinality == -1) {
		errE := errors.New("single value field cannot have max cardinality greater than 1")
		errors.Details(errE)["cardinality"] = cardinality
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return 0, 0, errE
	}

	// Boolean fields: max must be <= 1 (can be 0 or 1).
	if isBooleanField && (maxCardinality > 1 || maxCardinality == -1) {
		errE := errors.New("boolean field cannot have max cardinality greater than 1")
		errors.Details(errE)["cardinality"] = cardinality
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return 0, 0, errE
	}

	// With default tag, min cardinality must be > 0.
	if minCardinality == 0 && hasDefault {
		errE := errors.New("field cannot have default tag with min cardinality 0")
		errors.Details(errE)["cardinality"] = cardinality
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return 0, 0, errE
	}

	return minCardinality, maxCardinality, nil
}

// ExtractDocumentID extracts the document ID from a struct.
//
// It finds a field with tag "documentid" and returns its value as a slice of strings.
// If no such field is found, it returns an ErrDocumentIDNotFound error.
func ExtractDocumentID(doc any) ([]string, errors.E) {
	v := reflect.ValueOf(doc)
	// Handle pointer to struct.
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		errE := errors.New("expected struct")
		errors.Details(errE)["got"] = v.Kind().String()
		return nil, errE
	}

	return extractDocumentID(v, v.Type(), []string{})
}

// extractDocumentID extracts the document ID from a struct.
//
// It finds a field with tag "documentid" and returns its value as a slice of strings.
// If no such field is found, it returns an ErrDocumentIDNotFound error.
func extractDocumentID(v reflect.Value, t reflect.Type, fieldPath []string) ([]string, errors.E) {
	ids := [][]string{}

	for i := range t.NumField() {
		field := t.Field(i)
		fieldValue := v.Field(i)
		fieldType := fieldValue.Type()

		newFieldPath := append(slices.Clone(fieldPath), field.Name)

		// We use Lookup because the tag has empty value.
		if _, ok := field.Tag.Lookup("documentid"); ok {
			if fieldValue.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.String {
				id := make([]string, fieldValue.Len())
				for j := range fieldValue.Len() {
					id[j] = fieldValue.Index(j).String()
				}
				if len(id) > 0 {
					ids = append(ids, id)
					continue
				}
				errE := errors.New("empty ID")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			errE := errors.New("document ID field is not a string slice")
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return nil, errE
		}

		// If this is an embedded struct, recursively check its fields.
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			id, errE := extractDocumentID(fieldValue, fieldType, newFieldPath)
			if errors.Is(errE, ErrDocumentIDNotFound) {
				continue
			} else if errE != nil {
				return nil, errE
			}
			ids = append(ids, id)
		}
	}

	if len(ids) > 1 {
		errE := errors.New("multiple document IDs found")
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return nil, errE
	} else if len(ids) == 1 {
		return ids[0], nil
	}

	return nil, errors.WithStack(ErrDocumentIDNotFound)
}
