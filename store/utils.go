package store

import "reflect"

func isNoneType[T any]() bool {
	typ := reflect.TypeFor[T]()
	none := reflect.TypeFor[None]()
	return typ == none
}
