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

// HTML is a string with plain text which transform converts to HTML
// (escaped, linkified, wrapped into a paragraph block) and canonicalizes.
type HTML = internalCore.HTML

// RawHTML is a string with HTML which transform does not escape, only canonicalizes
// (parses into the editor schema and serializes back). Because transform canonicalizes
// any value, a value which is not already canonical is silently rewritten, and content
// the editor schema cannot represent is dropped. Authoring values already in the
// canonical form (the form document.CanonicalizeHTML returns unchanged and the frontend
// editor serializer produces) keeps the stored HTML byte for byte equal to the source
// and to what the editor produces for the same content.
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
	InstanceOf []Ref `cardinality:"0.." json:"instanceOf,omitempty" order:"-" property:"INSTANCE_OF" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,CLASS"`
}

// AmountWithUnit represents an amount with its unit.
type AmountWithUnit[T AmountType] struct {
	Value Amount[T] `json:"value" value:""`

	InUnit []Ref `cardinality:"0.." context:"edit" json:"inUnit,omitempty" order:"1" property:"IN_UNIT" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT"`
}

// AmountIntervalWithUnit represents an amount interval with its unit.
type AmountIntervalWithUnit[T AmountType] struct {
	Value Interval[Amount[T]] `json:"value" value:""`

	InUnit []Ref `cardinality:"0.." context:"edit" json:"inUnit,omitempty" order:"1" property:"IN_UNIT" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT"`
}

// TimeWithLocation represents a time with location information.
type TimeWithLocation struct {
	Value Time `json:"value" value:""`

	InLocation []Identifier `cardinality:"0.." json:"inLocation,omitempty" order:"1" property:"IN_LOCATION"`
}

// TimeIntervalWithLocation represents a time interval with location information.
type TimeIntervalWithLocation struct {
	Value Interval[Time] `json:"value" value:""`

	InLocation []Identifier `cardinality:"0.." json:"inLocation,omitempty" order:"1" property:"IN_LOCATION"`
}

// HTMLWithLanguage represents HTML with language information.
//
//nolint:lll
type HTMLWithLanguage struct {
	Value HTML `json:"value" value:""`

	InLanguage []Ref `cardinality:"0.." context:"edit" json:"inLanguage,omitempty" order:"1" property:"IN_LANGUAGE" values:"id=languages"`
}

// RawHTMLWithLanguage represents raw HTML with language information.
type RawHTMLWithLanguage = internalCore.RawHTMLWithLanguage

// SearchShortcut represents a search shortcut with its name.
type SearchShortcut struct {
	Value string `json:"value" value:""`

	Name           []StringWithLanguage `cardinality:"0.."  json:"name,omitempty"           property:"NAME"`
	CreateShortcut string               `cardinality:"0..1" json:"createShortcut,omitempty" property:"CREATE_SHORTCUT"`
}

// PropertyFields contains fields specific to properties.
//
//nolint:lll
type PropertyFields struct {
	Name                   []StringWithLanguage  `cardinality:"1.."  json:"name"                             property:"NAME"`
	ShortName              []StringWithLanguage  `cardinality:"0.."  json:"shortName,omitempty"              property:"SHORT_NAME"`
	AlternativeName        []StringWithLanguage  `cardinality:"0.."  json:"alternativeName,omitempty"        property:"ALTERNATIVE_NAME"`
	Mnemonic               string                `cardinality:"0..1" json:"mnemonic,omitempty"               property:"MNEMONIC"`
	Description            []RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty"            property:"DESCRIPTION"`
	IdentifierLinkTemplate string                `cardinality:"0..1" json:"identifierLinkTemplate,omitempty" property:"IDENTIFIER_LINK_TEMPLATE"`
	SubpropertyOf          []Ref                 `cardinality:"0.."  json:"subpropertyOf,omitempty"          property:"SUBPROPERTY_OF"           values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,PROPERTY"`
	InversePropertyOf      *Ref                  `cardinality:"0..1" json:"inversePropertyOf,omitempty"      property:"INVERSE_PROPERTY_OF"      values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,PROPERTY"`
}

// Property represents a property document.
type Property struct {
	PropertyFields
	DocumentFields
}

// ClassFields contains fields specific to classes.
//
//nolint:lll
type ClassFields struct {
	Name                 []StringWithLanguage  `cardinality:"1.."  json:"name"                           property:"NAME"`
	ShortName            []StringWithLanguage  `cardinality:"0.."  json:"shortName,omitempty"            property:"SHORT_NAME"`
	AlternativeName      []StringWithLanguage  `cardinality:"0.."  json:"alternativeName,omitempty"      property:"ALTERNATIVE_NAME"`
	Mnemonic             string                `cardinality:"0..1" json:"mnemonic,omitempty"             property:"MNEMONIC"`
	Description          []RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty"          property:"DESCRIPTION"`
	SubclassOf           []Ref                 `cardinality:"0.."  json:"subclassOf,omitempty"           property:"SUBCLASS_OF"            values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,CLASS"`
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

// PageFields contains fields specific to a page.
type PageFields struct {
	Title       []StringWithLanguage  `cardinality:"1.."  json:"title"                 property:"NAME"`
	Mnemonic    string                `cardinality:"0..1" json:"mnemonic,omitempty"    property:"MNEMONIC"`
	Description []RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty" property:"DESCRIPTION"`
	Content     []RawHTMLWithLanguage `cardinality:"0.."  json:"content,omitempty"     property:"CONTENT"`
}

// Page represents a page document.
type Page struct {
	PageFields
	DocumentFields
}
