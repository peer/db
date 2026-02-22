package core

import (
	"time"

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
	Timestamp time.Time              `json:"timestamp"`
	Precision document.TimePrecision `json:"precision"`
}

// Interval represents a time interval.
type Interval struct {
	From          *Time `json:"from,omitempty"`
	FromIsUnknown bool  `json:"fromIsUnknown,omitempty"`
	To            *Time `json:"to,omitempty"`
	ToIsUnknown   bool  `json:"toIsUnknown,omitempty"`
}

// DocumentFields contains common fields for all documents.
type DocumentFields struct {
	ID         []string `                  documentid:"" json:"id"`
	InstanceOf []Ref    `cardinality:"0.."               json:"instanceOf,omitempty" property:"INSTANCE_OF"`
}

// PropertyName represents a property name with language information.
type PropertyName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyShortName represents a property short name with language information.
type PropertyShortName struct {
	ShortName string `json:"shortName" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyDescription represents a property description with language information.
type PropertyDescription struct {
	Description HTML `json:"description" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyInstruction represents a property instruction with language information.
type PropertyInstruction struct {
	Instruction HTML `json:"instruction" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// PropertyFields contains fields specific to properties.
type PropertyFields struct {
	Name          []PropertyName        `cardinality:"1.."  json:"name"                    property:"NAME"`
	ShortName     []PropertyShortName   `cardinality:"0.."  json:"shortName,omitempty"     property:"SHORT_NAME"`
	Mnemonic      string                `cardinality:"0..1" json:"mnemonic,omitempty"      property:"MNEMONIC"`
	Description   []PropertyDescription `cardinality:"0.."  json:"description,omitempty"   property:"DESCRIPTION"`
	Instruction   []PropertyInstruction `cardinality:"0.."  json:"instruction,omitempty"   property:"INSTRUCTION"`
	SubpropertyOf []Ref                 `cardinality:"0.."  json:"subpropertyOf,omitempty" property:"SUBPROPERTY_OF"`
}

// Property represents a property document.
type Property struct {
	PropertyFields
	DocumentFields
}

// ClassName represents a class name with language information.
type ClassName struct {
	Name string `json:"name" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// ClassShortName represents a class short name with language information.
type ClassShortName struct {
	ShortName string `json:"shortName" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// ClassDescription represents a class description with language information.
type ClassDescription struct {
	Description HTML `json:"description" value:""`

	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" property:"IN_LANGUAGE"`
}

// ClassFields contains fields specific to classes.
type ClassFields struct {
	Name        []ClassName        `cardinality:"1.."  json:"name"                  property:"NAME"`
	ShortName   []ClassShortName   `cardinality:"0.."  json:"shortName,omitempty"   property:"SHORT_NAME"`
	Mnemonic    string             `cardinality:"0..1" json:"mnemonic,omitempty"    property:"MNEMONIC"`
	Description []ClassDescription `cardinality:"0.."  json:"description,omitempty" property:"DESCRIPTION"`
	SubclassOf  []Ref              `cardinality:"0.."  json:"subclassOf,omitempty"  property:"SUBCLASS_OF"`
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

// VocabularyFields contains fields specific to vocabularies.
type VocabularyFields struct {
	Name []VocabularyName `cardinality:"1.." json:"name"           property:"NAME"`
	Code []Identifier     `cardinality:"0.." json:"code,omitempty" property:"CODE"`
}

// Language represents a language vocabulary document.
type Language struct {
	VocabularyFields
	DocumentFields
}
