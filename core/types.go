package core

import (
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// Type aliases for types defined in internal/core.
// These allow the transform package to import internal/core directly,
// breaking the import cycle between core and transform.

// Ref represents a reference to another document by ID.
type Ref = internalCore.Ref

// Identifier is a string identifier.
type Identifier = internalCore.Identifier

// Link is a string URL, URI or IRI.
type Link = internalCore.Link

// File is a string URL, URI or IRI of a file.
type File = internalCore.File

// HTML is a string with HTML.
type HTML = internalCore.HTML

// RawHTML is a string with HTML that will not be escaped.
type RawHTML = internalCore.RawHTML

// None is a boolean that indicates a property is known to not have a value.
type None = internalCore.None

// Unknown is a boolean that indicates a property value exists but is unknown or cannot be determined.
type Unknown = internalCore.Unknown

// Time represents a time with precision.
type Time = internalCore.Time

// AmountType is an interface for all amount types.
type AmountType = internalCore.AmountType

// Amount represents a numeric amount with precision.
type Amount[T AmountType] = internalCore.Amount[T]

// IntervalBound is a type constraint for interval bounds.
type IntervalBound = internalCore.IntervalBound

// Interval represents an interval between two values.
type Interval[T IntervalBound] = internalCore.Interval[T]

// StringWithLanguage represents string with language information.
type StringWithLanguage = internalCore.StringWithLanguage

// Section represents a section of fields of an entity.
type Section = internalCore.Section

// Field represents a field of an entity.
type Field = internalCore.Field

// Fields represents a list of fields of an entity.
type Fields = internalCore.Fields

// DocumentFields contains common fields for all documents.
type DocumentFields struct {
	ID []string `documentid:"" json:"id"`
	// We set "order" to not allow "instance of" to be changed directly through fields.
	InstanceOf []Ref `cardinality:"0.." json:"instanceOf,omitempty" order:"-" property:"INSTANCE_OF"`
}

// AmountWithUnit represents an amount with its unit.
type AmountWithUnit[T AmountType] struct {
	Value Amount[T] `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InUnit []Ref `cardinality:"0.." json:"inUnit,omitempty" order:"-" property:"IN_UNIT"`
}

// AmountIntervalWithUnit represents an amount interval with its unit.
type AmountIntervalWithUnit[T AmountType] struct {
	Value Interval[Amount[T]] `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InUnit []Ref `cardinality:"0.." json:"inUnit,omitempty" order:"-" property:"IN_UNIT"`
}

// TimeWithLocation represents a time with location information.
type TimeWithLocation struct {
	Value Time `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLocation []Identifier `cardinality:"0.." json:"inLocation,omitempty" order:"-" property:"IN_LOCATION"`
}

// TimeIntervalWithLocation represents a time interval with location information.
type TimeIntervalWithLocation struct {
	Value Interval[Time] `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLocation []Identifier `cardinality:"0.." json:"inLocation,omitempty" order:"-" property:"IN_LOCATION"`
}

// HTMLWithLanguage represents HTML with language information.
type HTMLWithLanguage struct {
	Value HTML `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" order:"-" property:"IN_LANGUAGE"`
}

// RawHTMLWithLanguage represents raw HTML with language information.
type RawHTMLWithLanguage struct {
	Value RawHTML `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	InLanguage []Ref `cardinality:"0.." json:"inLanguage,omitempty" order:"-" property:"IN_LANGUAGE"`
}

// SearchShortcut represents a search shortcut with its name.
type SearchShortcut struct {
	Value string `json:"value" value:""`

	Name []StringWithLanguage `cardinality:"0.." json:"name,omitempty" property:"NAME"`
}

// LinkWithMediaType represents link (URL, URI or IRI) with its media type.
type LinkWithMediaType struct {
	Value Link `json:"value" value:""`

	// We set "order" to hide the field. It should not be set manually.
	MediaType []Identifier `cardinality:"0.." json:"mediaType,omitempty" order:"-" property:"MEDIA_TYPE"`
}

// PropertyFields contains fields specific to properties.
type PropertyFields struct {
	Name                   []StringWithLanguage  `cardinality:"1.."  json:"name"                             property:"NAME"`
	ShortName              []StringWithLanguage  `cardinality:"0.."  json:"shortName,omitempty"              property:"SHORT_NAME"`
	AlternativeName        []StringWithLanguage  `cardinality:"0.."  json:"alternativeName,omitempty"        property:"ALTERNATIVE_NAME"`
	Mnemonic               string                `cardinality:"0..1" json:"mnemonic,omitempty"               property:"MNEMONIC"`
	Description            []RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty"            property:"DESCRIPTION"`
	Instruction            []RawHTMLWithLanguage `cardinality:"0.."  json:"instruction,omitempty"            property:"INSTRUCTION"`
	IdentifierLinkTemplate string                `cardinality:"0..1" json:"identifierLinkTemplate,omitempty" property:"IDENTIFIER_LINK_TEMPLATE"`
	SubpropertyOf          []Ref                 `cardinality:"0.."  json:"subpropertyOf,omitempty"          property:"SUBPROPERTY_OF"`
	InversePropertyOf      *Ref                  `cardinality:"0..1" json:"inversePropertyOf,omitempty"      property:"INVERSE_PROPERTY_OF"`
}

// Property represents a property document.
type Property struct {
	PropertyFields
	DocumentFields
}

// ClassFields contains fields specific to classes.
type ClassFields struct {
	Name                 []StringWithLanguage  `cardinality:"1.."  json:"name"                           property:"NAME"`
	ShortName            []StringWithLanguage  `cardinality:"0.."  json:"shortName,omitempty"            property:"SHORT_NAME"`
	AlternativeName      []StringWithLanguage  `cardinality:"0.."  json:"alternativeName,omitempty"      property:"ALTERNATIVE_NAME"`
	Mnemonic             string                `cardinality:"0..1" json:"mnemonic,omitempty"             property:"MNEMONIC"`
	Description          []RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty"          property:"DESCRIPTION"`
	SubclassOf           []Ref                 `cardinality:"0.."  json:"subclassOf,omitempty"           property:"SUBCLASS_OF"`
	AbstractClass        bool                  `cardinality:"0..1" json:"abstractClass,omitempty"        property:"ABSTRACT_CLASS"`
	DisplayLabelTemplate []StringWithLanguage  `cardinality:"0.."  json:"displayLabelTemplate,omitempty" property:"DISPLAY_LABEL_TEMPLATE"`
	SearchShortcut       []SearchShortcut      `cardinality:"0.."  json:"searchShortcut,omitempty"       property:"SEARCH_SHORTCUT"`
	// We set "order" to prevent infinite recursion when determining fields from ClassFields.
	Fields *Fields `cardinality:"0..1" json:"fields,omitempty" order:"-" property:"FIELDS"`
}

// Class represents a class document.
type Class struct {
	ClassFields
	DocumentFields
}

// VocabularyFields contains fields specific to vocabularies.
type VocabularyFields struct {
	Name        []StringWithLanguage  `cardinality:"1.." json:"name"                  property:"NAME"`
	Description []RawHTMLWithLanguage `cardinality:"0.." json:"description,omitempty" property:"DESCRIPTION"`
	Code        []Identifier          `cardinality:"0.." json:"code,omitempty"        property:"CODE"`
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

// ValueType represents a value type vocabulary document.
type ValueType struct {
	VocabularyFields
	DocumentFields
}
