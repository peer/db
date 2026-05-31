package core

import (
	"reflect"
	"time"
)

// Reflected types for core value and wrapper types.
//
//nolint:gochecknoglobals
var (
	RefType          = reflect.TypeFor[Ref]()
	TimeType         = reflect.TypeFor[Time]()
	StdTimeType      = reflect.TypeFor[time.Time]()
	TimeIntervalType = reflect.TypeFor[Interval[Time]]()
	IdentifierType   = reflect.TypeFor[Identifier]()
	LinkType         = reflect.TypeFor[Link]()
	FileType         = reflect.TypeFor[File]()
	HTMLType         = reflect.TypeFor[HTML]()
	RawHTMLType      = reflect.TypeFor[RawHTML]()
	NoneType         = reflect.TypeFor[None]()
	UnknownType      = reflect.TypeFor[Unknown]()
)

// AmountTypes is the set of every Amount[T] reflect.Type. Membership testing
// is the canonical way to detect that a Go value is one of the supported
// numeric Amount instantiations.
//
//nolint:gochecknoglobals
var AmountTypes = map[reflect.Type]bool{
	reflect.TypeFor[Amount[int]]():     true,
	reflect.TypeFor[Amount[int8]]():    true,
	reflect.TypeFor[Amount[int16]]():   true,
	reflect.TypeFor[Amount[int32]]():   true,
	reflect.TypeFor[Amount[int64]]():   true,
	reflect.TypeFor[Amount[uint]]():    true,
	reflect.TypeFor[Amount[uint8]]():   true,
	reflect.TypeFor[Amount[uint16]]():  true,
	reflect.TypeFor[Amount[uint32]]():  true,
	reflect.TypeFor[Amount[uint64]]():  true,
	reflect.TypeFor[Amount[float32]](): true,
	reflect.TypeFor[Amount[float64]](): true,
}

// AmountIntervalTypes is the set of every Interval[Amount[T]] reflect.Type,
// the companion to [AmountTypes] for the bounded-interval variant.
//
//nolint:gochecknoglobals
var AmountIntervalTypes = map[reflect.Type]bool{
	reflect.TypeFor[Interval[Amount[int]]]():     true,
	reflect.TypeFor[Interval[Amount[int8]]]():    true,
	reflect.TypeFor[Interval[Amount[int16]]]():   true,
	reflect.TypeFor[Interval[Amount[int32]]]():   true,
	reflect.TypeFor[Interval[Amount[int64]]]():   true,
	reflect.TypeFor[Interval[Amount[uint]]]():    true,
	reflect.TypeFor[Interval[Amount[uint8]]]():   true,
	reflect.TypeFor[Interval[Amount[uint16]]]():  true,
	reflect.TypeFor[Interval[Amount[uint32]]]():  true,
	reflect.TypeFor[Interval[Amount[uint64]]]():  true,
	reflect.TypeFor[Interval[Amount[float32]]](): true,
	reflect.TypeFor[Interval[Amount[float64]]](): true,
}

// ScalarTypes is the union of single-value core types (Ref, Time, std time,
// TimeInterval, Identifier, Link, File, HTML, RawHTML, None, Unknown). Use
// this to gate "is this a known core scalar/wrapper, not a user struct"
// decisions; combine with [AmountTypes] / [AmountIntervalTypes] when you also
// want the Amount/Interval families included.
//
//nolint:gochecknoglobals
var ScalarTypes = map[reflect.Type]bool{
	RefType:          true,
	TimeType:         true,
	StdTimeType:      true,
	TimeIntervalType: true,
	IdentifierType:   true,
	LinkType:         true,
	FileType:         true,
	HTMLType:         true,
	RawHTMLType:      true,
	NoneType:         true,
	UnknownType:      true,
}

// IsKnownType reports whether t is any known core value or wrapper type
// (one of [ScalarTypes], [AmountTypes], or [AmountIntervalTypes]).
// Reflection-driven walkers use this to short-circuit traversal: a known core
// type should never be descended into for sub-fields.
func IsKnownType(t reflect.Type) bool {
	return ScalarTypes[t] || AmountTypes[t] || AmountIntervalTypes[t]
}

// UnwrapSliceAndPointer strips one level of slice and one level of pointer
// from t, in that order. Reflection-driven walkers call this to reach the
// underlying element type of a property field, which may be declared as
// `T`, `[]T`, `*T`, or `[]*T`.
func UnwrapSliceAndPointer(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
