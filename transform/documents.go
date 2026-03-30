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
//		Age  int      `property:"AGE"`
//	}
//
//	mnemonics := map[string][]string{
//		"NAME": {"people.example.com", "NAME"},
//		"AGE":  {"people.example.com", "AGE"},
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
//	documents, err := transform.Documents(ctx, mnemonics, docs)
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
// becomes the main claim, while other fields in the struct become sub-claims.
//
//	type PersonName struct {
//		Value  string               `value:""`
//		Period core.Interval[core.Time] `property:"PERIOD"`
//	}
//
// Cannot be combined with property and cardinality tags.
// Those tags belong to the field which uses the nested struct the value field is in.
//
// ## type
//
// Specifies how to interpret fields. Supported types for string fields:
//
//   - "id": create an identifier claim,
//   - "link": create a link claim,
//   - "file": create a link claim,
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
//	Homepage  string `property:"HOMEPAGE" type:"link"`
//	Bio       string `property:"BIO" type:"html"`
//	IsAbsent  bool   `property:"NAME" type:"none"`
//	IsUnknown bool   `property:"AGE" type:"unknown"`
//
// ## precision
//
// Required for bare numeric types (int, float, etc.) and time.Time.
// Must not be used with any other field type (use core.Amount[T] or core.Time for their built-in precision).
//
// For bare numeric types, precision is a floating-point number representing measurement precision:
//
//	Height float64   `property:"HEIGHT" precision:"0.01"`
//	Year   int       `property:"YEAR"   precision:"1"`
//
// For time.Time, precision is one of the TimePrecision string codes:
// "G", "100M", "10M", "M", "100k", "10k", "k", "100y", "10y", "y", "m", "d", "h", "min", "s", "ms", "us", "ns".
//
//	Born time.Time `property:"BORN" precision:"d"`
//
// ## location
//
// Optional. Only supported for time.Time, core.Time, and core.Interval[core.Time] fields.
// Specifies the timezone for formatting the time. The value must be a valid IANA timezone name
// (e.g., "America/New_York", "Europe/London", "UTC"). Parsed using time.LoadLocation.
// When not specified, UTC is used.
//
//	Born  time.Time `property:"BORN"  precision:"d" location:"America/New_York"`
//	Start core.Time `property:"START" location:"Europe/Ljubljana"`
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
//   - pointer fields: max must be <= 1 (can be 0 or 1),
//   - single value fields: max must be <= 1 (can be 0 or 1),
//   - max cardinality cannot be 0.
//
// Default: min=0, max=unbounded for slices; min=0, max=1 for pointer and single value fields.
//
//	Required []string `property:"NAME" cardinality:"1.."` // At least one required.
//	Optional *int     `property:"AGE" cardinality:"0..1"` // Zero or one.
//	Exactly2 []string `property:"IDS" cardinality:"2"`    // Exactly two.
//
// ## default
//
// On regular fields, specifies the default claim to create when the field doesn't
// satisfy minimum cardinality, instead of returning an error.
// Can only be used with cardinality tag when min > 0.
// The number of claims added equals (min - actual count).
//
// On value fields (fields with value:""), specifies what claim to create when the
// value field is empty.
// Does not require a cardinality tag when used on value fields.
//
// Supported values:
//
//   - "none": add none-value claim(s),
//   - "unknown": add unknown-value claim(s).
//
// Example (regular field):
//
//	Name []string `property:"NAME" cardinality:"1.." default:"none"`
//
// Example (value field):
//
//	type PersonWeight struct {
//		Value     *int      `value:"" default:"unknown"`
//		Time core.Time `property:"TIMESTAMP"`
//	}
//
// Semantic difference between values:
//   - none: it is known that the value does not exist,
//   - unknown: it is known that the value exists but is unknown or cannot be determined.
//
// ## confidence
//
// Optional. Overrides the confidence level of the created claim(s).
// Must be a float in the range [-1, 1]. When not specified, uses document.HighConfidence (1.0).
//
// Positive values indicate confidence in the claim, negative values indicate
// confidence in the negation of the claim.
//
// Cannot be used on value fields (fields with value:"" tag); use it on the enclosing
// field with property:"" instead.
//
//	Name string `property:"NAME" confidence:"0.75"` // Medium confidence.
//	Age  int    `property:"AGE"  confidence:"-0.5" precision:"1"` // Low confidence in negation.
//
// # Field Types
//
// Supported field types:
//   - string: string claim (or identifier claim/link claim/text claim with type tag),
//   - int, int8, int16, int32, int64: amount claim (requires precision tag),
//   - uint, uint8, uint16, uint32, uint64: amount claim (requires precision tag),
//   - float32, float64: amount claim (requires precision tag),
//   - bool: has claim when true (or none-value/unknown-value claim with type tag),
//   - time.Time: time claim (requires precision tag),
//   - core.Ref: reference claim,
//   - core.Time: time claim,
//   - core.Amount[T]: amount claim,
//   - core.Interval[core.Time]: time interval claim,
//   - core.Interval[core.Amount[T]]: amount interval claim,
//   - core.Identifier: identifier claim,
//   - core.Link: link claim,
//   - core.HTML: text claim (with escaping),
//   - core.RawHTML: text claim (without escaping),
//   - core.None: none-value claim when true,
//   - core.Unknown: unknown-value claim when true,
//   - struct: nested claims (value field + sub-claims),
//   - []T: slice of any supported type,
//   - *T: pointer to any supported type.
//
// # Empty Values
//
// Empty values (non-numeric zero values) do not produce claims unless:
//   - field has cardinality with min > 0 and default:"none" tag: creates none-value claim(s),
//   - field has cardinality with min > 0 and default:"unknown" tag: creates unknown-value claim(s),
//   - field has cardinality with min > 0 without default tag: returns error,
//   - value field with default:"none" tag: creates a none-value claim,
//   - value field with default:"unknown" tag: creates an unknown-value claim,
//   - value field without a default tag: creates has claim with sub-claims,
//   - nested struct with empty value but has sub-claims: creates has claim with sub-claims.
//
// # Examples
//
// ## Basic Document
//
//	type Article struct {
//		ID    []string  `documentid:""`
//		Title string    `property:"TITLE"`
//		Views int       `property:"VIEWS" precision:"1"`
//		Born  time.Time `property:"BORN"  precision:"d"`
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
//		Value  string                   `value:""`
//		Period core.Interval[core.Time] `property:"PERIOD"`
//		Note   string                   `property:"NOTE"`
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
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
)

//nolint:gochecknoglobals
var (
	coreRef          = reflect.TypeFor[core.Ref]()
	coreTime         = reflect.TypeFor[core.Time]()
	timeTime         = reflect.TypeFor[time.Time]()
	coreTimeInterval = reflect.TypeFor[core.Interval[core.Time]]()
	coreIdentifier   = reflect.TypeFor[core.Identifier]()
	coreLink         = reflect.TypeFor[core.Link]()
	coreHTML         = reflect.TypeFor[core.HTML]()
	coreRawHTML      = reflect.TypeFor[core.RawHTML]()
	coreNone         = reflect.TypeFor[core.None]()
	coreUnknown      = reflect.TypeFor[core.Unknown]()

	coreAmountTypes = map[reflect.Type]bool{
		reflect.TypeFor[core.Amount[int]]():     true,
		reflect.TypeFor[core.Amount[int8]]():    true,
		reflect.TypeFor[core.Amount[int16]]():   true,
		reflect.TypeFor[core.Amount[int32]]():   true,
		reflect.TypeFor[core.Amount[int64]]():   true,
		reflect.TypeFor[core.Amount[uint]]():    true,
		reflect.TypeFor[core.Amount[uint8]]():   true,
		reflect.TypeFor[core.Amount[uint16]]():  true,
		reflect.TypeFor[core.Amount[uint32]]():  true,
		reflect.TypeFor[core.Amount[uint64]]():  true,
		reflect.TypeFor[core.Amount[float32]](): true,
		reflect.TypeFor[core.Amount[float64]](): true,
	}

	coreAmountIntervalTypes = map[reflect.Type]bool{
		reflect.TypeFor[core.Interval[core.Amount[int]]]():     true,
		reflect.TypeFor[core.Interval[core.Amount[int8]]]():    true,
		reflect.TypeFor[core.Interval[core.Amount[int16]]]():   true,
		reflect.TypeFor[core.Interval[core.Amount[int32]]]():   true,
		reflect.TypeFor[core.Interval[core.Amount[int64]]]():   true,
		reflect.TypeFor[core.Interval[core.Amount[uint]]]():    true,
		reflect.TypeFor[core.Interval[core.Amount[uint8]]]():   true,
		reflect.TypeFor[core.Interval[core.Amount[uint16]]]():  true,
		reflect.TypeFor[core.Interval[core.Amount[uint32]]]():  true,
		reflect.TypeFor[core.Interval[core.Amount[uint64]]]():  true,
		reflect.TypeFor[core.Interval[core.Amount[float32]]](): true,
		reflect.TypeFor[core.Interval[core.Amount[float64]]](): true,
	}
)

var ErrDocumentIDNotFound = errors.Base("document ID not found")

type claimNotMadeError struct {
	// For value claims we want to be able to pass its default value to consumer of the error
	// so that it can decide how to handle the error (value claim has default struct tag on itself).
	Default string
}

func (e *claimNotMadeError) Error() string {
	return "claim not made"
}

func (e *claimNotMadeError) Is(target error) bool {
	// We want to be able to use errors.Is(err, errClaimNotMade) for all claimNotMadeError errors.
	_, ok := target.(*claimNotMadeError)
	return ok
}

var (
	errClaimNotMade       = &claimNotMadeError{}
	errValueClaimNotFound = errors.Base("value claim not found")
)

const (
	defaultNone    = "none"
	defaultUnknown = "unknown"

	typeID      = "id"
	typeLink    = "link"
	typeFile    = "file"
	typeHTML    = "html"
	typeRawHTML = "rawhtml"
	typeNone    = "none"
	typeUnknown = "unknown"
)

// Documents transforms structs into PeerDB document.D documents.
//
// It takes a map between property mnemonics and their base IDs, and a slice of documents
// which can be various struct types. It uses reflection to inspect structs and their
// struct tags to determine how to map struct fields to document claims.
func Documents(ctx context.Context, mnemonics map[string][]string, documents []any) ([]*document.D, errors.E) {
	result := []*document.D{}

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
	Mnemonics map[string][]string
	Claims    *document.ClaimTypes
}

// transformDocument transforms a struct to a document.
func transformDocument(mnemonics map[string][]string, doc any) (*document.D, errors.E) {
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

	t := v.Type()

	// Extract document ID.
	docID, errE := extractDocumentID(v, t, []string{})
	if errE != nil {
		return nil, errE
	}

	result := &document.D{
		CoreDocument: document.CoreDocument{
			ID:   identifier.From(docID...),
			Base: docID,
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
		return nil, errE
	}

	return result, result.Validate()
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
		propertyBase, ok := tr.Mnemonics[propertyMnemonic]
		if !ok {
			errE := errors.New("mnemonic not found")
			errors.Details(errE)["name"] = propertyMnemonic
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}
		propertyID := identifier.From(propertyBase...)

		newIDPath := append(slices.Clone(idPath), propertyMnemonic)

		// Get tags.
		typeTag := field.Tag.Get("type")
		defaultTag := field.Tag.Get("default")
		precisionTag := field.Tag.Get("precision")
		locationTag := field.Tag.Get("location")
		confidenceTag := field.Tag.Get("confidence")
		cardinality := field.Tag.Get("cardinality")

		hasDefaultTag := defaultTag != ""

		if defaultTag != "" && defaultTag != defaultNone && defaultTag != defaultUnknown {
			errE := errors.New("default tag must be \"none\" or \"unknown\"")
			errors.Details(errE)["default"] = defaultTag
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}

		minCardinality, maxCardinality, errE := parseCardinality(cardinality, fieldValue, hasDefaultTag)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}

		confidence, errE := parseConfidence(confidenceTag)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}

		// Despite passing newIDPath here, we still pass existing claims. In fact, this is exactly the situation why
		// we have claims map in the first place: because if multiple fields add claims for the same property, we have
		// to track this to make sure claim IDs do not collide.
		errE = tr.processField(
			fieldValue, fieldType, propertyID, typeTag, defaultTag, precisionTag, locationTag, confidence, minCardinality, maxCardinality,
			newIDPath, newFieldPath, claims,
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
	precisionTag string,
	locationTag string,
	confidence document.Confidence,
	minCardinality int,
	maxCardinality int,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) errors.E {
	count := 0

	// Handle slices.
	if fieldValue.Kind() == reflect.Slice {
		for i := range fieldValue.Len() {
			elem := fieldValue.Index(i)
			errE := tr.processSingleValue(elem, elem.Type(), propertyID, typeTag, precisionTag, locationTag, confidence, idPath, fieldPath, claims)
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
		var err error = errClaimNotMade
		count = 0

		if !fieldValue.IsNil() {
			elem := fieldValue.Elem()
			err = tr.processSingleValue(elem, elem.Type(), propertyID, typeTag, precisionTag, locationTag, confidence, idPath, fieldPath, claims)
		}

		if errors.Is(err, errClaimNotMade) { //nolint:revive
			// Do nothing.
		} else if err != nil {
			// errors.WithStack will not really add a stack trace here because at this point
			// err comes from processSingleValue which returns errors.E.
			return errors.WithStack(err)
		} else {
			count++
		}

		// Handle single value.
	} else {
		errE := tr.processSingleValue(fieldValue, fieldType, propertyID, typeTag, precisionTag, locationTag, confidence, idPath, fieldPath, claims)

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
			// Add (minCardinality - count) NoneClaims.
			for range minCardinality - count {
				claimID := newClaimID(idPath, propertyID, claims)
				noneClaim := &document.NoneClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: confidence,
					},
					Prop: document.Reference{ID: propertyID},
				}
				errE := tr.Claims.Add(noneClaim)
				if errE != nil {
					errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
					return errE
				}
			}
		case defaultUnknown:
			// Add (minCardinality - count) UnknownClaims.
			for range minCardinality - count {
				claimID := newClaimID(idPath, propertyID, claims)
				unknownClaim := &document.UnknownClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: confidence,
					},
					Prop: document.Reference{ID: propertyID},
				}
				errE := tr.Claims.Add(unknownClaim)
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
// It returns an errClaimNotMade error if no claim has been made (e.g., the value was empty).
func (tr *transformer) processSingleValue(
	fieldValue reflect.Value,
	fieldType reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	precisionTag string,
	locationTag string,
	confidence document.Confidence,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) errors.E {
	claim, errE := makeClaim(fieldValue, fieldType, propertyID, typeTag, "", precisionTag, locationTag, confidence, idPath, claims)
	if errors.Is(errE, errClaimNotMade) {
		return errE
	} else if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return errE
	}

	// Handle structs.
	if claim == nil && fieldValue.Kind() == reflect.Struct {
		if precisionTag != "" {
			errE := errors.New("precision tag is not supported for struct field types")
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			return errE
		}

		if locationTag != "" {
			errE := errors.New("location tag is not supported for struct field types")
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			return errE
		}

		// We reconstruct the newIDPath under current propertyID-based claim count.
		// This is the same count which is used below to construct the value claim.
		newIDPath := append(slices.Clone(idPath), strconv.Itoa(claims[propertyID]))

		// Create transformer for sub-claims.
		subTr := transformer{
			Mnemonics: tr.Mnemonics,
			Claims:    &document.ClaimTypes{},
		}

		// Here we use newIDPath because all other claims are nested under the value claim as its sub-claims.
		// Because we use newIDPath, we can use an empty claims map because all claim IDs made under newIDPath
		// cannot collide with claim IDs made under idPath.
		errE = subTr.processStructFields(fieldValue, fieldType, newIDPath, fieldPath, map[identifier.Identifier]int{})
		if errE != nil {
			return errE
		}

		// Here we use idPath because this claim really belongs at the level above this struct.
		// It is only inside the struct so that we can list also its sub-claims next to it.
		claim, errE = extractValueClaim(fieldValue, fieldType, propertyID, confidence, idPath, fieldPath, claims)
		if e, ok := errors.AsType[*claimNotMadeError](errE); ok {
			if subTr.Claims.Size() == 0 {
				// There are no sub-claims nor a value claim, so we just return errClaimNotMade here.
				return errE
			}

			// There is a value claim defined, but in this particular instance it has empty value,
			// but there are sub-claims for it, so we make a claim for it.
			// TODO: What if all sub-claims are "none" claims?
			claimID := newClaimID(idPath, propertyID, claims)
			switch e.Default {
			case defaultNone:
				claim = &document.NoneClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: confidence,
					},
					Prop: document.Reference{ID: propertyID},
				}
			case defaultUnknown:
				claim = &document.UnknownClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: confidence,
					},
					Prop: document.Reference{ID: propertyID},
				}
			default:
				// By default, we make nested claims.
				claim = &document.HasClaim{
					CoreClaim: document.CoreClaim{
						ID:         claimID,
						Confidence: confidence,
					},
					Prop: document.Reference{ID: propertyID},
				}
			}
		} else if errors.Is(errE, errValueClaimNotFound) {
			if subTr.Claims.Size() == 0 {
				// There are no sub-claims nor a value claim, which makes nested claims be an empty value of sorts,
				// so we just return errClaimNotMade here.
				return errors.WithStack(errClaimNotMade)
			}

			// Value claim is not defined, so these are nested claims.
			claimID := newClaimID(idPath, propertyID, claims)
			claim = &document.HasClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
			}
		} else if errE != nil {
			return errE
		}

		// We copy all claims to the value claim as its sub-claims.
		for c := range subTr.Claims.AllClaims() {
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
// If such field is found, but it has an empty value, it returns a claimNotMadeError
// error (with default value set, if provided through a struct tag).
//
//nolint:ireturn
func extractValueClaim(
	structValue reflect.Value,
	structType reflect.Type,
	propertyID identifier.Identifier,
	confidence document.Confidence,
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

			if _, hasCardinality := field.Tag.Lookup("cardinality"); hasCardinality {
				errE := errors.New("cardinality tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			if _, hasConfidence := field.Tag.Lookup("confidence"); hasConfidence {
				errE := errors.New("confidence tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			// Get tags.
			typeTag := field.Tag.Get("type")
			defaultTag := field.Tag.Get("default")
			precisionTag := field.Tag.Get("precision")
			locationTag := field.Tag.Get("location")

			if defaultTag != "" && defaultTag != defaultNone && defaultTag != defaultUnknown {
				errE := errors.New("default tag must be \"none\" or \"unknown\"")
				errors.Details(errE)["default"] = defaultTag
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			claim, errE := processValueClaimField(fieldValue, fieldType, propertyID, typeTag, defaultTag, precisionTag, locationTag, confidence, idPath, newFieldPath, claims)
			if errE != nil {
				return nil, errE
			}

			valueClaimsAndErrors = append(valueClaimsAndErrors, claim)
			continue
		}

		// If this is an embedded struct, recursively check its fields.
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			vc, errE := extractValueClaim(fieldValue, fieldType, propertyID, confidence, idPath, newFieldPath, claims)
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
// If the field has an empty value, it returns a claimNotMadeError
// error (with default value set, if provided through a struct tag).
//
//nolint:ireturn
func processValueClaimField(
	fieldValue reflect.Value,
	fieldType reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	defaultTag string,
	precisionTag string,
	locationTag string,
	confidence document.Confidence,
	idPath []string,
	fieldPath []string,
	claims map[identifier.Identifier]int,
) (document.Claim, errors.E) {
	// Handle pointers.
	if fieldValue.Kind() == reflect.Ptr {
		if fieldValue.IsNil() {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		fieldValue = fieldValue.Elem()
		fieldType = fieldValue.Type()
	}

	claim, errE := makeClaim(fieldValue, fieldType, propertyID, typeTag, defaultTag, precisionTag, locationTag, confidence, idPath, claims)
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
// If the field is supported but has an empty value, it returns a claimNotMadeError
// error (with default value set, if provided through a struct tag).
// If the field is not supported, it returns nil.
//
//nolint:ireturn,maintidx
func makeClaim(
	fieldValue reflect.Value,
	t reflect.Type,
	propertyID identifier.Identifier,
	typeTag string,
	defaultTag string,
	precisionTag string,
	locationTag string,
	confidence document.Confidence,
	idPath []string,
	claims map[identifier.Identifier]int,
) (document.Claim, errors.E) {
	// Handle core.Ref.
	if t == coreRef {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for core.Ref fields")
		}

		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Ref fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Ref fields")
		}

		ref := fieldValue.Interface().(core.Ref) //nolint:errcheck,forcetypeassert
		if len(ref.ID) == 0 {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}
		refID := identifier.From(ref.ID...)

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.ReferenceClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
			To:   document.Reference{ID: refID},
		}, nil
	}

	// Handle time.Time.
	if t == timeTime {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for time.Time fields")
		}

		if precisionTag == "" {
			return nil, errors.New("precision tag is required for time.Time fields")
		}

		var precision document.TimePrecision
		errE := errors.WithStack(precision.UnmarshalText([]byte(precisionTag)))
		if errE != nil {
			errors.Details(errE)["precision"] = precisionTag
			return nil, errE
		}

		loc, errE := parseLocation(locationTag)
		if errE != nil {
			return nil, errE
		}

		goTime := fieldValue.Interface().(time.Time) //nolint:errcheck,forcetypeassert
		if goTime.IsZero() {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:      document.Reference{ID: propertyID},
			Time:      document.NewTime(goTime, precision, loc),
			Precision: precision,
		}, nil
	}

	// Handle core.Time.
	if t == coreTime {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for core.Time fields")
		}

		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Time fields; precision is part of core.Time")
		}

		loc, errE := parseLocation(locationTag)
		if errE != nil {
			return nil, errE
		}

		coreTime := fieldValue.Interface().(core.Time) //nolint:errcheck,forcetypeassert
		if coreTime.Time.IsZero() {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:      document.Reference{ID: propertyID},
			Time:      document.NewTime(coreTime.Time, coreTime.Precision, loc),
			Precision: coreTime.Precision,
		}, nil
	}

	// Handle core.Interval[core.Time].
	if t == coreTimeInterval {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for core.Interval[core.Time] fields")
		}

		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Interval[core.Time] fields; precision is part of core.Time")
		}

		loc, errE := parseLocation(locationTag)
		if errE != nil {
			return nil, errE
		}

		interval := fieldValue.Interface().(core.Interval[core.Time]) //nolint:errcheck,forcetypeassert

		// Return claimNotMadeError if interval is completely zero (no bounds, no flags).
		if interval.From == nil && !interval.FromIsOpen && !interval.FromIsUnknown && !interval.FromIsNone &&
			interval.To == nil && !interval.ToIsClosed && !interval.ToIsUnknown && !interval.ToIsNone {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		claim := &document.TimeIntervalClaim{ //nolint:exhaustruct
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
		}

		// Map From bound.
		if interval.From != nil {
			fromPrecision := interval.From.Precision
			fromTime := document.NewTime(interval.From.Time, fromPrecision, loc)
			claim.From = &fromTime
			claim.FromPrecision = &fromPrecision
			claim.FromIsOpen = interval.FromIsOpen
		} else if interval.FromIsUnknown {
			claim.FromIsUnknown = true
		} else if interval.FromIsNone {
			claim.FromIsNone = true
		} else {
			return nil, errors.New(`interval's "from" bound is not set`)
		}

		// Map To bound.
		if interval.To != nil {
			toPrecision := interval.To.Precision
			toTime := document.NewTime(interval.To.Time, toPrecision, loc)
			claim.To = &toTime
			claim.ToPrecision = &toPrecision
			claim.ToIsClosed = interval.ToIsClosed
		} else if interval.ToIsUnknown {
			claim.ToIsUnknown = true
		} else if interval.ToIsNone {
			claim.ToIsNone = true
		} else {
			return nil, errors.New(`interval's "to" bound is not set`)
		}

		return claim, nil
	}

	// Handle core.Interval[core.Amount[T]].
	if coreAmountIntervalTypes[t] { //nolint:nestif
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for core.Interval[core.Amount[T]] fields")
		}

		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Interval[core.Amount[T]] fields; precision is part of core.Amount")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Interval[core.Amount[T]] fields")
		}

		// Interval struct field indices: 0=From, 1=FromIsOpen, 2=FromIsUnknown, 3=FromIsNone, 4=To, 5=ToIsClosed, 6=ToIsUnknown, 7=ToIsNone.
		fromField := fieldValue.Field(0)
		fromIsOpen := fieldValue.Field(1).Bool()
		fromIsUnknown := fieldValue.Field(2).Bool() //nolint:mnd
		fromIsNone := fieldValue.Field(3).Bool()    //nolint:mnd
		toField := fieldValue.Field(4)              //nolint:mnd
		toIsClosed := fieldValue.Field(5).Bool()    //nolint:mnd
		toIsUnknown := fieldValue.Field(6).Bool()   //nolint:mnd
		toIsNone := fieldValue.Field(7).Bool()      //nolint:mnd

		// Return claimNotMadeError if interval is completely zero (no bounds, no flags).
		if fromField.IsNil() && !fromIsOpen && !fromIsUnknown && !fromIsNone &&
			toField.IsNil() && !toIsClosed && !toIsUnknown && !toIsNone {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		claim := &document.AmountIntervalClaim{ //nolint:exhaustruct
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
		}

		// Map From bound.
		if !fromField.IsNil() { //nolint:dupl
			fromAmount, ok := getNumericValue(fromField.Elem().Field(0))
			if !ok {
				errE := errors.New("unexpected from kind")
				errors.Details(errE)["kind"] = fromField.Elem().Field(0).Kind().String()
				panic(errE)
			}
			fromPrecision, ok := getNumericValue(fromField.Elem().Field(1))
			if !ok {
				errE := errors.New("unexpected from precision kind")
				errors.Details(errE)["kind"] = fromField.Elem().Field(1).Kind().String()
				panic(errE)
			}
			if math.IsInf(fromAmount, 0) || math.IsNaN(fromAmount) {
				errE := errors.New(`interval's "from" is infinity or not a number`)
				errors.Details(errE)["value"] = fromAmount
				return nil, errE
			}
			fromAmt := document.NewAmount(fromAmount, fromPrecision)
			claim.From = &fromAmt
			claim.FromPrecision = &fromPrecision
			claim.FromIsOpen = fromIsOpen
		} else if fromIsUnknown {
			claim.FromIsUnknown = true
		} else if fromIsNone {
			claim.FromIsNone = true
		} else {
			return nil, errors.New(`interval's "from" bound is not set`)
		}

		// Map To bound.
		if !toField.IsNil() { //nolint:dupl
			toAmount, ok := getNumericValue(toField.Elem().Field(0))
			if !ok {
				errE := errors.New("unexpected to kind")
				errors.Details(errE)["kind"] = toField.Elem().Field(0).Kind().String()
				panic(errE)
			}
			toPrecision, ok := getNumericValue(toField.Elem().Field(1))
			if !ok {
				errE := errors.New("unexpected to precision kind")
				errors.Details(errE)["kind"] = toField.Elem().Field(1).Kind().String()
				panic(errE)
			}
			if math.IsInf(toAmount, 0) || math.IsNaN(toAmount) {
				errE := errors.New(`interval's "to" is infinity or not a number`)
				errors.Details(errE)["value"] = toAmount
				return nil, errE
			}
			toAmt := document.NewAmount(toAmount, toPrecision)
			claim.To = &toAmt
			claim.ToPrecision = &toPrecision
			claim.ToIsClosed = toIsClosed
		} else if toIsUnknown {
			claim.ToIsUnknown = true
		} else if toIsNone {
			claim.ToIsNone = true
		} else {
			return nil, errors.New(`interval's "to" bound is not set`)
		}

		return claim, nil
	}

	// Handle core.Amount[T].
	if coreAmountTypes[t] {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for core.Amount[T] fields")
		}

		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Amount[T] fields; precision is part of core.Amount")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Amount[T] fields")
		}

		amount, ok := getNumericValue(fieldValue.Field(0))
		if !ok {
			errE := errors.New("unexpected amount kind")
			errors.Details(errE)["kind"] = fieldValue.Field(0).Kind().String()
			panic(errE)
		}
		precision, ok := getNumericValue(fieldValue.Field(1))
		if !ok {
			errE := errors.New("unexpected precision kind")
			errors.Details(errE)["kind"] = fieldValue.Field(1).Kind().String()
			panic(errE)
		}

		if math.IsInf(precision, 0) || math.IsNaN(precision) || precision <= 0 {
			errE := errors.New("precision must be finite positive number")
			errors.Details(errE)["precision"] = precision
			return nil, errE
		}

		if math.IsInf(amount, 0) || math.IsNaN(amount) {
			errE := errors.New("value must be a finite number")
			errors.Details(errE)["value"] = amount
			return nil, errE
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:      document.Reference{ID: propertyID},
			Amount:    document.NewAmount(amount, precision),
			Precision: precision,
		}, nil
	}

	// Handle core.Identifier.
	if t == coreIdentifier {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Identifier fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Identifier fields")
		}

		identifier := fieldValue.Interface().(core.Identifier) //nolint:errcheck,forcetypeassert
		if identifier == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeID {
			return nil, errors.New("identifier field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)

		return &document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:  document.Reference{ID: propertyID},
			Value: string(identifier),
		}, nil
	}

	// Handle core.Link.
	if t == coreLink {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Link fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Link fields")
		}

		link := fieldValue.Interface().(core.Link) //nolint:errcheck,forcetypeassert
		if link == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeLink {
			return nil, errors.New("link field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.LinkClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
			IRI:  string(link),
		}, nil
	}

	// Handle core.HTML.
	if t == coreHTML {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.HTML fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.HTML fields")
		}

		h := fieldValue.Interface().(core.HTML) //nolint:errcheck,forcetypeassert
		if h == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeHTML {
			return nil, errors.New("HTML field used with conflicting tag")
		}

		// We still sanitize HTML, so that our user HTML is consistent.
		sanitized := sanitizeHTML(escapeHTML(string(h)))
		if sanitized == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.HTMLClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
			HTML: sanitized,
		}, nil
	}

	// Handle core.RawHTML.
	if t == coreRawHTML {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.RawHTML fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.RawHTML fields")
		}

		rawHTML := fieldValue.Interface().(core.RawHTML) //nolint:errcheck,forcetypeassert
		if rawHTML == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeRawHTML {
			return nil, errors.New("raw HTML field used with conflicting tag")
		}

		// No escaping for raw HTML, but we do sanitize it.
		sanitized := sanitizeHTML(string(rawHTML))
		if sanitized == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.HTMLClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
			HTML: sanitized,
		}, nil
	}

	// Handle core.None.
	if t == coreNone {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.None fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.None fields")
		}

		none := fieldValue.Interface().(core.None) //nolint:errcheck,forcetypeassert
		if !bool(none) {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeNone {
			return nil, errors.New("none field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.NoneClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
		}, nil
	}

	// Handle core.Unknown.
	if t == coreUnknown {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for core.Unknown fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for core.Unknown fields")
		}

		unknown := fieldValue.Interface().(core.Unknown) //nolint:errcheck,forcetypeassert
		if !bool(unknown) {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		if typeTag != "" && typeTag != typeUnknown {
			return nil, errors.New("unknown field used with conflicting tag")
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.UnknownClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
		}, nil
	}

	// Handle string types.
	if fieldValue.Kind() == reflect.String {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for string fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for string fields")
		}

		str := fieldValue.String()
		if str == "" {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)

		if typeTag == typeID {
			return &document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop:  document.Reference{ID: propertyID},
				Value: str,
			}, nil
		}

		if typeTag == typeHTML {
			// We still sanitize HTML, so that our user HTML is consistent.
			sanitized := sanitizeHTML(escapeHTML(str))
			if sanitized == "" {
				return nil, errors.WithStack(&claimNotMadeError{
					Default: defaultTag,
				})
			}
			return &document.HTMLClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
				HTML: sanitized,
			}, nil
		}

		if typeTag == typeRawHTML {
			// No escaping for raw HTML, but we do sanitize it.
			sanitized := sanitizeHTML(str)
			if sanitized == "" {
				return nil, errors.WithStack(&claimNotMadeError{
					Default: defaultTag,
				})
			}
			return &document.HTMLClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
				HTML: sanitized,
			}, nil
		}

		if typeTag == typeLink || typeTag == typeFile {
			return &document.LinkClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
				IRI:  str,
			}, nil
		}

		if typeTag != "" {
			return nil, errors.New("string field used with unsupported type tag")
		}

		return &document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:   document.Reference{ID: propertyID},
			String: str,
		}, nil
	}

	// Handle bool.
	if fieldValue.Kind() == reflect.Bool {
		if precisionTag != "" {
			return nil, errors.New("precision tag is not supported for bool fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for bool fields")
		}

		if !fieldValue.Bool() {
			return nil, errors.WithStack(&claimNotMadeError{
				Default: defaultTag,
			})
		}

		claimID := newClaimID(idPath, propertyID, claims)

		if typeTag == typeNone {
			return &document.NoneClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
			}, nil
		}

		if typeTag == typeUnknown {
			return &document.UnknownClaim{
				CoreClaim: document.CoreClaim{
					ID:         claimID,
					Confidence: confidence,
				},
				Prop: document.Reference{ID: propertyID},
			}, nil
		}

		if typeTag != "" {
			return nil, errors.New("bool field used with unsupported type tag")
		}

		return &document.HasClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop: document.Reference{ID: propertyID},
		}, nil
	}

	// Handle numeric types.
	if amount, ok := getNumericValue(fieldValue); ok {
		if typeTag != "" {
			return nil, errors.New("type tag is not supported for numeric fields")
		}

		if precisionTag == "" {
			return nil, errors.New("precision tag is required for numeric fields")
		}

		if locationTag != "" {
			return nil, errors.New("location tag is not supported for numeric fields")
		}

		precision, err := strconv.ParseFloat(precisionTag, 64)
		if err != nil {
			errE := errors.Wrap(err, "precision tag is not a valid number")
			errors.Details(errE)["precision"] = precisionTag
			return nil, errE
		}

		if math.IsInf(precision, 0) || math.IsNaN(precision) || precision <= 0 {
			errE := errors.New("precision must be finite positive number")
			errors.Details(errE)["precision"] = precision
			return nil, errE
		}

		if math.IsInf(amount, 0) || math.IsNaN(amount) {
			errE := errors.New("value must be a finite number")
			errors.Details(errE)["value"] = amount
			return nil, errE
		}

		claimID := newClaimID(idPath, propertyID, claims)
		return &document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: confidence,
			},
			Prop:      document.Reference{ID: propertyID},
			Amount:    document.NewAmount(amount, precision),
			Precision: precision,
		}, nil
	}

	return nil, nil //nolint:nilnil
}

// parseLocation parses a location tag string using time.LoadLocation.
// Returns nil if the tag is empty (defaults to UTC in document.NewTime).
func parseLocation(locationTag string) (*time.Location, errors.E) {
	if locationTag == "" {
		return nil, nil //nolint:nilnil
	}
	loc, err := time.LoadLocation(locationTag)
	if err != nil {
		errE := errors.Wrap(err, "invalid location")
		errors.Details(errE)["location"] = locationTag
		return nil, errE
	}
	return loc, nil
}

// newClaimID returns a new claim ID based on property and existing claims.
func newClaimID(idPath []string, propertyID identifier.Identifier, claims map[identifier.Identifier]int) identifier.Identifier {
	i := claims[propertyID]
	claims[propertyID] = i + 1
	newIDPath := append(slices.Clone(idPath), strconv.Itoa(i))
	claimID := identifier.From(newIDPath...)
	return claimID
}

// getNumericValue returns the numeric value from a reflect.Value of a numeric kind as float64.
func getNumericValue(v reflect.Value) (float64, bool) {
	switch v.Kind() { //nolint:exhaustive
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	default:
		return 0, false
	}
}

// parseCardinalityTag parses a cardinality tag string and returns min and max values.
//
// Supported formats:
//   - "1" - exactly one (min=1, max=1)
//   - "1.." - one or more (min=1, max=-1 for unbounded)
//   - "0..1" - zero or one (min=0, max=1)
//   - "0.." - zero or more (min=0, max=-1 for unbounded)
//   - "2..5" - between 2 and 5 (min=2, max=5)
//
// If cardinality is empty, returns (0, -1) where -1 means unbounded.
//
// Returns (minCardinality, maxCardinality) where maxCardinality=-1 means unbounded.
func parseCardinalityTag(cardinality string) (int, int, errors.E) {
	if cardinality == "" {
		return 0, -1, nil
	}

	if strings.Contains(cardinality, "..") {
		parts := strings.Split(cardinality, "..")
		if len(parts) != 2 { //nolint:mnd
			errE := errors.New("invalid cardinality format")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}
		minStr := strings.TrimSpace(parts[0])
		if minStr == "" {
			errE := errors.New("cardinality min value is empty")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}
		minCardinality, err := strconv.Atoi(minStr)
		if err != nil {
			errE := errors.New("cardinality min value is not a valid integer")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errors.WrapWith(err, errE)
		}
		if minCardinality < 0 {
			errE := errors.New("cardinality min value cannot be negative")
			errors.Details(errE)["cardinality"] = cardinality
			return 0, 0, errE
		}

		maxCardinality := -1
		maxStr := strings.TrimSpace(parts[1])
		if maxStr != "" {
			maxCardinality, err = strconv.Atoi(maxStr)
			if err != nil {
				errE := errors.New("cardinality max value is not a valid integer")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errors.WrapWith(err, errE)
			}
			if maxCardinality <= 0 {
				errE := errors.New("cardinality max value cannot be negative or zero")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errE
			}
			if maxCardinality < minCardinality {
				errE := errors.New("cardinality max value cannot be less than min")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errE
			}
		}

		return minCardinality, maxCardinality, nil
	}

	val, err := strconv.Atoi(strings.TrimSpace(cardinality))
	if err != nil {
		errE := errors.New("cardinality value is not a valid integer")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errors.WrapWith(err, errE)
	}
	if val <= 0 {
		errE := errors.New("cardinality value cannot be negative or zero")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errE
	}

	return val, val, nil
}

// parseCardinality parses a cardinality tag string and validates it against the Go field type.
//
// It enforces that pointer and single-value fields have max cardinality <= 1,
// and that the default tag requires min cardinality > 0.
//
// Returns (minCardinality, maxCardinality) where maxCardinality=-1 means unbounded.
func parseCardinality(cardinality string, fieldValue reflect.Value, hasDefault bool) (int, int, errors.E) {
	minCardinality, maxCardinality, errE := parseCardinalityTag(cardinality)
	if errE != nil {
		return 0, 0, errE
	}

	isPointer := fieldValue.Kind() == reflect.Ptr
	isSlice := fieldValue.Kind() == reflect.Slice
	// isSingleValue is true for all non-pointer, non-slice fields, including bool.
	isSingleValue := !isPointer && !isSlice

	if (isPointer || isSingleValue) && maxCardinality == -1 && cardinality == "" {
		maxCardinality = 1
	}

	// Pointer fields: max must be <= 1 (can be 0 or 1).
	if isPointer && (maxCardinality > 1 || maxCardinality == -1) {
		errE := errors.New("pointer field cannot have max cardinality greater than 1")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errE
	}

	// Single value fields (including bool): max must be <= 1 (can be 0 or 1).
	if isSingleValue && (maxCardinality > 1 || maxCardinality == -1) {
		errE := errors.New("single value field cannot have max cardinality greater than 1")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errE
	}

	// With default tag, min cardinality must be > 0.
	if minCardinality == 0 && hasDefault {
		errE := errors.New("field cannot have default tag with min cardinality 0")
		errors.Details(errE)["cardinality"] = cardinality
		return 0, 0, errE
	}

	return minCardinality, maxCardinality, nil
}

// parseConfidence parses a confidence tag string and returns a document.Confidence value.
//
// If the tag is empty, it returns document.HighConfidence.
// The confidence value must be a float in the range [-1, 1].
func parseConfidence(tag string) (document.Confidence, errors.E) {
	if tag == "" {
		return document.HighConfidence, nil
	}

	v, err := strconv.ParseFloat(tag, 64)
	if err != nil {
		errE := errors.Wrap(err, "confidence tag is not a valid number")
		errors.Details(errE)["confidence"] = tag
		return 0, errE
	}

	if v < -1 || v > 1 || math.IsInf(v, 0) || math.IsNaN(v) {
		errE := errors.New("confidence is out of range [-1, 1]")
		errors.Details(errE)["confidence"] = v
		return 0, errE
	}

	return document.Confidence(v), nil
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
