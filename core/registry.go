package core

import (
	"reflect"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// ClassRegistry maps class document IDs to the reflect.Type of the Go struct
// that represents instances of that class. External packages register their
// types in init() functions.
//
//nolint:gochecknoglobals
var ClassRegistry = map[identifier.Identifier]reflect.Type{}

// ClassFieldsRegistry maps class document IDs to the reflect.Type of the Go
// struct that holds only that class's own fields (i.e. the property-tagged
// fields declared on the class itself, excluding anything inherited from
// parent classes via Go struct embedding). A class without own fields (a
// pure leaf in the data model) should be omitted from this registry.
// External packages register their fields types in init() functions.
//
//nolint:gochecknoglobals
var ClassFieldsRegistry = map[identifier.Identifier]reflect.Type{}

// ClassDescriptionFunc returns the class description documents for a set of
// related classes. The mnemonics parameter resolves property mnemonics to
// document base IDs when constructing the Fields schema. Implementations
// should accept a nil mnemonics map and omit Fields in that case so callers
// can introspect class metadata without having a mnemonics map.
type ClassDescriptionFunc func(mnemonics map[string][]string) ([]any, errors.E)

// ClassDescriptionRegistry is the list of class description constructors.
// External packages append to it from init() functions to register the
// classes they define (including abstract parents that have no entry in
// [ClassRegistry]).
//
//nolint:gochecknoglobals
var ClassDescriptionRegistry []ClassDescriptionFunc

func init() { //nolint:gochecknoinits
	ClassRegistry[identifier.From(Namespace, "CLASS")] = reflect.TypeFor[Class]()
	ClassRegistry[identifier.From(Namespace, "PROPERTY")] = reflect.TypeFor[Property]()
	ClassRegistry[identifier.From(Namespace, "LANGUAGE")] = reflect.TypeFor[Language]()
	ClassRegistry[identifier.From(Namespace, "UNIT")] = reflect.TypeFor[Unit]()
	ClassRegistry[identifier.From(Namespace, "VALUE_TYPE")] = reflect.TypeFor[ValueType]()

	// ClassFieldsRegistry holds only the Go struct that carries a class's own
	// fields (excluding anything inherited via embedding). Classes whose
	// entities are pure leaves of the hierarchy (no own fields) are omitted.

	ClassFieldsRegistry[identifier.From(Namespace, "CLASS")] = reflect.TypeFor[ClassFields]()
	ClassFieldsRegistry[identifier.From(Namespace, "PROPERTY")] = reflect.TypeFor[PropertyFields]()
	ClassFieldsRegistry[identifier.From(Namespace, "VOCABULARY")] = reflect.TypeFor[VocabularyFields]()

	ClassDescriptionRegistry = append(ClassDescriptionRegistry, Classes)
}
