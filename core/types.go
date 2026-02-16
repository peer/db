package core

import (
	"time"

	"gitlab.com/peerdb/peerdb/document"
)

// Ref represents a reference to another document by ID.
type Ref struct {
	ID []string
}

// Identifier is a string identifier.
type Identifier string

// URL is a string URL.
type URL string

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
	Timestamp time.Time
	Precision document.TimePrecision
}

// Interval represents a time interval.
type Interval struct {
	From          *Time
	FromIsUnknown bool
	To            *Time
	ToIsUnknown   bool
}

// DocumentFields contains common fields for all documents.
type DocumentFields struct {
	ID         []string `                  documentid:""`
	InstanceOf []Ref    `cardinality:"0.."               property:"INSTANCE_OF"`
}

// PropertyName represents a property name with language information.
type PropertyName struct {
	Name string `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// PropertyShortName represents a property short name with language information.
type PropertyShortName struct {
	ShortName string `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// PropertyDescription represents a property description with language information.
type PropertyDescription struct {
	Description HTML `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// PropertyInstruction represents a property instruction with language information.
type PropertyInstruction struct {
	Instruction HTML `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// PropertyFields contains fields specific to properties.
type PropertyFields struct {
	Name          []PropertyName        `cardinality:"1.."  property:"NAME"`
	ShortName     []PropertyShortName   `cardinality:"0.."  property:"SHORT_NAME"`
	Mnemonic      string                `cardinality:"0..1" property:"MNEMONIC"`
	Description   []PropertyDescription `cardinality:"0.."  property:"DESCRIPTION"`
	Instruction   []PropertyInstruction `cardinality:"0.."  property:"INSTRUCTION"`
	SubpropertyOf []Ref                 `cardinality:"0.."  property:"SUBPROPERTY_OF"`
}

// Property represents a property document.
type Property struct {
	PropertyFields
	DocumentFields
}

// ClassName represents a class name with language information.
type ClassName struct {
	Name string `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// ClassShortName represents a class short name with language information.
type ClassShortName struct {
	ShortName string `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// ClassDescription represents a class description with language information.
type ClassDescription struct {
	Description HTML `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// ClassFields contains fields specific to classes.
type ClassFields struct {
	Name        []ClassName        `cardinality:"1.."  property:"NAME"`
	ShortName   []ClassShortName   `cardinality:"0.."  property:"SHORT_NAME"`
	Mnemonic    string             `cardinality:"0..1" property:"MNEMONIC"`
	Description []ClassDescription `cardinality:"0.."  property:"DESCRIPTION"`
	SubclassOf  []Ref              `cardinality:"0.."  property:"SUBCLASS_OF"`
}

// Class represents a class document.
type Class struct {
	ClassFields
	DocumentFields
}

// VocabularyName represents a vocabulary name with language information.
type VocabularyName struct {
	Name string `value:""`

	InLanguage []Ref `cardinality:"0.." property:"IN_LANGUAGE"`
}

// VocabularyFields contains fields specific to vocabularies.
type VocabularyFields struct {
	Name []VocabularyName `cardinality:"1.." property:"NAME"`
	Code []Identifier     `cardinality:"0.." property:"CODE"`
}

// Language represents a language vocabulary document.
type Language struct {
	VocabularyFields
	DocumentFields
}
