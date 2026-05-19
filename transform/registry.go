package transform

import (
	"reflect"

	"gitlab.com/tozd/identifier"
)

// ClassRegistry maps class document IDs to the reflect.Type of the Go struct
// that represents instances of that class. External packages register their
// types in init() functions.
//
//nolint:gochecknoglobals
var ClassRegistry = map[identifier.Identifier]reflect.Type{}
