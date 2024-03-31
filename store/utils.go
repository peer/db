package store

import "reflect"

func isAnyType[T any]() bool {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	return typ.String() == "interface {}"
}
