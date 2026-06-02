package core

import (
	"reflect"

	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/transform"
)

func init() { //nolint:gochecknoinits
	transform.ClassRegistry[identifier.From(Namespace, "CLASS")] = reflect.TypeFor[Class]()
	transform.ClassRegistry[identifier.From(Namespace, "PROPERTY")] = reflect.TypeFor[Property]()
	transform.ClassRegistry[identifier.From(Namespace, "LANGUAGE")] = reflect.TypeFor[Language]()
	transform.ClassRegistry[identifier.From(Namespace, "UNIT")] = reflect.TypeFor[Unit]()
	transform.ClassRegistry[identifier.From(Namespace, "VALUE_TYPE")] = reflect.TypeFor[ValueType]()

	// ClassFieldsRegistry holds only the Go struct that carries a class's own
	// fields (excluding anything inherited via embedding). Classes whose
	// entities are pure leaves of the hierarchy (no own fields) are omitted.

	transform.ClassFieldsRegistry[identifier.From(Namespace, "CLASS")] = reflect.TypeFor[ClassFields]()
	transform.ClassFieldsRegistry[identifier.From(Namespace, "PROPERTY")] = reflect.TypeFor[PropertyFields]()
	transform.ClassFieldsRegistry[identifier.From(Namespace, "VOCABULARY")] = reflect.TypeFor[VocabularyFields]()

	transform.ClassDescriptionRegistry = append(transform.ClassDescriptionRegistry, Classes)
}
