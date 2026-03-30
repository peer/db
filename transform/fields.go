package transform

import (
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/core"
)

// Fields extracts field descriptions from a struct type using struct tags.
//
// It reads the same struct tags as [Documents] (property, cardinality, type, value)
// plus additional tags (section, "section@XX", values) to produce a [core.Fields]
// describing the struct's field schema.
//
// The languageCodes parameter maps language code suffixes (used in "section@XX" struct tags,
// e.g., "section@en-GB") to language document base IDs. The mnemonics parameter maps
// property mnemonic names to property document base IDs.
func Fields[T any](
	languageCodes map[string][]string,
	mnemonics map[string][]string,
) (core.Fields, errors.E) {
	v := reflect.ValueOf(new(T)).Elem() //nolint:varnamelen
	t := v.Type()

	// Handle pointer to struct.
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		errE := errors.New("expected struct")
		errors.Details(errE)["got"] = t.Kind().String()
		return core.Fields{}, errE
	}

	fc := fieldsCollector{
		languageCodes: languageCodes,
		mnemonics:     mnemonics,
	}

	order := 1.0
	sections, fields, errE := fc.processLevel(t, &order, []string{})
	if errE != nil {
		return core.Fields{}, errE
	}

	return core.Fields{
		Section: sections,
		Field:   fields,
	}, nil
}

// fieldsCollector holds state for the Fields function.
type fieldsCollector struct {
	languageCodes map[string][]string
	mnemonics     map[string][]string
}

// processLevel processes struct fields at the current level, producing sections and fields.
// Sections come from embedded structs with section tags; fields come from regular fields with property tags.
// Embedded structs without section tags are recursed into, flattening their fields at this level.
func (fc *fieldsCollector) processLevel(
	structType reflect.Type,
	order *float64,
	fieldPath []string,
) ([]core.Section, []core.Field, errors.E) {
	var sections []core.Section
	fields := make([]core.Field, 0, structType.NumField())

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

		// Handle embedded structs.
		if field.Anonymous && fieldType.Kind() == reflect.Struct {
			sectionNames := fc.extractSectionNames(field.Tag)
			if len(sectionNames) > 0 {
				// This is a section.
				section := core.Section{
					Name:        sectionNames,
					OrderInList: *order,
					Field:       nil,
				}
				*order++

				// Process inner fields (no nested sections allowed).
				innerOrder := 1.0
				innerFields, errE := fc.processInnerFields(fieldType, &innerOrder, newFieldPath)
				if errE != nil {
					return nil, nil, errE
				}
				section.Field = innerFields

				sections = append(sections, section)
				continue
			}

			// Plain embedded struct without section tag, recurse at current level.
			innerSections, innerFields, errE := fc.processLevel(fieldType, order, newFieldPath)
			if errE != nil {
				return nil, nil, errE
			}
			sections = append(sections, innerSections...)
			fields = append(fields, innerFields...)
			continue
		}

		if propertyMnemonic == "" {
			continue
		}

		f, errE := fc.makeField(field, propertyMnemonic, *order, newFieldPath)
		if errE != nil {
			return nil, nil, errE
		}
		fields = append(fields, f)
		*order++
	}

	return sections, fields, nil
}

// processInnerFields processes struct fields inside a section.
// Sections cannot be nested, so embedded structs with section tags produce an error.
func (fc *fieldsCollector) processInnerFields(
	structType reflect.Type,
	order *float64,
	fieldPath []string,
) ([]core.Field, errors.E) {
	fields := make([]core.Field, 0, structType.NumField())

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

		// Handle embedded structs.
		if field.Anonymous && fieldType.Kind() == reflect.Struct {
			sectionNames := fc.extractSectionNames(field.Tag)
			if len(sectionNames) > 0 {
				errE := errors.New("sections cannot be nested inside sections")
				errors.Details(errE)["field"] = strings.Join(newFieldPath, ".")
				return nil, errE
			}

			// Plain embedded struct, recurse.
			innerFields, errE := fc.processInnerFields(fieldType, order, newFieldPath)
			if errE != nil {
				return nil, errE
			}
			fields = append(fields, innerFields...)
			continue
		}

		if propertyMnemonic == "" {
			continue
		}

		f, errE := fc.makeField(field, propertyMnemonic, *order, newFieldPath)
		if errE != nil {
			return nil, errE
		}
		fields = append(fields, f)
		*order++
	}

	return fields, nil
}

// makeField creates a core.Field from a struct field's tags.
func (fc *fieldsCollector) makeField(
	structField reflect.StructField,
	mnemonic string,
	order float64,
	fieldPath []string,
) (core.Field, errors.E) {
	// Check mnemonic exists.
	propertyBase, ok := fc.mnemonics[mnemonic]
	if !ok {
		errE := errors.New("mnemonic not found")
		errors.Details(errE)["name"] = mnemonic
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return core.Field{}, errE
	}

	// Create property ref using the base ID.
	propertyRef := core.Ref{ID: propertyBase}

	// Determine value type.
	valueTypeRef, errE := determineValueType(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return core.Field{}, errE
	}

	// Parse cardinality.
	cardinality, errE := parseFieldCardinality(structField)
	if errE != nil {
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		return core.Field{}, errE
	}

	// Parse values tag.
	values, errE := parseValuesTag(structField, fieldPath)
	if errE != nil {
		return core.Field{}, errE
	}

	return core.Field{
		Property:    propertyRef,
		ValueType:   valueTypeRef,
		OrderInList: order,
		Cardinality: cardinality,
		Values:      values,
	}, nil
}

// extractSectionNames extracts section names from struct tags.
// The bare "section" tag produces a name without InLanguage.
// Tags like "section@en-GB" produce names with InLanguage set to the corresponding language.
func (fc *fieldsCollector) extractSectionNames(
	tag reflect.StructTag,
) []core.StringWithLanguage {
	var names []core.StringWithLanguage

	// Check for bare "section" tag.
	if name, ok := tag.Lookup("section"); ok {
		names = append(names, core.StringWithLanguage{
			Value:      name,
			InLanguage: nil,
		})
	}

	// Check for section@XX tags, iterating language codes in sorted order for determinism.
	codes := make([]string, 0, len(fc.languageCodes))
	for code := range fc.languageCodes {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		langBase := fc.languageCodes[code]
		key := "section@" + code
		if name, ok := tag.Lookup(key); ok {
			names = append(names, core.StringWithLanguage{
				Value: name,
				InLanguage: []core.Ref{{
					ID: langBase,
				}},
			})
		}
	}

	return names
}

// valueTypeRef creates a core.Ref pointing to a VALUE_TYPE vocabulary entry.
func valueTypeRef(code string) core.Ref {
	return core.Ref{ID: []string{core.Namespace, "VALUE_TYPE", code}}
}

// determineValueType determines the value type ref for a struct field based on its Go type and type tag.
func determineValueType(field reflect.StructField) (core.Ref, errors.E) {
	fieldType := field.Type
	typeTag := field.Tag.Get("type")

	return determineValueTypeFromReflect(fieldType, typeTag)
}

// determineValueTypeFromReflect determines the value type from a reflect.Type and optional type tag.
//
//nolint:cyclop
func determineValueTypeFromReflect(fieldType reflect.Type, typeTag string) (core.Ref, errors.E) {
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
			return core.Ref{}, errors.New("string field used with unsupported type tag")
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
			return core.Ref{}, errors.New("bool field used with unsupported type tag")
		}
	case reflect.Struct:
		// Look for value field in struct to determine type.
		return determineStructValueType(fieldType)
	}

	errE := errors.New("field has unsupported or unexpected value type")
	errors.Details(errE)["type"] = fieldType.String()
	return core.Ref{}, errE
}

// determineStructValueType determines the value type for a struct by looking at its value field.
func determineStructValueType(structType reflect.Type) (core.Ref, errors.E) {
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
			if errE == nil {
				return ref, nil
			}
		}
	}

	errE := errors.New("struct has no value field")
	errors.Details(errE)["type"] = structType.String()
	return core.Ref{}, errE
}

// parseFieldCardinality parses the cardinality tag and returns a core.Interval[core.Amount[int]].
//
// Unlike parseCardinality used by Documents, this does not enforce Go-type constraints
// (e.g., single values can have unbounded max in field descriptions).
func parseFieldCardinality(field reflect.StructField) (core.Interval[core.Amount[int]], errors.E) {
	cardTag := field.Tag.Get("cardinality")

	minCard, maxCard, errE := parseFieldsCardinality(cardTag, field.Type)
	if errE != nil {
		return core.Interval[core.Amount[int]]{}, errE
	}

	result := core.Interval[core.Amount[int]]{}

	fromAmount := core.Amount[int]{Amount: minCard, Precision: 1}
	result.From = &fromAmount

	if maxCard == -1 {
		// Unbounded upper bound is mapped to none.
		result.ToIsNone = true
	} else {
		toAmount := core.Amount[int]{Amount: maxCard, Precision: 1}
		result.To = &toAmount
		result.ToIsClosed = true
	}

	return result, nil
}

// parseFieldsCardinality parses a cardinality tag string for field descriptions.
//
// It parses the same format as parseCardinality but does not enforce Go-type constraints,
// since field descriptions describe intended cardinality regardless of the Go type used.
func parseFieldsCardinality(cardinality string, fieldType reflect.Type) (int, int, errors.E) {
	minCardinality := 0
	maxCardinality := -1

	if cardinality != "" { //nolint:nestif
		if strings.Contains(cardinality, "..") {
			parts := strings.SplitN(cardinality, "..", 2) //nolint:mnd
			minStr := strings.TrimSpace(parts[0])
			if minStr == "" {
				errE := errors.New("cardinality min value is empty")
				errors.Details(errE)["cardinality"] = cardinality
				return 0, 0, errE
			}
			var err error
			minCardinality, err = strconv.Atoi(minStr)
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
		} else {
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
			minCardinality = val
			maxCardinality = val
		}
	} else {
		// Default cardinality based on Go type.
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
func parseValuesTag(field reflect.StructField, fieldPath []string) ([]core.Ref, errors.E) {
	tag := field.Tag.Get("values")
	if tag == "" {
		return nil, nil
	}

	// Validate that the field type is core.Ref (or slice/pointer of core.Ref).
	baseType := field.Type
	if baseType.Kind() == reflect.Slice {
		baseType = baseType.Elem()
	}
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}
	if baseType != coreRef {
		errE := errors.New("values tag can only be used with core.Ref field type")
		errors.Details(errE)["field"] = strings.Join(fieldPath, ".")
		errors.Details(errE)["type"] = field.Type.String()
		return nil, errE
	}

	entries := strings.Split(tag, ";")
	refs := make([]core.Ref, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		refs = append(refs, core.Ref{
			ID: parts,
		})
	}

	return refs, nil
}
