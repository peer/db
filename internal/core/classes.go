package core

import "gitlab.com/tozd/identifier"

// Well-known core class IDs based on their mnemonics.
//
//nolint:gochecknoglobals
var (
	ClassClassID      = identifier.From(Namespace, "CLASS")
	DocumentClassID   = identifier.From(Namespace, "DOCUMENT")
	LanguageClassID   = identifier.From(Namespace, "LANGUAGE")
	PropertyClassID   = identifier.From(Namespace, "PROPERTY")
	UnitClassID       = identifier.From(Namespace, "UNIT")
	ValueTypeClassID  = identifier.From(Namespace, "VALUE_TYPE")
	VocabularyClassID = identifier.From(Namespace, "VOCABULARY")
)
