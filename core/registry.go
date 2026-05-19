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
}
