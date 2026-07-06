// Package core provides core types used across packages.
package core

import (
	"math"
	"time"

	"gitlab.com/tozd/go/errors"

	internalDocument "gitlab.com/peerdb/peerdb/internal/document"
)

// Ref represents a reference to another document by ID.
type Ref struct {
	ID []string `json:"id"`
}

// Identifier is a string identifier.
type Identifier string

// Link is a string URL, URI or IRI.
type Link string

// File is a string URL, URI or IRI of a file.
type File string

// HTML is a string with plain text which transform converts to HTML
// (escaped, linkified, wrapped into a paragraph block) and canonicalizes.
type HTML string

// RawHTML is a string with HTML which transform does not escape, only canonicalizes
// (parses into the editor schema and serializes back). Because transform canonicalizes
// any value, a value which is not already canonical is silently rewritten, and content
// the editor schema cannot represent is dropped. Authoring values already in the
// canonical form (the form document.CanonicalizeHTML returns unchanged and the frontend
// editor serializer produces) keeps the stored HTML byte for byte equal to the source
// and to what the editor produces for the same content.
type RawHTML string

// None is a boolean that indicates a property is known to not have a value.
type None bool

// Unknown is a boolean that indicates a property value exists but is unknown or cannot be determined.
type Unknown bool

// Time represents a time with precision.
type Time struct {
	// We do not use document.Time here for easier interoperability with other systems.
	Time      time.Time                      `json:"time"`
	Precision internalDocument.TimePrecision `json:"precision"`
}

func (Time) intervalBound() {}

// Validate checks that Precision is a defined TimePrecision value.
func (t Time) Validate() errors.E {
	if t.Precision < internalDocument.TimePrecisionGigaYears || t.Precision > internalDocument.TimePrecisionNanosecond {
		return errors.New("unknown precision")
	}
	return nil
}

// AmountType is an interface for all amount types.
type AmountType interface {
	int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64
}

// Amount represents a numeric amount with precision.
//
// Infinite or NaN values are not supported for Amount[float32] and Amount[float64].
type Amount[T AmountType] struct {
	// We do not use document.Amount here for easier interoperability with other systems.
	Amount    T `json:"amount"`
	Precision T `json:"precision"`
}

func (Amount[T]) intervalBound() {}

// Validate checks that the amount values are finite numbers and precision is positive.
func (a Amount[T]) Validate() errors.E {
	switch v := any(a).(type) {
	case Amount[float32]:
		if math.IsInf(float64(v.Amount), 0) || math.IsNaN(float64(v.Amount)) {
			return errors.New("amount must be a finite number")
		}
		if math.IsInf(float64(v.Precision), 0) || math.IsNaN(float64(v.Precision)) {
			return errors.New("precision must be a finite number")
		}
	case Amount[float64]:
		if math.IsInf(v.Amount, 0) || math.IsNaN(v.Amount) {
			return errors.New("amount must be a finite number")
		}
		if math.IsInf(v.Precision, 0) || math.IsNaN(v.Precision) {
			return errors.New("precision must be a finite number")
		}
	}
	if a.Precision <= 0 {
		return errors.New("precision must be positive")
	}
	return nil
}

// IntervalBound is a type constraint for interval bounds.
type IntervalBound interface {
	Validate() errors.E

	// Only Time and Amount implement this unexported method.
	intervalBound()
}

// Interval field indices for reflect access.
const (
	IntervalFromIdx          = 0
	IntervalFromIsOpenIdx    = 1
	IntervalFromIsUnknownIdx = 2
	IntervalFromIsNoneIdx    = 3
	IntervalToIdx            = 4
	IntervalToIsOpenIdx      = 5
	IntervalToIsUnknownIdx   = 6
	IntervalToIsNoneIdx      = 7
)

// Interval represents an interval between two values.
//
// If From or To is nil, it is zero value, unless *IsUnknown or *IsNone is true, respectively.
//
// Only one of FromIs* fields can be set at a time. If FromIsUnknown or FromIsNone is true, From must be nil.
// Only one of ToIs* fields can be set at a time. If ToIsUnknown or ToIsNone is true, To must be nil.
//
// FromIsOpen and ToIsOpen are exclusive-bound flags.
type Interval[T IntervalBound] struct {
	From          *T   `json:"from,omitempty"`
	FromIsOpen    bool `json:"fromIsOpen,omitempty"`
	FromIsUnknown bool `json:"fromIsUnknown,omitempty"`
	FromIsNone    bool `json:"fromIsNone,omitempty"`

	To          *T   `json:"to,omitempty"`
	ToIsOpen    bool `json:"toIsOpen,omitempty"`
	ToIsUnknown bool `json:"toIsUnknown,omitempty"`
	ToIsNone    bool `json:"toIsNone,omitempty"`
}

// Validate checks that the interval has valid bounds.
func (i *Interval[T]) Validate() errors.E {
	fromIsCount := 0
	if i.FromIsOpen {
		fromIsCount++
	}
	if i.FromIsUnknown {
		fromIsCount++
	}
	if i.FromIsNone {
		fromIsCount++
	}
	if fromIsCount > 1 {
		return errors.New("only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
	}
	if i.From != nil && (i.FromIsUnknown || i.FromIsNone) {
		return errors.New("From must not be set when FromIsUnknown or FromIsNone is true")
	}
	if i.From != nil {
		errE := (*i.From).Validate()
		if errE != nil {
			return errE
		}
	}

	toIsCount := 0
	if i.ToIsOpen {
		toIsCount++
	}
	if i.ToIsUnknown {
		toIsCount++
	}
	if i.ToIsNone {
		toIsCount++
	}
	if toIsCount > 1 {
		return errors.New("only one of ToIsOpen, ToIsUnknown, ToIsNone can be set")
	}
	if i.To != nil && (i.ToIsUnknown || i.ToIsNone) {
		return errors.New("To must not be set when ToIsUnknown or ToIsNone is true")
	}
	if i.To != nil {
		errE := (*i.To).Validate()
		if errE != nil {
			return errE
		}
	}

	return nil
}

// StringWithLanguage represents string with language information.
type StringWithLanguage struct {
	Value string `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" order:"-" property:"IN_LANGUAGE" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// RawHTMLWithLanguage represents raw HTML with language information.
type RawHTMLWithLanguage struct {
	Value RawHTML `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" order:"-" property:"IN_LANGUAGE" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,LANGUAGE"`
}

// Section represents a section of fields of an entity.
//
// ID is the section's stable identity (the name used in section struct tags) and Name its
// translated display names. Both use the NAME property; consumers access them by claim type
// (identifier claim vs string claims).
type Section struct {
	ID          Identifier           `cardinality:"1"   json:"id"              property:"NAME"`
	Name        []StringWithLanguage `cardinality:"1.." json:"name"            property:"NAME"`
	OrderInList Amount[float64]      `cardinality:"1"   json:"orderInList"     property:"ORDER_IN_LIST"`
	Field       []Field              `cardinality:"0.." json:"field,omitempty" property:"FIELD"`
}

// Field represents a field of an entity.
//
//nolint:lll
type Field struct {
	Property        Ref                   `cardinality:"1"    json:"property"                  property:"HAS_PROPERTY"      values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,PROPERTY"`
	ValueType       Ref                   `cardinality:"1"    json:"valueType"                 property:"HAS_VALUE_TYPE"    values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,VALUE_TYPE"`
	OrderInList     Amount[float64]       `cardinality:"1"    json:"orderInList"               property:"ORDER_IN_LIST"`
	Cardinality     Interval[Amount[int]] `cardinality:"1"    json:"cardinality"               property:"CARDINALITY"`
	Values          []string              `cardinality:"0.."  json:"values,omitempty"          property:"FIELD_VALUES"`
	SubField        []Field               `cardinality:"0.."  json:"subField,omitempty"        property:"SUB_FIELD"`
	InverseProperty *Ref                  `cardinality:"0..1" json:"inverseProperty,omitempty" property:"INVERSE_PROPERTY"  values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,PROPERTY"`
	Embed           []string              `cardinality:"0.."  json:"embed,omitempty"           property:"EMBED_PROPERTY"`
	Default         *Ref                  `cardinality:"0..1" json:"default,omitempty"         property:"FIELD_DEFAULT"     values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,VALUE_TYPE"`
	Instruction     []RawHTMLWithLanguage `cardinality:"0.."  json:"instruction,omitempty"     property:"FIELD_INSTRUCTION"`
}

// Fields represents a list of fields of an entity.
type Fields struct {
	Section []Section `cardinality:"0.." json:"section,omitempty" property:"SECTION"`
	Field   []Field   `cardinality:"0.." json:"field,omitempty"   property:"FIELD"`
}
