package transform

import (
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
// The mnemonics parameter maps property mnemonic names to property document base IDs.
func Fields[T any](
	mnemonics map[string][]string,
) (*internalCore.Fields, errors.E) {
	v := reflect.ValueOf(new(T)).Elem() //nolint:varnamelen
	t := v.Type()

	// Handle pointer to struct.
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		errE := errors.New("expected struct")
		errors.Details(errE)["got"] = t.Kind().String()
		return nil, errE
	}

	fc := fieldsCollector{
		mnemonics: mnemonics,
		sections:  make(map[string]*sectionData),
	}

	order := 1.0
	errE := fc.processLevel(t, &order, []string{}, []reflect.Type{t})
	if errE != nil {
		return nil, errE
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
	mnemonics map[string][]string
	sections  map[string]*sectionData
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

// finalizeSections validates that all named sections have defined orders and builds the result.
// Fields collected under the empty section ID are returned as top-level fields.
func (fc *fieldsCollector) finalizeSections() (*internalCore.Fields, errors.E) {
	if len(fc.sections) == 0 {
		return nil, nil //nolint:nilnil
	}

	var topFields []internalCore.Field
	sections := make([]internalCore.Section, 0, len(fc.sections))

	for id, sd := range fc.sections {
		if id == "" {
			topFields = sd.fields
			continue
		}
		if !sd.orderDefined {
			errE := errors.New("section order not defined")
			errors.Details(errE)["section"] = id
			return nil, errE
		}
		sections = append(sections, internalCore.Section{
			ID:          internalCore.Identifier(id),
			OrderInList: sd.order,
			Field:       sd.fields,
		})
	}

	if len(sections) == 0 && len(topFields) == 0 {
		return nil, nil //nolint:nilnil
	}

	return &internalCore.Fields{
		Section: sections,
		Field:   topFields,
	}, nil
}

// processLevel processes struct fields at the current level.
// Sections are accumulated in fc.sections. Embedded structs with section tags define sections
// and set the default section for their inner fields. Embedded structs without section tags
// are flattened into the current level. All fields are added to the corresponding section
// in fc.sections; fields without a section tag use the empty string as section ID.
func (fc *fieldsCollector) processLevel(
	structType reflect.Type,
	order *float64,
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
				// Embedded struct defines a section.
				sectionOrder, errE := resolveOrder(orderTag, order)
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

				// Process inner fields with this section as default.
				errE = fc.processFieldsWithDefaultSection(fieldType, sectionID, newFieldPath, structPath)
				if errE != nil {
					return errE
				}
				continue
			}

			// Plain embedded struct without section tag, recurse at current level.
			errE := fc.processLevel(fieldType, order, newFieldPath, structPath)
			if errE != nil {
				return errE
			}
			continue
		}

		if propertyMnemonic == "" {
			continue
		}

		// All fields go into their section (empty string for top-level).
		sectionID := strings.TrimSpace(field.Tag.Get("section"))
		sd := fc.getOrCreateSection(sectionID)
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

// processFieldsWithDefaultSection processes struct fields inside an embedded struct
// with a section tag. Fields default to the given section unless they override with
// their own section tag. Embedded structs with section tags produce an error
// (sections cannot be nested).
func (fc *fieldsCollector) processFieldsWithDefaultSection(
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
				errE := errors.New("sections cannot be nested inside sections")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return errE
			}

			// Plain embedded struct, recurse with same default section.
			errE := fc.processFieldsWithDefaultSection(fieldType, defaultSection, newFieldPath, structPath)
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

	// Parse values tag.
	values, errE := parseValuesTag(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return internalCore.Field{}, errE
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

	// Collect sub-fields from struct types.
	subFields, errE := fc.collectSubFields(structField.Type, fieldPath, structPath)
	if errE != nil {
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
	}, nil
}

// collectSubFields extracts sub-field descriptions from a struct type.
// Sub-fields are non-value fields within a nested struct that has a property tag.
// Returns nil if the type is not a struct or has no sub-fields.
func (fc *fieldsCollector) collectSubFields(fieldType reflect.Type, fieldPath []string, structPath []reflect.Type) ([]internalCore.Field, errors.E) {
	// Unwrap slice.
	if fieldType.Kind() == reflect.Slice {
		fieldType = fieldType.Elem()
	}

	// Unwrap pointer.
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	if fieldType.Kind() != reflect.Struct {
		return nil, nil
	}

	// Skip known core types that are not user-defined structs with sub-fields.
	if isKnownCoreType(fieldType) {
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

// isKnownCoreType returns true for core types that should not be
// inspected for sub-fields.
func isKnownCoreType(t reflect.Type) bool {
	return coreStructTypes[t] || coreAmountTypes[t] || coreAmountIntervalTypes[t]
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
	// Unwrap slice.
	if fieldType.Kind() == reflect.Slice {
		fieldType = fieldType.Elem()
	}

	// Unwrap pointer.
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// Check core types first.
	switch {
	case fieldType == coreRef:
		return valueTypeRef("REFERENCE"), nil
	case fieldType == coreTime:
		return valueTypeRef("TIME"), nil
	case fieldType == timeTime:
		return valueTypeRef("TIME"), nil
	case fieldType == coreTimeInterval:
		return valueTypeRef("TIME_INTERVAL"), nil
	case fieldType == coreIdentifier:
		return valueTypeRef("IDENTIFIER"), nil
	case fieldType == coreLink:
		return valueTypeRef("LINK"), nil
	case fieldType == coreHTML:
		return valueTypeRef("HTML"), nil
	case fieldType == coreRawHTML:
		return valueTypeRef("HTML"), nil
	case fieldType == coreNone:
		return valueTypeRef("NONE"), nil
	case fieldType == coreUnknown:
		return valueTypeRef("UNKNOWN"), nil
	case coreAmountTypes[fieldType]:
		return valueTypeRef("AMOUNT"), nil
	case coreAmountIntervalTypes[fieldType]:
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
			if fieldType.Kind() == reflect.Ptr {
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
		toAmount := internalCore.Amount[int]{Amount: maxCard, Precision: 1}
		result.To = &toAmount
		result.ToIsClosed = true
	}

	return result, nil
}

// parseFieldsCardinality parses a cardinality tag string for field descriptions.
//
// It is similar to [parseCardinality], but applies default cardinality
// based on Go type without enforcing Go-type constraints on explicit values.
func parseFieldsCardinality(cardinality string, fieldType reflect.Type) (int, int, errors.E) {
	minCardinality, maxCardinality, errE := parseCardinalityTag(cardinality)
	if errE != nil {
		return 0, 0, errE
	}

	// Apply default cardinality based on Go type only when not explicitly specified.
	if cardinality == "" {
		baseType := fieldType
		if baseType.Kind() == reflect.Ptr {
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

// parseValuesTag parses the "values" struct tag into a slice of core.Ref.
//
// The format is "namespace,mnemonic;namespace2,part1,part2".
//
// The values tag can only be used with core.Ref field type.
func parseValuesTag(field reflect.StructField) ([]internalCore.Ref, errors.E) {
	tag := field.Tag.Get("values")
	if tag == "" {
		return nil, nil
	}

	// Validate that the field type is internalCore.Ref (or slice/pointer of internalCore.Ref).
	baseType := field.Type
	if baseType.Kind() == reflect.Slice {
		baseType = baseType.Elem()
	}
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}
	if baseType != coreRef {
		errE := errors.New("values tag can only be used with core.Ref field type")
		errors.Details(errE)["type"] = field.Type.String()
		return nil, errE
	}

	entries := strings.Split(tag, ";")
	refs := make([]internalCore.Ref, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		refs = append(refs, internalCore.Ref{
			ID: parts,
		})
	}

	return refs, nil
}
