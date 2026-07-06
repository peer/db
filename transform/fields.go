package transform

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// Fields extracts field descriptions from a struct type using struct tags.
//
// It reads the same struct tags as [Documents] (property, cardinality, type, value)
// plus additional tags (section, order, values) to produce a [core.Fields]
// describing the struct's field schema.
//
// The section tag can be used on embedded structs to define a section (with its order)
// and set the default section for fields inside. The section tag can also be used on
// individual fields to assign them to a section. Fields without a section tag (and not
// inside an embedded struct with a section tag) are placed in the top-level Fields.Field.
//
// Every referenced section must have its order defined via an embedded struct with
// the section tag. Sub-fields cannot have sections.
//
// The mnemonics parameter maps property mnemonic names to property document
// base IDs. Passing nil for mnemonics short-circuits the function and returns
// (nil, nil).
//
// The sections parameter maps section names (as used in section tags) to their
// translated names, keyed by language (e.g. "en-GB"). Each section becomes NAME
// claims (one per language) with the language as an IN_LANGUAGE sub-claim. Every
// section used by the struct must have its names provided; if one is missing (or
// sections is nil while the struct uses sections), an error is returned.
//
// The instructions parameter maps field paths to their translated instructions
// (raw HTML strings, expected to be formatted as paragraphs), keyed by language
// (e.g. "en-GB"). A field path is the dot-joined Go field names from the root of T
// down to the field, including embedded structs by their field name (which for
// anonymous fields is the type name), e.g. "TempTestFields.tempSectionGamma.GammaText";
// sub-field paths continue through the nested struct's fields. Each instruction
// becomes a FIELD_INSTRUCTION claim (one per language) with the language as an
// IN_LANGUAGE sub-claim. A path not matching any field is an error.
func Fields[T any](
	mnemonics map[string][]string,
	sections map[string]map[string]string,
	instructions map[string]map[string]string,
) (*internalCore.Fields, errors.E) {
	if mnemonics == nil {
		return nil, nil //nolint:nilnil
	}

	v := reflect.ValueOf(new(T)).Elem() //nolint:varnamelen
	t := v.Type()

	// Handle pointer to struct.
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		errE := errors.New("expected struct")
		errors.Details(errE)["got"] = t.Kind().String()
		return nil, errE
	}

	fc := fieldsCollector{
		mnemonics:        mnemonics,
		sectionNames:     sections,
		sections:         make(map[string]*sectionData),
		sectionOrder:     1.0,
		instructions:     instructions,
		usedInstructions: make(map[string]bool),
	}

	errE := fc.processLevel(t, "", []string{}, []reflect.Type{t})
	if errE != nil {
		return nil, errE
	}

	// A provided instruction not consumed by any field means its path does not
	// match a field (a typo or a stale path after a rename), so it is an error.
	for _, path := range slices.Sorted(maps.Keys(instructions)) {
		if !fc.usedInstructions[path] {
			errE := errors.New("instruction field path not found")
			errors.Details(errE)["field"] = path
			return nil, errE
		}
	}

	return fc.finalizeSections()
}

// sectionData tracks state for a section being built.
type sectionData struct {
	order        internalCore.Amount[float64]
	orderDefined bool
	fieldOrder   float64
	fields       []internalCore.Field
}

// fieldsCollector holds state for the Fields function.
type fieldsCollector struct {
	mnemonics        map[string][]string
	sectionNames     map[string]map[string]string
	sections         map[string]*sectionData
	sectionOrder     float64
	instructions     map[string]map[string]string
	usedInstructions map[string]bool
}

// getOrCreateSection returns the sectionData for the given ID, creating it if needed.
func (fc *fieldsCollector) getOrCreateSection(id string) *sectionData {
	sd, ok := fc.sections[id]
	if !ok {
		sd = &sectionData{
			order:        internalCore.Amount[float64]{},
			orderDefined: false,
			fieldOrder:   1.0,
			fields:       nil,
		}
		fc.sections[id] = sd
	}
	return sd
}

// finalizeSections validates that all named sections have defined orders and provided names,
// and builds the result. Fields collected under the empty section ID are returned as top-level
// fields.
func (fc *fieldsCollector) finalizeSections() (*internalCore.Fields, errors.E) {
	if len(fc.sections) == 0 {
		return nil, nil //nolint:nilnil
	}

	var topFields []internalCore.Field
	sections := make([]internalCore.Section, 0, len(fc.sections))

	// Iterate section IDs in sorted order so errors are deterministic.
	for _, id := range slices.Sorted(maps.Keys(fc.sections)) {
		sd := fc.sections[id]
		if id == "" {
			topFields = sd.fields
			continue
		}
		if !sd.orderDefined {
			errE := errors.New("section order not defined")
			errors.Details(errE)["section"] = id
			return nil, errE
		}
		name, errE := fc.sectionName(id)
		if errE != nil {
			return nil, errE
		}
		sections = append(sections, internalCore.Section{
			ID:          internalCore.Identifier(id),
			Name:        name,
			OrderInList: sd.order,
			Field:       sd.fields,
		})
	}

	if len(sections) == 0 && len(topFields) == 0 {
		return nil, nil //nolint:nilnil
	}

	// Sort sections by OrderInList for deterministic output, with ID as tiebreaker.
	slices.SortFunc(sections, func(a, b internalCore.Section) int {
		if c := cmp.Compare(a.OrderInList.Amount, b.OrderInList.Amount); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	return &internalCore.Fields{
		Section: sections,
		Field:   topFields,
	}, nil
}

// sectionName builds the section's NAME values from the translated names provided to Fields:
// one StringWithLanguage per language (sorted by language for deterministic output), each with
// the language as an IN_LANGUAGE reference. Errors if no names are provided for the section.
func (fc *fieldsCollector) sectionName(id string) ([]internalCore.StringWithLanguage, errors.E) {
	translations := fc.sectionNames[id]
	if len(translations) == 0 {
		errE := errors.New("section names not provided")
		errors.Details(errE)["section"] = id
		return nil, errE
	}

	name := make([]internalCore.StringWithLanguage, 0, len(translations))
	for _, language := range slices.Sorted(maps.Keys(translations)) {
		name = append(name, internalCore.StringWithLanguage{
			Value: translations[language],
			InLanguage: []internalCore.Ref{{
				ID: []string{internalCore.Namespace, "LANGUAGE", language},
			}},
		})
	}
	return name, nil
}

// processLevel processes struct fields, accumulating them into fc.sections.
// The defaultSection is the section ID that fields without a section tag inherit;
// it is empty at the top level.
//
// Embedded structs with section tags define sections (at top level) or produce an
// error (inside sections, since nesting is not allowed). Embedded structs without
// section tags are flattened into the current level. Fields without a section tag
// use defaultSection as their section ID.
func (fc *fieldsCollector) processLevel(
	structType reflect.Type,
	defaultSection string,
	fieldPath []string,
	structPath []reflect.Type,
) errors.E {
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldType := field.Type

		newFieldPath := append(slices.Clone(fieldPath), field.Name)

		// Skip documentid fields.
		if _, ok := field.Tag.Lookup("documentid"); ok {
			continue
		}

		// Skip value fields.
		if _, ok := field.Tag.Lookup("value"); ok {
			if _, hasInverse := field.Tag.Lookup("inverseProperty"); hasInverse {
				errE := errors.New("inverseProperty tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}
			if _, hasEmbed := field.Tag.Lookup("embed"); hasEmbed {
				errE := errors.New("embed tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}
			if strings.TrimSpace(field.Tag.Get("section")) != "" {
				errE := errors.New("section tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}
			if field.Tag.Get("order") != "" {
				errE := errors.New("order tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}
			if field.Tag.Get("context") != "" {
				errE := errors.New("context tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}
			continue
		}

		propertyMnemonic := field.Tag.Get("property")
		if propertyMnemonic == "-" {
			continue
		}

		orderTag := field.Tag.Get("order")
		if orderTag == "-" {
			continue
		}

		// Handle embedded structs.
		if field.Anonymous && fieldType.Kind() == reflect.Struct {
			sectionID := strings.TrimSpace(field.Tag.Get("section"))
			if sectionID != "" {
				// Sections can only be defined at the top level.
				if defaultSection != "" {
					errE := errors.New("sections cannot be nested inside sections")
					errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
					return errE
				}

				// Embedded struct defines a section.
				sectionOrder, errE := resolveOrder(orderTag, &fc.sectionOrder)
				if errE != nil {
					errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
					return errE
				}
				sd := fc.getOrCreateSection(sectionID)
				if sd.orderDefined {
					errE := errors.New("section defined more than once")
					errors.Details(errE)["section"] = sectionID
					errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
					return errE
				}
				sd.order = sectionOrder
				sd.orderDefined = true

				// Recurse with this section as default.
				errE = fc.processLevel(fieldType, sectionID, newFieldPath, structPath)
				if errE != nil {
					return errE
				}
				continue
			}

			// Plain embedded struct, recurse at current level.
			errE := fc.processLevel(fieldType, defaultSection, newFieldPath, structPath)
			if errE != nil {
				return errE
			}
			continue
		}

		if propertyMnemonic == "" {
			continue
		}

		// Determine effective section: field's own section tag overrides default.
		effectiveSection := strings.TrimSpace(field.Tag.Get("section"))
		if effectiveSection == "" {
			effectiveSection = defaultSection
		}

		sd := fc.getOrCreateSection(effectiveSection)
		fieldOrder, errE := resolveOrder(orderTag, &sd.fieldOrder)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return errE
		}
		f, errE := fc.makeField(field, propertyMnemonic, fieldOrder, newFieldPath, structPath)
		if errE != nil {
			return errE
		}
		sd.fields = append(sd.fields, f)
	}

	return nil
}

// processSubFields processes struct fields for sub-field extraction.
// Section tags are not allowed on sub-fields or their embedded structs.
func (fc *fieldsCollector) processSubFields(
	structType reflect.Type,
	order *float64,
	fieldPath []string,
	structPath []reflect.Type,
) ([]internalCore.Field, errors.E) {
	fields := make([]internalCore.Field, 0, structType.NumField())

	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldType := field.Type

		newFieldPath := append(slices.Clone(fieldPath), field.Name)

		// Skip documentid fields.
		if _, ok := field.Tag.Lookup("documentid"); ok {
			continue
		}

		// Skip value fields.
		if _, ok := field.Tag.Lookup("value"); ok {
			if _, hasInverse := field.Tag.Lookup("inverseProperty"); hasInverse {
				errE := errors.New("inverseProperty tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}
			if _, hasEmbed := field.Tag.Lookup("embed"); hasEmbed {
				errE := errors.New("embed tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}
			if strings.TrimSpace(field.Tag.Get("section")) != "" {
				errE := errors.New("section tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}
			if field.Tag.Get("order") != "" {
				errE := errors.New("order tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}
			if field.Tag.Get("context") != "" {
				errE := errors.New("context tag cannot be used with value tag")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}
			continue
		}

		propertyMnemonic := field.Tag.Get("property")
		if propertyMnemonic == "-" {
			continue
		}

		orderTag := field.Tag.Get("order")
		if orderTag == "-" {
			continue
		}

		// Handle embedded structs.
		if field.Anonymous && fieldType.Kind() == reflect.Struct {
			if strings.TrimSpace(field.Tag.Get("section")) != "" {
				errE := errors.New("sub-fields cannot have sections")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			// Plain embedded struct, recurse.
			innerFields, errE := fc.processSubFields(fieldType, order, newFieldPath, structPath)
			if errE != nil {
				return nil, errE
			}
			fields = append(fields, innerFields...)
			continue
		}

		if propertyMnemonic == "" {
			continue
		}

		// Sub-fields cannot have sections.
		if strings.TrimSpace(field.Tag.Get("section")) != "" {
			errE := errors.New("sub-fields cannot have sections")
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return nil, errE
		}

		fieldOrder, errE := resolveOrder(orderTag, order)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
			return nil, errE
		}
		f, errE := fc.makeField(field, propertyMnemonic, fieldOrder, newFieldPath, structPath)
		if errE != nil {
			return nil, errE
		}
		fields = append(fields, f)
	}

	return fields, nil
}

// resolveOrder returns the order for a field or section as an Amount[float64].
//
// If orderTag is non-empty, it is parsed using document.NewAmountDetectPrecision.
// If orderTag is empty, the current auto-increment order is used (with precision 1) and advanced.
func resolveOrder(orderTag string, order *float64) (internalCore.Amount[float64], errors.E) {
	if orderTag == "" {
		v := *order
		*order++
		return internalCore.Amount[float64]{Amount: v, Precision: 1}, nil
	}

	amount, precision, errE := document.NewAmountDetectPrecision(orderTag)
	if errE != nil {
		errors.Details(errE)["order"] = orderTag
		return internalCore.Amount[float64]{}, errE
	}

	v, err := strconv.ParseFloat(string(amount), 64)
	if err != nil {
		// This should not be possible.
		errE := errors.WithStack(err)
		errors.Details(errE)["order"] = orderTag
		errors.Details(errE)["amount"] = amount
		panic(errE)
	}

	return internalCore.Amount[float64]{Amount: v, Precision: precision}, nil
}

// makeField creates a core.Field from a struct field's tags.
func (fc *fieldsCollector) makeField(
	structField reflect.StructField,
	mnemonic string,
	order internalCore.Amount[float64],
	fieldPath []string,
	structPath []reflect.Type,
) (internalCore.Field, errors.E) {
	// Check mnemonic exists.
	propertyBase, ok := fc.mnemonics[mnemonic]
	if !ok {
		errE := errors.New("mnemonic not found")
		errors.Details(errE)["name"] = mnemonic
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}

	// Create property ref using the base ID.
	propertyRef := internalCore.Ref{ID: propertyBase}

	// Determine value type.
	valueTypeRef, errE := determineValueType(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}

	// Parse cardinality.
	cardinality, errE := parseFieldCardinality(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}

	// Parse values tag. For simple fields, values is on the property field.
	// For struct fields, values is on the value:"" field inside.
	values, errE := parseValuesTag(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}
	if values == nil {
		values, errE = parseStructValueFieldValues(structField.Type)
		if errE != nil {
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			return internalCore.Field{}, errE
		}
	}

	// Parse inverseProperty tag.
	var inverseProperty *internalCore.Ref
	if inversePropertyMnemonic, ok := structField.Tag.Lookup("inverseProperty"); ok {
		inverseBase, ok := fc.mnemonics[inversePropertyMnemonic]
		if !ok {
			errE := errors.New("inverse property mnemonic not found")
			errors.Details(errE)["name"] = inversePropertyMnemonic
			errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
			return internalCore.Field{}, errE
		}
		inverseProperty = &internalCore.Ref{ID: inverseBase}
	}

	// Parse embed tag.
	embed, errE := parseEmbedTag(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}

	// Collect sub-fields from struct types.
	subFields, errE := fc.collectSubFields(structField.Type, fieldPath, structPath)
	if errE != nil {
		return internalCore.Field{}, errE
	}

	// Parse default tag. For simple fields it is on the property field; for struct fields it is
	// on the value:"" field inside. It records that the value may be a none-value/unknown-value
	// claim.
	defaultRef, errE := parseFieldDefault(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
	}

	return internalCore.Field{
		Property:        propertyRef,
		ValueType:       valueTypeRef,
		OrderInList:     order,
		Cardinality:     cardinality,
		Values:          values,
		SubField:        subFields,
		InverseProperty: inverseProperty,
		Embed:           embed,
		Context:         parseContextTag(structField),
		Default:         defaultRef,
		Instruction:     fc.fieldInstruction(fieldPath),
	}, nil
}

// fieldInstruction builds the field's instruction values from the translated instructions
// provided to Fields, looked up by the field's dot-joined path: one RawHTMLWithLanguage per
// language (sorted by language for deterministic output), each with the language as an
// IN_LANGUAGE reference. Returns nil when no instructions are provided for the field.
func (fc *fieldsCollector) fieldInstruction(fieldPath []string) []internalCore.RawHTMLWithLanguage {
	path := strings.Join(fieldPath, ".")
	translations := fc.instructions[path]
	if len(translations) == 0 {
		return nil
	}
	fc.usedInstructions[path] = true

	instruction := make([]internalCore.RawHTMLWithLanguage, 0, len(translations))
	for _, language := range slices.Sorted(maps.Keys(translations)) {
		instruction = append(instruction, internalCore.RawHTMLWithLanguage{
			Value: internalCore.RawHTML(translations[language]),
			InLanguage: []internalCore.Ref{{
				ID: []string{internalCore.Namespace, "LANGUAGE", language},
			}},
		})
	}
	return instruction
}

// collectSubFields extracts sub-field descriptions from a struct type.
// Sub-fields are non-value fields within a nested struct that has a property tag.
// Returns nil if the type is not a struct or has no sub-fields.
func (fc *fieldsCollector) collectSubFields(fieldType reflect.Type, fieldPath []string, structPath []reflect.Type) ([]internalCore.Field, errors.E) {
	fieldType = internalCore.UnwrapSliceAndPointer(fieldType)

	if fieldType.Kind() != reflect.Struct {
		return nil, nil
	}

	// Skip known core types that are not user-defined structs with sub-fields.
	if internalCore.IsKnownType(fieldType) {
		return nil, nil
	}

	// Detect recursion: if this type is already on the struct path, return an error.
	if slices.Contains(structPath, fieldType) {
		errE := errors.New("recursive struct type detected")
		errors.Details(errE)["type"] = fieldType.String()
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return nil, errE
	}

	subOrder := 1.0
	subFields, errE := fc.processSubFields(fieldType, &subOrder, fieldPath, append(slices.Clone(structPath), fieldType))
	if errE != nil {
		return nil, errE
	}

	if len(subFields) == 0 {
		return nil, nil
	}

	return subFields, nil
}

// valueTypeRef creates a core.Ref pointing to a VALUE_TYPE vocabulary entry.
func valueTypeRef(code string) internalCore.Ref {
	return internalCore.Ref{ID: []string{internalCore.Namespace, "VALUE_TYPE", code}}
}

// determineValueType determines the value type ref for a struct field based on its Go type and type tag.
func determineValueType(field reflect.StructField) (internalCore.Ref, errors.E) {
	fieldType := field.Type
	typeTag := field.Tag.Get("type")

	return determineValueTypeFromReflect(fieldType, typeTag)
}

// determineValueTypeFromReflect determines the value type from a reflect.Type and optional type tag.
//
//nolint:cyclop
func determineValueTypeFromReflect(fieldType reflect.Type, typeTag string) (internalCore.Ref, errors.E) {
	fieldType = internalCore.UnwrapSliceAndPointer(fieldType)

	// Check core types first.
	switch {
	case fieldType == internalCore.RefType:
		return valueTypeRef("REFERENCE"), nil
	case fieldType == internalCore.TimeType:
		return valueTypeRef("TIME"), nil
	case fieldType == internalCore.StdTimeType:
		return valueTypeRef("TIME"), nil
	case fieldType == internalCore.TimeIntervalType:
		return valueTypeRef("TIME_INTERVAL"), nil
	case fieldType == internalCore.IdentifierType:
		return valueTypeRef("IDENTIFIER"), nil
	case fieldType == internalCore.LinkType:
		return valueTypeRef("LINK"), nil
	case fieldType == internalCore.FileType:
		return valueTypeRef("FILE"), nil
	case fieldType == internalCore.HTMLType:
		return valueTypeRef("HTML"), nil
	case fieldType == internalCore.RawHTMLType:
		return valueTypeRef("HTML"), nil
	case fieldType == internalCore.NoneType:
		return valueTypeRef("NONE"), nil
	case fieldType == internalCore.UnknownType:
		return valueTypeRef("UNKNOWN"), nil
	case internalCore.AmountTypes[fieldType]:
		return valueTypeRef("AMOUNT"), nil
	case internalCore.AmountIntervalTypes[fieldType]:
		return valueTypeRef("AMOUNT_INTERVAL"), nil
	}

	// Check primitive types.
	switch fieldType.Kind() { //nolint:exhaustive
	case reflect.String:
		switch typeTag {
		case typeID:
			return valueTypeRef("IDENTIFIER"), nil
		case typeHTML, typeRawHTML:
			return valueTypeRef("HTML"), nil
		case typeLink:
			return valueTypeRef("LINK"), nil
		case typeFile:
			return valueTypeRef("FILE"), nil
		case "":
			return valueTypeRef("STRING"), nil
		default:
			return internalCore.Ref{}, errors.New("string field used with unsupported type tag")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return valueTypeRef("AMOUNT"), nil
	case reflect.Bool:
		switch typeTag {
		case typeNone:
			return valueTypeRef("NONE"), nil
		case typeUnknown:
			return valueTypeRef("UNKNOWN"), nil
		case "":
			return valueTypeRef("HAS"), nil
		default:
			return internalCore.Ref{}, errors.New("bool field used with unsupported type tag")
		}
	case reflect.Struct:
		// Look for value field in struct to determine type.
		// If no value field exists, the struct maps to a HAS claim in Documents.
		ref, errE := determineStructValueType(fieldType)
		if errors.Is(errE, errValueClaimNotFound) {
			return valueTypeRef("HAS"), nil
		}
		return ref, errE
	}

	errE := errors.New("field has unsupported or unexpected value type")
	errors.Details(errE)["type"] = fieldType.String()
	return internalCore.Ref{}, errE
}

// determineStructValueType determines the value type for a struct by looking at its value field.
//
// Returns errValueClaimNotFound if no value field is found.
func determineStructValueType(structType reflect.Type) (internalCore.Ref, errors.E) {
	for i := range structType.NumField() {
		field := structType.Field(i)
		if _, ok := field.Tag.Lookup("value"); ok {
			fieldType := field.Type
			if fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			return determineValueTypeFromReflect(fieldType, field.Tag.Get("type"))
		}

		// Recurse into embedded structs.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			ref, errE := determineStructValueType(field.Type)
			if !errors.Is(errE, errValueClaimNotFound) {
				return ref, errE
			}
		}
	}

	return internalCore.Ref{}, errors.WithStack(errValueClaimNotFound)
}

// parseFieldCardinality parses the cardinality tag and returns a core.Interval[core.Amount[int]].
//
// Unlike parseCardinality used by Documents, this does not enforce Go-type constraints
// (e.g., single values can have unbounded max in field descriptions).
func parseFieldCardinality(field reflect.StructField) (internalCore.Interval[internalCore.Amount[int]], errors.E) {
	cardTag := field.Tag.Get("cardinality")

	minCard, maxCard, errE := parseFieldsCardinality(cardTag, field.Type)
	if errE != nil {
		return internalCore.Interval[internalCore.Amount[int]]{}, errE
	}

	result := internalCore.Interval[internalCore.Amount[int]]{}

	fromAmount := internalCore.Amount[int]{Amount: minCard, Precision: 1}
	result.From = &fromAmount

	if maxCard == -1 {
		// Unbounded upper bound is mapped to none.
		result.ToIsNone = true
	} else {
		// Cardinality upper is inclusive; default ToIsOpen=false gives that.
		toAmount := internalCore.Amount[int]{Amount: maxCard, Precision: 1}
		result.To = &toAmount
	}

	return result, nil
}

// parseFieldsCardinality parses a cardinality tag string for field descriptions.
//
// It is similar to [parseCardinality], but applies default cardinality
// based on Go type without enforcing Go-type constraints on explicit values.
func parseFieldsCardinality(cardinality string, fieldType reflect.Type) (int, int, errors.E) {
	minCardinality, maxCardinality, errE := internalCore.ParseCardinalityTag(cardinality)
	if errE != nil {
		return 0, 0, errE
	}

	// Apply default cardinality based on Go type only when not explicitly specified.
	if cardinality == "" {
		baseType := fieldType
		if baseType.Kind() == reflect.Pointer {
			baseType = baseType.Elem()
		}
		isSlice := baseType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Slice
		if !isSlice {
			// Pointer and single value fields default to 0..1.
			maxCardinality = 1
		}
		// Slice fields default to 0..unbounded (already set).
	}

	return minCardinality, maxCardinality, nil
}

// parseValuesTag parses the "values" struct tag into a slice of search shortcut strings.
//
// Each entry is a search shortcut query string (the same grammar consumed by
// SearchShortcutGet) and entries are separated by ";". Identifier tokens within
// an entry may be either a 22-character base58 identifier or a comma-separated
// list of base parts (hashed via [identifier.From]).
//
// Each entry is validated by parsing but stored as-is; the frontend interprets it
// at render time.
//
// The values tag can only be used with core.Ref field type.
func parseValuesTag(field reflect.StructField) ([]string, errors.E) {
	tag := field.Tag.Get("values")
	if tag == "" {
		return nil, nil
	}

	// Validate that the field type is internalCore.Ref (or slice/pointer of internalCore.Ref).
	baseType := internalCore.UnwrapSliceAndPointer(field.Type)
	if baseType != internalCore.RefType {
		errE := errors.New("values tag can only be used with core.Ref field type")
		errors.Details(errE)["type"] = field.Type.String()
		return nil, errE
	}

	entries := strings.Split(tag, ";")
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		errE := validateShortcut(entry)
		if errE != nil {
			errors.Details(errE)["entry"] = entry
			return nil, errE
		}
		values = append(values, entry)
	}

	return values, nil
}

// parseContextTag parses the "context" struct tag into a slice of opaque context
// identifiers, separated by ",". Transform does not interpret them; consumers
// decide what they mean.
func parseContextTag(field reflect.StructField) []string {
	tag := field.Tag.Get("context")
	if tag == "" {
		return nil
	}
	entries := strings.Split(tag, ",")
	contexts := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		contexts = append(contexts, entry)
	}
	return contexts
}

// parseStructValueFieldValues looks inside a struct type for a value:"" field
// and parses its values tag. Returns nil if the type is not a struct or has
// no value field with a values tag.
func parseStructValueFieldValues(fieldType reflect.Type) ([]string, errors.E) {
	fieldType = internalCore.UnwrapSliceAndPointer(fieldType)

	if fieldType.Kind() != reflect.Struct {
		return nil, nil
	}

	for i := range fieldType.NumField() {
		field := fieldType.Field(i)
		if _, ok := field.Tag.Lookup("value"); ok {
			return parseValuesTag(field)
		}

		// Recurse into embedded structs.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			values, errE := parseStructValueFieldValues(field.Type)
			if values != nil || errE != nil {
				return values, errE
			}
		}
	}

	return nil, nil
}

// parseFieldDefault returns the value type ref for a field's "default" tag, or nil if there is
// no default. The default tag is on the property field for simple fields, or on the value:""
// field inside for struct fields (the same place the "values" tag is read from).
func parseFieldDefault(field reflect.StructField) (*internalCore.Ref, errors.E) {
	if tag, ok := field.Tag.Lookup("default"); ok {
		return defaultValueTypeRef(tag)
	}
	return parseStructValueFieldDefault(field.Type)
}

// defaultValueTypeRef maps a "default" tag value ("none" or "unknown") to the corresponding
// VALUE_TYPE ref. An empty tag yields no default; any other value is an error.
func defaultValueTypeRef(tag string) (*internalCore.Ref, errors.E) {
	switch tag {
	case "":
		return nil, nil //nolint:nilnil
	case defaultNone:
		ref := valueTypeRef("NONE")
		return &ref, nil
	case defaultUnknown:
		ref := valueTypeRef("UNKNOWN")
		return &ref, nil
	default:
		errE := errors.New("default tag must be \"none\" or \"unknown\"")
		errors.Details(errE)["default"] = tag
		return nil, errE
	}
}

// parseStructValueFieldDefault looks inside a struct type for a value:"" field and returns the
// value type ref for its "default" tag. Returns nil if the type is not a struct or has no value
// field with a default.
func parseStructValueFieldDefault(fieldType reflect.Type) (*internalCore.Ref, errors.E) {
	fieldType = internalCore.UnwrapSliceAndPointer(fieldType)

	if fieldType.Kind() != reflect.Struct {
		return nil, nil //nolint:nilnil
	}

	for i := range fieldType.NumField() {
		field := fieldType.Field(i)
		if _, ok := field.Tag.Lookup("value"); ok {
			return defaultValueTypeRef(field.Tag.Get("default"))
		}

		// Recurse into embedded structs.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			ref, errE := parseStructValueFieldDefault(field.Type)
			if ref != nil || errE != nil {
				return ref, errE
			}
		}
	}

	return nil, nil //nolint:nilnil
}
