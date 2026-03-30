package core

import (
	"gitlab.com/tozd/identifier"
)

// Well-known core property IDs based on their mnemonics.
//
// Keep this list in sync with src/core/properties.ts and sorted alphabetically.
//
//nolint:gochecknoglobals
var (
	AlternativeNamePropID      = identifier.From(Namespace, "ALTERNATIVE_NAME")
	CardinalityPropID          = identifier.From(Namespace, "CARDINALITY")
	CodePropID                 = identifier.From(Namespace, "CODE")
	DefaultVariantPropID       = identifier.From(Namespace, "DEFAULT_VARIANT")
	DescriptionPropID          = identifier.From(Namespace, "DESCRIPTION")
	DisplayLabelTemplatePropID = identifier.From(Namespace, "DISPLAY_LABEL_TEMPLATE")
	DistinctFromPropID         = identifier.From(Namespace, "DISTINCT_FROM")
	FieldPropID                = identifier.From(Namespace, "FIELD")
	FieldsPropID               = identifier.From(Namespace, "FIELDS")
	FieldValuesPropID          = identifier.From(Namespace, "FIELD_VALUES")
	HasPropertyPropID          = identifier.From(Namespace, "HAS_PROPERTY")
	HasValueTypePropID         = identifier.From(Namespace, "HAS_VALUE_TYPE")
	InLanguagePropID           = identifier.From(Namespace, "IN_LANGUAGE")
	InLocationPropID           = identifier.From(Namespace, "IN_LOCATION")
	InstanceOfPropID           = identifier.From(Namespace, "INSTANCE_OF")
	InstructionPropID          = identifier.From(Namespace, "INSTRUCTION")
	InUnitPropID               = identifier.From(Namespace, "IN_UNIT")
	InversePropertyOfPropID    = identifier.From(Namespace, "INVERSE_PROPERTY_OF")
	InversePropertyPropID      = identifier.From(Namespace, "INVERSE_PROPERTY")
	ListPropID                 = identifier.From(Namespace, "LIST")
	MediaTypePropID            = identifier.From(Namespace, "MEDIA_TYPE")
	MnemonicPropID             = identifier.From(Namespace, "MNEMONIC")
	NamePropID                 = identifier.From(Namespace, "NAME")
	NamingPropID               = identifier.From(Namespace, "NAMING")
	OrderInListPropID          = identifier.From(Namespace, "ORDER_IN_LIST")
	SectionPropID              = identifier.From(Namespace, "SECTION")
	SelectedVariantPropID      = identifier.From(Namespace, "SELECTED_VARIANT")
	SettingPropID              = identifier.From(Namespace, "SETTING")
	ShortNamePropID            = identifier.From(Namespace, "SHORT_NAME")
	SubclassOfPropID           = identifier.From(Namespace, "SUBCLASS_OF")
	SubentityOfPropID          = identifier.From(Namespace, "SUBENTITY_OF")
	SubFieldPropID             = identifier.From(Namespace, "SUB_FIELD")
	SubpropertyOfPropID        = identifier.From(Namespace, "SUBPROPERTY_OF")
	TitlePropID                = identifier.From(Namespace, "TITLE")
	VariantPropID              = identifier.From(Namespace, "VARIANT")
)
