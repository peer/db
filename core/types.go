package core

import (
	"math"
	"time"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/document"
)

// Ref represents a reference to another document by ID.
type Ref struct {
	ID []string `json:"id"`
}

// Identifier is a string identifier.
type Identifier string

// IRI is a string URL, URI or IRI.
type IRI string

// HTML is a string with HTML.
type HTML string

// RawHTML is a string with HTML that will not be escaped.
type RawHTML string

// None is a boolean that indicates a property is known to not have a value.
type None bool

// Unknown is a boolean that indicates a property value exists but is unknown or cannot be determined.
type Unknown bool

// Time represents a time with precision.
type Time struct {
	// We do not use document.Timestamp here for easier interoperability with other systems.
	Timestamp time.Time              `json:"time"`
	Precision document.TimePrecision `json:"precision"`
}

// Validate checks that Precision is a defined TimePrecision value.
func (t Time) Validate() errors.E {
	if t.Precision < document.TimePrecisionGigaYears || t.Precision > document.TimePrecisionNanosecond {
		return errors.New("unknown Precision")
	}
	return nil
}

// Amount represents a numeric amount with precision.
//
// Infinite or NaN values are not supported for Amount[float32] and Amount[float64].
type Amount[T int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64] struct {
	Amount    T `json:"amount"`
	Precision T `json:"precision"`
}

// Validate checks that the amount values are finite numbers.
func (a Amount[T]) Validate() errors.E {
	switch v := any(a).(type) {
	case Amount[float32]:
		if math.IsInf(float64(v.Amount), 0) || math.IsNaN(float64(v.Amount)) {
			return errors.New("Amount must be a finite number")
		}
		if math.IsInf(float64(v.Precision), 0) || math.IsNaN(float64(v.Precision)) {
			return errors.New("Precision must be a finite number")
		}
	case Amount[float64]:
		if math.IsInf(v.Amount, 0) || math.IsNaN(v.Amount) {
			return errors.New("Amount must be a finite number")
		}
		if math.IsInf(v.Precision, 0) || math.IsNaN(v.Precision) {
			return errors.New("Precision must be a finite number")
		}
	}
	return nil
}

// IntervalBound is a type constraint for interval bounds.
type IntervalBound interface {
	Time | Amount[int] | Amount[int8] | Amount[int16] | Amount[int32] | Amount[int64] |
		Amount[uint] | Amount[uint8] | Amount[uint16] | Amount[uint32] | Amount[uint64] |
		Amount[float32] | Amount[float64]
	Validate() errors.E
}

// Interval represents an interval between two values.
//
// If From or To is nil, it is zero value value, unless *IsUnknown or *IsNone is true, respectively.
//
// Only one of FromIs* fields can be set at a time. If FromIsUnknown or FromIsNone is true, From must be nil.
// Only one of ToIs* fields can be set at a time. If ToIsUnknown or ToIsNone is true, To must be nil.
type Interval[T IntervalBound] struct {
	From          *T   `json:"from,omitempty"`
	FromIsOpen    bool `json:"fromIsOpen,omitempty"`
	FromIsUnknown bool `json:"fromIsUnknown,omitempty"`
	FromIsNone    bool `json:"fromIsNone,omitempty"`

	To          *T   `json:"to,omitempty"`
	ToIsClosed  bool `json:"toIsClosed,omitempty"`
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
	if i.ToIsClosed {
		toIsCount++
	}
	if i.ToIsUnknown {
		toIsCount++
	}
	if i.ToIsNone {
		toIsCount++
	}
	if toIsCount > 1 {
		return errors.New("only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
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

// DocumentFields contains common fields for all documents.
type DocumentFields struct {
	ID         []string `                  documentid:"" json:"id"`
	InstanceOf []Ref    `cardinality:"0.."               json:"instanceOf,omitempty" property:"INSTANCE_OF"`
}

// PropertyName represents a property name (main, short or alternative) with language information.
type PropertyName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyDescription represents a property description with language information.
type PropertyDescription struct {
	Description RawHTML `json:"description" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyInstruction represents a property instruction with language information.
type PropertyInstruction struct {
	Instruction RawHTML `json:"instruction" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyFields contains fields specific to properties.
type PropertyFields struct {
	Name            []PropertyName        `cardinality:"1.."  json:"name"                      property:"NAME"`
	ShortName       []PropertyName        `cardinality:"0.."  json:"shortName,omitempty"       property:"SHORT_NAME"`
	AlternativeName []PropertyName        `cardinality:"0.."  json:"alternativeName,omitempty" property:"ALTERNATIVE_NAME"`
	Mnemonic        string                `cardinality:"0..1" json:"mnemonic,omitempty"        property:"MNEMONIC"`
	Description     []PropertyDescription `cardinality:"0.."  json:"description,omitempty"     property:"DESCRIPTION"`
	Instruction     []PropertyInstruction `cardinality:"0.."  json:"instruction,omitempty"     property:"INSTRUCTION"`
	SubpropertyOf   []Ref                 `cardinality:"0.."  json:"subpropertyOf,omitempty"   property:"SUBPROPERTY_OF"`
}

// Property represents a property document.
type Property struct {
	PropertyFields
	DocumentFields
}

// ClassName represents a class name (main, short or alternative) with language information.
type ClassName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// ClassDescription represents a class description with language information.
type ClassDescription struct {
	Description RawHTML `json:"description" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// ClassFields contains fields specific to classes.
type ClassFields struct {
	Name            []ClassName        `cardinality:"1.."  json:"name"                      property:"NAME"`
	ShortName       []ClassName        `cardinality:"0.."  json:"shortName,omitempty"       property:"SHORT_NAME"`
	AlternativeName []ClassName        `cardinality:"0.."  json:"alternativeName,omitempty" property:"ALTERNATIVE_NAME"`
	Mnemonic        string             `cardinality:"0..1" json:"mnemonic,omitempty"        property:"MNEMONIC"`
	Description     []ClassDescription `cardinality:"0.."  json:"description,omitempty"     property:"DESCRIPTION"`
	SubclassOf      []Ref              `cardinality:"0.."  json:"subclassOf,omitempty"      property:"SUBCLASS_OF"`
}

// Class represents a class document.
type Class struct {
	ClassFields
	DocumentFields
}

// VocabularyName represents a vocabulary name with language information.
type VocabularyName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// VocabularyDescription represents a vocabulary description with language information.
type VocabularyDescription struct {
	Description RawHTML `json:"description" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// VocabularyFields contains fields specific to vocabularies.
type VocabularyFields struct {
	Name        []VocabularyName        `cardinality:"1.." json:"name"                  property:"NAME"`
	Description []VocabularyDescription `cardinality:"0.." json:"description,omitempty" property:"DESCRIPTION"`
	Code        []Identifier            `cardinality:"0.." json:"code,omitempty"        property:"CODE"`
}

// Language represents a language vocabulary document.
type Language struct {
	VocabularyFields
	DocumentFields
}

// Unit represents a unit of measurement vocabulary document.
type Unit struct {
	VocabularyFields
	DocumentFields
}

// SectionName represents a name of a section.
type SectionName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// Section represents a section of fields of an entity.
type Section struct {
	Name        []SectionName `cardinality:"1.." json:"name"            property:"NAME"`
	OrderInList int           `cardinality:"1"   json:"orderInList"     property:"ORDER_IN_LIST"`
	Field       []Field       `cardinality:"0.." json:"field,omitempty" property:"FIELD"`
}

// FieldName represents a name of a field.
type FieldName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// Field represents a field of an entity.
type Field struct {
	Name        []FieldName           `cardinality:"1.." json:"name"             property:"NAME"`
	OrderInList int                   `cardinality:"1"   json:"orderInList"      property:"ORDER_IN_LIST"`
	Cardinality Interval[Amount[int]] `cardinality:"1"   json:"cardinality"      property:"CARDINALITY"   unit:"1"`
	Values      []Identifier          `cardinality:"0.." json:"values,omitempty" property:"FIELD_VALUES"`
}

// Fields represents a list of fields of an entity.
type Fields struct {
	Section []Section `cardinality:"0.." json:"section,omitempty" property:"SECTION"`
	Field   []Field   `cardinality:"0.." json:"field,omitempty"   property:"FIELD"`
}
