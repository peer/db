package store

import "reflect"

func isNoneType[T any]() bool {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	none := reflect.TypeOf((*None)(nil)).Elem()
	return typ == none
}
