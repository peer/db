package export

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// diagramSkipIDs returns the set of class identifiers that should be omitted
// from the diagram. When skipCore is true the canonical core entities are
// excluded. Their absence from the idToName map also suppresses every edge
// that would have pointed to them.
func diagramSkipIDs(skipCore bool) map[identifier.Identifier]bool {
	if !skipCore {
		return nil
	}
	return map[identifier.Identifier]bool{
		internalCore.ClassClassID:      true,
		internalCore.PropertyClassID:   true,
		internalCore.VocabularyClassID: true,
		internalCore.LanguageClassID:   true,
		internalCore.UnitClassID:       true,
		internalCore.ValueTypeClassID:  true,
	}
}

// diagramFieldRow describes one row inside a Mermaid entity block.
type diagramFieldRow struct {
	valueType string
	name      string
	flags     []string
	comment   string
}

// diagramRelation describes one Mermaid ER diagram edge.
type diagramRelation struct {
	source    string
	target    string
	cardLeft  string
	cardRight string
	label     string
	// dashed selects ".." instead of "--" between the cardinality symbols,
	// used for non-identifying relationships (class hierarchy).
	dashed bool
}

// diagramEntity is one entity in the rendered diagram. typ is the Go struct
// from [core.ClassFieldsRegistry] that holds this class's own fields, or
// nil for classes with no own fields (every field is inherited).
type diagramEntity struct {
	typ     reflect.Type
	name    string
	id      identifier.Identifier
	parents []identifier.Identifier
}

// Diagram writes a Mermaid ER diagram describing every class registered with
// PeerDB together with its fields and reference relationships.
//
// Entities are sourced from [core.ClassDescriptionRegistry] (which yields
// the canonical mnemonic and any SUBCLASS_OF facts) and augmented with the Go
// struct types from [core.ClassFieldsRegistry] (used to walk each class's
// own fields). Reference fields produce solid edges; class hierarchy is
// rendered with dashed IS_SUBCLASS edges.
//
// When skipCore is true the core entities and INSTANCE_OF references are
// omitted.
//
// Anything the generator cannot resolve cleanly is logged at warn level to logger.
// The generator continues and produces a partial result.
func Diagram(logger zerolog.Logger, w io.Writer, skipCore bool) errors.E {
	skipIDs := diagramSkipIDs(skipCore)

	// DocumentFields (the ID and INSTANCE_OF) are shared by every document and
	// are not associated with any single class, so the diagram walks them into
	// every entity.
	documentFieldsType := reflect.TypeFor[core.DocumentFields]()

	// The diagram assumes every entity embeds DocumentFields and its own-fields
	// struct rather than reading them from the full class type, so verify those
	// assumptions hold for every registered class type and warn otherwise.
	validateDiagramTypes(logger, core.ClassRegistry, core.ClassFieldsRegistry, documentFieldsType)

	entities, idToName, errE := collectDiagramEntities(logger, skipIDs)
	if errE != nil {
		return errE
	}

	var buf strings.Builder
	buf.WriteString("---\nconfig:\n  layout: elk\n---\nerDiagram\n")

	for _, e := range entities {
		var rows []diagramFieldRow
		var relations []diagramRelation

		// Walk the shared DocumentFields into every entity ahead of its own fields.
		for _, t := range []reflect.Type{documentFieldsType, e.typ} {
			if t == nil {
				continue
			}
			var entityRows []diagramFieldRow
			var entityRelations []diagramRelation
			entityRows, entityRelations, errE = collectDiagramEntity(logger, t, e.name, idToName, skipIDs)
			if errE != nil {
				return errE
			}
			rows = append(rows, entityRows...)
			relations = append(relations, entityRelations...)
		}

		for _, parentID := range e.parents {
			parentName, ok := idToName[parentID]
			if !ok {
				continue
			}
			relations = append(relations, diagramRelation{
				source:    e.name,
				target:    parentName,
				cardLeft:  "}o",
				cardRight: "||",
				label:     "IS_SUBCLASS",
				dashed:    true,
			})
		}

		buf.WriteString("\n")

		for _, r := range relations {
			sep := "--"
			if r.dashed {
				sep = ".."
			}
			fmt.Fprintf(
				&buf, "  %q %s%s%s %q : %q\n",
				r.source, r.cardLeft, sep, r.cardRight, r.target, r.label,
			)
		}

		fmt.Fprintf(&buf, "  %q {\n", e.name)
		for _, f := range rows {
			flagStr := ""
			if len(f.flags) > 0 {
				flagStr = " " + strings.Join(f.flags, ",")
			}
			fmt.Fprintf(&buf, "    %s %s%s %q\n", f.valueType, f.name, flagStr, f.comment)
		}
		buf.WriteString("  }\n")
	}

	_, err := io.WriteString(w, buf.String())
	return errors.WithStack(err)
}

// validateDiagramTypes logs a warning for every class in classRegistry whose
// full Go struct does not embed documentFieldsType (the shared DocumentFields
// the diagram inlines into every entity) or, when the class also has a fields
// struct in classFieldsRegistry, does not embed that own-fields struct.
//
// Abstract classes are absent from classRegistry (they have no instantiable Go
// struct), so they are not checked: there is no full type to inspect.
func validateDiagramTypes(
	logger zerolog.Logger,
	classRegistry, classFieldsRegistry map[identifier.Identifier]reflect.Type,
	documentFieldsType reflect.Type,
) {
	for id, fullType := range classRegistry {
		structType := fullType
		if structType.Kind() == reflect.Pointer {
			structType = structType.Elem()
		}

		if !embedsStruct(structType, documentFieldsType) {
			logger.Warn().
				Str("classID", id.String()).
				Str("goType", fullType.String()).
				Str("fieldsType", documentFieldsType.String()).
				Msg("class type does not embed the shared DocumentFields the diagram assumes")
		}

		ownType, ok := classFieldsRegistry[id]
		if !ok {
			continue
		}
		if !embedsStruct(structType, ownType) {
			logger.Warn().
				Str("classID", id.String()).
				Str("goType", fullType.String()).
				Str("fieldsType", ownType.String()).
				Msg("class type does not embed its ClassFieldsRegistry fields struct")
		}
	}
}

// embedsStruct reports whether target equals structType or is embedded
// anonymously (recursively, through value or pointer embeds) within it.
func embedsStruct(structType, target reflect.Type) bool {
	if structType == target {
		return true
	}
	if structType.Kind() != reflect.Struct {
		return false
	}
	for i := range structType.NumField() {
		field := structType.Field(i)
		if !field.Anonymous {
			continue
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() != reflect.Struct {
			continue
		}
		if embedsStruct(fieldType, target) {
			return true
		}
	}
	return false
}

// collectDiagramEntities merges [core.ClassDescriptionRegistry] (mnemonic
// + SUBCLASS_OF facts, including abstract classes) with
// [core.ClassFieldsRegistry] (the Go struct holding each class's own
// fields) into a sorted list of entities. Classes whose ID is in skipIDs are
// omitted entirely.
func collectDiagramEntities(
	logger zerolog.Logger,
	skipIDs map[identifier.Identifier]bool,
) ([]diagramEntity, map[identifier.Identifier]string, errors.E) {
	byID := map[identifier.Identifier]diagramEntity{}

	for _, fn := range core.ClassDescriptionRegistry {
		docs, errE := fn(nil)
		if errE != nil {
			return nil, nil, errE
		}
		for _, doc := range docs {
			id, mnemonic, parents, ok := extractDiagramClassInfo(logger, doc)
			if !ok {
				continue
			}
			if skipIDs[id] {
				continue
			}
			byID[id] = diagramEntity{
				typ:     nil,
				name:    mnemonic,
				id:      id,
				parents: parents,
			}
		}
	}

	for id, t := range core.ClassFieldsRegistry {
		if skipIDs[id] {
			continue
		}
		e, ok := byID[id]
		if !ok {
			// A class with own fields but no description - use the Go type
			// name as a fallback so it is still visible.
			logger.Warn().
				Str("classID", id.String()).
				Str("goType", t.String()).
				Msg("class has a fields struct but no description; using Go type name as entity label")
			e = diagramEntity{typ: nil, name: t.Name(), id: id, parents: nil}
		}
		e.typ = t
		byID[id] = e
	}

	// Drop parent references to classes that are not part of the diagram. A
	// parent might be missing because skipIDs removed it (silent) or because
	// nobody registered the parent class (worth a warning).
	for id, e := range byID {
		if len(e.parents) == 0 {
			continue
		}
		kept := e.parents[:0]
		for _, p := range e.parents {
			if _, present := byID[p]; present {
				kept = append(kept, p)
				continue
			}
			if skipIDs[p] {
				continue
			}
			logger.Warn().
				Str("child", e.name).
				Str("parentID", p.String()).
				Msg("SUBCLASS_OF parent is not in any registry; dropping edge")
		}
		e.parents = kept
		byID[id] = e
	}

	entities := make([]diagramEntity, 0, len(byID))
	idToName := make(map[identifier.Identifier]string, len(byID))
	for _, e := range byID {
		entities = append(entities, e)
		idToName[e.id] = e.name
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].name < entities[j].name
	})

	return entities, idToName, nil
}

// extractDiagramClassInfo reads (classID, mnemonic, parentClassIDs) from a
// class description document via reflection on documentid,
// property:"MNEMONIC", and property:"SUBCLASS_OF" tags. It expects the doc
// to be a (pointer to) struct shaped like core.Class.
func extractDiagramClassInfo(logger zerolog.Logger, doc any) (identifier.Identifier, string, []identifier.Identifier, bool) {
	v := reflect.ValueOf(doc)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			logger.Warn().Msg("class description is a nil pointer; skipping")
			return identifier.Identifier{}, "", nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		logger.Warn().
			Str("kind", v.Kind().String()).
			Str("type", fmt.Sprintf("%T", doc)).
			Msg("class description is not a struct; skipping")
		return identifier.Identifier{}, "", nil, false
	}

	var base []string
	var mnemonic string
	var parents []identifier.Identifier
	scanDiagramClassFields(v, &base, &mnemonic, &parents)
	if len(base) == 0 {
		logger.Warn().
			Str("type", v.Type().String()).
			Str("mnemonic", mnemonic).
			Msg("class description has no documentid base; skipping")
		return identifier.Identifier{}, "", nil, false
	}
	if mnemonic == "" {
		// A class description without a mnemonic cannot be referenced by name.
		logger.Warn().
			Str("type", v.Type().String()).
			Strs("base", base).
			Msg("class description has no mnemonic; skipping")
		return identifier.Identifier{}, "", nil, false
	}
	return identifier.From(base...), mnemonic, parents, true
}

// scanDiagramClassFields walks the struct (and embedded structs) collecting
// the document base ID, the MNEMONIC string, and the SUBCLASS_OF reference
// targets.
func scanDiagramClassFields(
	v reflect.Value,
	base *[]string, mnemonic *string, parents *[]identifier.Identifier,
) {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		fv := v.Field(i)
		if sf.Anonymous && fv.Kind() == reflect.Struct {
			scanDiagramClassFields(fv, base, mnemonic, parents)
			continue
		}
		if _, ok := sf.Tag.Lookup("documentid"); ok {
			*base = readDiagramStringSlice(fv)
			continue
		}
		switch sf.Tag.Get("property") {
		case "MNEMONIC":
			if fv.Kind() == reflect.String {
				*mnemonic = fv.String()
			}
		case "SUBCLASS_OF":
			if fv.Kind() != reflect.Slice {
				continue
			}
			for j := range fv.Len() {
				rv := fv.Index(j)
				if rv.Kind() != reflect.Struct {
					continue
				}
				idField := rv.FieldByName("ID")
				if !idField.IsValid() {
					continue
				}
				parts := readDiagramStringSlice(idField)
				if len(parts) > 0 {
					*parents = append(*parents, identifier.From(parts...))
				}
			}
		}
	}
}

// readDiagramStringSlice extracts a []string from a reflect.Value, returning
// nil for incompatible kinds.
func readDiagramStringSlice(v reflect.Value) []string {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.String {
		return nil
	}
	out := make([]string, v.Len())
	for i := range v.Len() {
		out[i] = v.Index(i).String()
	}
	return out
}

// collectDiagramEntity walks a struct type and gathers field rows and outgoing relations.
func collectDiagramEntity(
	logger zerolog.Logger,
	structType reflect.Type, entityName string,
	idToName map[identifier.Identifier]string,
	skipIDs map[identifier.Identifier]bool,
) ([]diagramFieldRow, []diagramRelation, errors.E) {
	var rows []diagramFieldRow
	var relations []diagramRelation

	var walk func(t reflect.Type) errors.E
	walk = func(t reflect.Type) errors.E {
		for i := range t.NumField() {
			field := t.Field(i)

			if _, ok := field.Tag.Lookup("documentid"); ok {
				rows = append(rows, diagramFieldRow{
					valueType: "string",
					name:      "ID",
					flags:     []string{"PK"},
					comment:   "",
				})
				continue
			}

			if _, ok := field.Tag.Lookup("value"); ok {
				continue
			}

			propertyMnemonic := field.Tag.Get("property")
			if propertyMnemonic == "-" {
				continue
			}

			if field.Anonymous && field.Type.Kind() == reflect.Struct {
				errE := walk(field.Type)
				if errE != nil {
					return errE
				}
				continue
			}

			if propertyMnemonic == "" {
				continue
			}

			// INSTANCE_OF always points to CLASS; when CLASS is skipped, suppress the row.
			if skipIDs[internalCore.ClassClassID] && propertyMnemonic == "INSTANCE_OF" {
				continue
			}

			fieldLogger := logger.With().Str("entity", entityName).Str("property", propertyMnemonic).Logger()
			valueType, isRef := classifyDiagramValueType(fieldLogger, field.Type, field.Tag.Get("type"))
			cardMin, cardMax := classifyDiagramCardinality(fieldLogger, field)

			flags := []string{}
			if isRef {
				flags = append(flags, "FK")
			}

			rows = append(rows, diagramFieldRow{
				valueType: valueType,
				name:      propertyMnemonic,
				flags:     flags,
				comment:   cardinalityLabel(cardMin, cardMax),
			})

			// Emit edges from a Ref-typed field whose values tag resolves to a registered class.
			for _, target := range resolveDiagramRefTargets(fieldLogger, field, propertyMnemonic, idToName, skipIDs) {
				relations = append(relations, diagramRelation{
					source:    entityName,
					target:    target,
					cardLeft:  cardinalityLeftSymbol(cardMin, cardMax),
					cardRight: cardinalityRightSymbol(cardMin, cardMax),
					label:     propertyMnemonic,
					dashed:    false,
				})
			}

			// Walk sub-fields of struct-valued fields so nested fields become rows
			// and nested Refs become edges too.
			walkSubFields(logger, entityName, propertyMnemonic, field.Type, idToName, &rows, &relations, map[reflect.Type]bool{}, skipIDs)
		}
		return nil
	}

	errE := walk(structType)
	if errE != nil {
		return nil, nil, errE
	}

	return rows, relations, nil
}

// walkSubFields recurses into a struct-valued field, emitting one row per
// property-tagged sub-field (named "PARENT.SUB") and an edge whenever the
// sub-field is a Ref whose values tag resolves to a registered class. The
// visited set guards against recursion through self-referential types like
// Field.SubField.
func walkSubFields(
	logger zerolog.Logger,
	entityName, parentMnemonic string, fieldType reflect.Type,
	idToName map[identifier.Identifier]string,
	rows *[]diagramFieldRow, relations *[]diagramRelation,
	visited map[reflect.Type]bool,
	skipIDs map[identifier.Identifier]bool,
) {
	t := internalCore.UnwrapSliceAndPointer(fieldType)
	if t.Kind() != reflect.Struct {
		return
	}
	if internalCore.IsKnownType(t) {
		return
	}
	if visited[t] {
		return
	}
	visited[t] = true
	defer delete(visited, t)

	for i := range t.NumField() {
		f := t.Field(i)
		if _, ok := f.Tag.Lookup("documentid"); ok {
			continue
		}
		// The value:"" field is already represented by the parent row's type.
		if _, ok := f.Tag.Lookup("value"); ok {
			continue
		}
		mnemonic := f.Tag.Get("property")
		if mnemonic == "-" {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			walkSubFields(logger, entityName, parentMnemonic, f.Type, idToName, rows, relations, visited, skipIDs)
			continue
		}
		if mnemonic == "" {
			continue
		}

		compoundName := parentMnemonic + "[" + mnemonic + "]"
		fieldLogger := logger.With().Str("entity", entityName).Str("property", compoundName).Logger()
		valueType, isRef := classifyDiagramValueType(fieldLogger, f.Type, f.Tag.Get("type"))
		cardMin, cardMax := classifyDiagramCardinality(fieldLogger, f)

		flags := []string{}
		if isRef {
			flags = append(flags, "FK")
		}

		*rows = append(*rows, diagramFieldRow{
			valueType: valueType,
			name:      compoundName,
			flags:     flags,
			comment:   cardinalityLabel(cardMin, cardMax),
		})

		for _, target := range resolveDiagramRefTargets(fieldLogger, f, compoundName, idToName, skipIDs) {
			*relations = append(*relations, diagramRelation{
				source:    entityName,
				target:    target,
				cardLeft:  cardinalityLeftSymbol(cardMin, cardMax),
				cardRight: cardinalityRightSymbol(cardMin, cardMax),
				label:     compoundName,
				dashed:    false,
			})
		}

		walkSubFields(logger, entityName, compoundName, f.Type, idToName, rows, relations, visited, skipIDs)
	}
}

// classifyDiagramValueType returns the PeerDB value-type label for a Go type
// (matching the VALUE_TYPE vocabulary entries) and reports whether the type is
// a reference (so the row can be flagged FK). A type the classifier cannot
// match is logged at warn level and returned as "N/A".
func classifyDiagramValueType(logger zerolog.Logger, fieldType reflect.Type, typeTag string) (string, bool) {
	t := internalCore.UnwrapSliceAndPointer(fieldType)

	switch {
	case t == internalCore.RefType:
		return "reference", true
	case t == internalCore.TimeType, t == internalCore.StdTimeType:
		return "time", false
	case t == internalCore.TimeIntervalType:
		return "time_interval", false
	case t == internalCore.IdentifierType:
		return "identifier", false
	case t == internalCore.LinkType:
		return "link", false
	case t == internalCore.FileType:
		return "file", false
	case t == internalCore.HTMLType, t == internalCore.RawHTMLType:
		return "html", false
	case t == internalCore.NoneType:
		return "none", false
	case t == internalCore.UnknownType:
		return "unknown", false
	case internalCore.AmountTypes[t]:
		return "amount", false
	case internalCore.AmountIntervalTypes[t]:
		return "amount_interval", false
	}

	switch t.Kind() { //nolint:exhaustive
	case reflect.String:
		switch typeTag {
		case "id":
			return "identifier", false
		case "html", "rawhtml":
			return "html", false
		case "link":
			return "link", false
		case "file":
			return "file", false
		default:
			return "string", false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "amount", false
	case reflect.Bool:
		switch typeTag {
		case "none":
			return "none", false
		case "unknown":
			return "unknown", false
		default:
			return "has", false
		}
	case reflect.Struct:
		vt, ok := classifyDiagramStructValueType(logger, t)
		if ok {
			return vt, vt == "reference"
		}
		return "has", false
	}

	logger.Warn().
		Str("type", fieldType.String()).
		Str("typeTag", typeTag).
		Msg("unable to classify field type; falling back to N/A")
	return "N/A", false
}

// classifyDiagramStructValueType inspects a struct for a value:"" field and
// classifies its type. Returns ok=false if no value field is present.
func classifyDiagramStructValueType(logger zerolog.Logger, structType reflect.Type) (string, bool) {
	for i := range structType.NumField() {
		f := structType.Field(i)
		if _, ok := f.Tag.Lookup("value"); ok {
			vt, _ := classifyDiagramValueType(logger, f.Type, f.Tag.Get("type"))
			return vt, true
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			vt, ok := classifyDiagramStructValueType(logger, f.Type)
			if ok {
				return vt, true
			}
		}
	}
	return "", false
}

// classifyDiagramCardinality returns the (min, max) cardinality for a field.
// max == -1 means unbounded. Slice fields default to 0..unbounded; pointer or
// single-value fields default to 0..1. Malformed cardinality tags are logged
// at warn level and the field falls back to the permissive 0..unbounded.
func classifyDiagramCardinality(logger zerolog.Logger, field reflect.StructField) (int, int) {
	tag := field.Tag.Get("cardinality")
	if tag != "" {
		minC, maxC, errE := internalCore.ParseCardinalityTag(tag)
		if errE != nil {
			logger.Warn().
				Str("cardinality", tag).
				Err(errE).
				Msg("malformed cardinality tag; falling back to 0..*")
			return 0, -1
		}
		return minC, maxC
	}
	t := field.Type
	if t.Kind() == reflect.Slice {
		return 0, -1
	}
	return 0, 1
}

// cardinalityLabel renders (min, max) as "min..max" with "*" for unbounded max.
func cardinalityLabel(minC, maxC int) string {
	if maxC == -1 {
		return fmt.Sprintf("%d..*", minC)
	}
	if minC == maxC {
		return strconv.Itoa(minC)
	}
	return fmt.Sprintf("%d..%d", minC, maxC)
}

// cardinalityRightSymbol maps (min, max) to the right-side Mermaid cardinality symbol.
func cardinalityRightSymbol(minC, maxC int) string {
	if maxC == -1 {
		if minC == 0 {
			return "o{"
		}
		return "|{"
	}
	if maxC <= 1 {
		if minC == 0 {
			return "o|"
		}
		return "||"
	}
	if minC == 0 {
		return "o{"
	}
	return "|{"
}

// cardinalityLeftSymbol returns the left-side Mermaid cardinality. Without an
// explicit inverse-cardinality declaration we cannot constrain how many
// sources reference the same target, so we default to "zero or many".
func cardinalityLeftSymbol(_, _ int) string {
	return "}o"
}

// resolveDiagramRefTargets returns target entity names for INSTANCE_OF clauses
// in a field's "values" tag.
func resolveDiagramRefTargets(
	logger zerolog.Logger,
	field reflect.StructField,
	propertyName string,
	idToName map[identifier.Identifier]string,
	skipIDs map[identifier.Identifier]bool,
) []string {
	tag, isRef := diagramValuesTag(field)
	if isRef && tag == "" {
		logger.Warn().
			Str("property", propertyName).
			Str("type", field.Type.String()).
			Msg("Ref-typed field has no values tag; FK row will have no edge")
	}
	return parseDiagramValuesTargets(logger, tag, propertyName, idToName, skipIDs)
}

// diagramValuesTag returns the values-tag string that applies to the given
// property field, choosing the outer field or its inner value:"" field based
// on the field's element type. The boolean reports whether the resolved
// field is core.Ref (i.e. the row would be flagged FK), which lets callers
// warn when a Ref is missing its values tag.
func diagramValuesTag(field reflect.StructField) (string, bool) {
	if internalCore.UnwrapSliceAndPointer(field.Type) == internalCore.RefType {
		return field.Tag.Get("values"), true
	}
	return diagramStructValuesTag(field.Type)
}

// diagramStructValuesTag walks into a struct type and returns the "values" tag
// on its value:"" field (recursing through anonymous embeds). Returns ("", false)
// when the type is not a struct or has no value:"" field; otherwise the second
// return reports whether the inner value:"" field is core.Ref.
func diagramStructValuesTag(t reflect.Type) (string, bool) {
	t = internalCore.UnwrapSliceAndPointer(t)
	if t.Kind() != reflect.Struct {
		return "", false
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if _, ok := f.Tag.Lookup("value"); ok {
			return f.Tag.Get("values"), internalCore.UnwrapSliceAndPointer(f.Type) == internalCore.RefType
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if tag, isRef := diagramStructValuesTag(f.Type); tag != "" || isRef {
				return tag, isRef
			}
		}
	}
	return "", false
}

// parseDiagramValuesTargets parses a values-tag string (search shortcut grammar)
// and returns the target entity names for INSTANCE_OF=class clauses that
// resolve to entries in idToName. INSTANCE_OF clauses whose target class is
// not part of the diagram are logged at warn level - this is the canonical
// "FK row without an outgoing edge" case.
func parseDiagramValuesTargets(
	logger zerolog.Logger,
	tag string,
	propertyName string,
	idToName map[identifier.Identifier]string,
	skipIDs map[identifier.Identifier]bool,
) []string {
	if tag == "" {
		return nil
	}
	var targets []string
	for entry := range strings.SplitSeq(tag, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		for part := range strings.SplitSeq(entry, "&") {
			eq := strings.IndexByte(part, '=')
			if eq <= 0 || eq == len(part)-1 {
				continue
			}
			key, value := part[:eq], part[eq+1:]
			keyID, ok := resolveDiagramValuesID(key)
			if !ok || keyID != internalCore.InstanceOfPropID {
				continue
			}
			valueID, ok := resolveDiagramValuesID(value)
			if !ok {
				logger.Warn().
					Str("property", propertyName).
					Str("value", value).
					Msg("values tag clause has unparseable target identifier; skipping")
				continue
			}
			name, ok := idToName[valueID]
			if !ok {
				// Target class is intentionally excluded (e.g. by --skip-core);
				// stay silent. Otherwise it is a real missing-registration.
				if skipIDs[valueID] {
					continue
				}
				logger.Warn().
					Str("property", propertyName).
					Str("targetID", valueID.String()).
					Str("targetToken", value).
					Msg("values tag points to a class not in the diagram; FK row will have no edge")
				continue
			}
			if !slices.Contains(targets, name) {
				targets = append(targets, name)
			}
		}
	}
	return targets
}

// resolveDiagramValuesID resolves a single identifier token from a "values" tag
// into an [identifier.Identifier]. The token is either a 22-character base58 ID
// or a comma-separated list of base parts. Because the "values" tag shares the
// search shortcut grammar, the grammar's non-identifier tokens can also appear:
// the "self"/"reverse" sentinels and nested "parent:prop" keys are not class or
// property identifiers, so they return ok=false (and thus produce no edge).
func resolveDiagramValuesID(token string) (identifier.Identifier, bool) {
	if token == "" || token == "self" || token == "reverse" {
		return identifier.Identifier{}, false
	}
	if strings.Contains(token, ":") {
		// Nested keys ("parent:prop") are not used for class resolution here.
		return identifier.Identifier{}, false
	}
	if strings.Contains(token, ",") {
		parts := strings.Split(token, ",")
		if slices.Contains(parts, "") {
			return identifier.Identifier{}, false
		}
		return identifier.From(parts...), true
	}
	id, errE := identifier.MaybeString(token)
	if errE != nil {
		return identifier.Identifier{}, false
	}
	return id, true
}
