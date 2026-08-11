package xeno

import (
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
)

// languageEN, languageSL and languagePT are the three languages the test data is written in. Every
// human-readable label of the schema is given in all of them, so the language handling of the
// frontend has something to choose between.
const (
	languageEN = "en-GB"
	languageSL = "sl-SI"
	languagePT = "pt-PT"
)

// language returns a reference to a core language vocabulary document.
func language(code string) core.Ref {
	return core.Ref{ID: []string{core.Namespace, "LANGUAGE", code}}
}

// coreRef returns a reference to a core document with the given ID segments.
func coreRef(parts ...string) core.Ref {
	return core.Ref{ID: append([]string{core.Namespace}, parts...)}
}

// ref returns a reference to a test data document with the given ID segments.
func ref(parts ...string) core.Ref {
	return core.Ref{ID: append([]string{Namespace}, parts...)}
}

// strings3 returns a string value in every language. Passing the same text more than once is how a
// label which happens to be identical in two of them is written; it still has to be given per
// language because a consumer picks the value by language and a missing one is a missing label.
func strings3(en, sl, pt string) []core.StringWithLanguage {
	return []core.StringWithLanguage{
		{Value: en, InLanguage: []core.Ref{language(languageEN)}},
		{Value: sl, InLanguage: []core.Ref{language(languageSL)}},
		{Value: pt, InLanguage: []core.Ref{language(languagePT)}},
	}
}

// html3 returns an HTML value in every language. The values are expected to be canonical HTML
// (paragraph-wrapped), because transform canonicalizes them anyway and a non-canonical value would
// be silently rewritten.
func html3(en, sl, pt string) []core.RawHTMLWithLanguage {
	return []core.RawHTMLWithLanguage{
		{Value: core.RawHTML(en), InLanguage: []core.Ref{language(languageEN)}},
		{Value: core.RawHTML(sl), InLanguage: []core.Ref{language(languageSL)}},
		{Value: core.RawHTML(pt), InLanguage: []core.Ref{language(languagePT)}},
	}
}

// propertyOption configures a property built by property.
type propertyOption func(*core.PropertyFields)

// describes gives the property a description in both languages.
func describes(en, sl, pt string) propertyOption {
	return func(p *core.PropertyFields) {
		p.Description = html3(en, sl, pt)
	}
}

// subpropertyOf makes the property a subproperty of the given test data properties. Claims of a
// subproperty are also indexed for its ancestors when the site enables ancestor indexing, so a
// search for the parent property finds documents which only carry the child.
func subpropertyOf(mnemonics ...string) propertyOption {
	return func(p *core.PropertyFields) {
		for _, mnemonic := range mnemonics {
			p.SubpropertyOf = append(p.SubpropertyOf, ref(mnemonic))
		}
	}
}

// subpropertyOfCore makes the property a subproperty of a core property. Declaring one under
// SUBENTITY_OF is what tells the indexer that the property defines a hierarchy between the documents
// it points at, the way SUBCLASS_OF does between classes: the facet for it nests, and a filter on an
// ancestor also matches everything below it.
func subpropertyOfCore(mnemonics ...string) propertyOption {
	return func(p *core.PropertyFields) {
		for _, mnemonic := range mnemonics {
			p.SubpropertyOf = append(p.SubpropertyOf, coreRef(mnemonic))
		}
	}
}

// inverseOf declares the property to be the inverse of another test data property, so that a claim
// of one is presented on the referenced document as a claim of the other.
func inverseOf(mnemonic string) propertyOption {
	return func(p *core.PropertyFields) {
		r := ref(mnemonic)
		p.InversePropertyOf = &r
	}
}

// linkTemplate gives the property an RFC 6570 level 1 template turning an identifier claim value
// into a link, so catalogue codes render as links to the (imaginary) registry which issued them.
func linkTemplate(template string) propertyOption {
	return func(p *core.PropertyFields) {
		p.IdentifierLinkTemplate = template
	}
}

// notTextSearchable keeps the property's values out of the full-text index, for values which are
// only ever filtered on and would otherwise pollute text search.
func notTextSearchable() propertyOption {
	return func(p *core.PropertyFields) {
		p.ExcludeFromTextSearch = true
	}
}

// alternativeNames gives the property alternative names in both languages, which text search also
// matches.
func alternativeNames(en, sl, pt string) propertyOption {
	return func(p *core.PropertyFields) {
		p.AlternativeName = strings3(en, sl, pt)
	}
}

// property builds a test data property document.
func property(mnemonic, en, sl, pt string, opts ...propertyOption) *core.Property {
	fields := core.PropertyFields{
		Name:                   strings3(en, sl, pt),
		ShortName:              nil,
		AlternativeName:        nil,
		Mnemonic:               mnemonic,
		Description:            nil,
		IdentifierLinkTemplate: "",
		ExcludeFromTextSearch:  false,
		SubpropertyOf:          nil,
		InversePropertyOf:      nil,
	}
	for _, opt := range opts {
		opt(&fields)
	}
	return &core.Property{
		PropertyFields: fields,
		DocumentFields: core.DocumentFields{
			ID:         []string{Namespace, mnemonic},
			InstanceOf: []core.Ref{coreRef("PROPERTY")},
		},
	}
}

// classOption configures a class built by class.
type classOption func(*core.ClassFields)

// classDescribes gives the class a description in both languages.
func classDescribes(en, sl, pt string) classOption {
	return func(c *core.ClassFields) {
		c.Description = html3(en, sl, pt)
	}
}

// subclassOf makes the class a subclass of the given test data classes.
func subclassOf(mnemonics ...string) classOption {
	return func(c *core.ClassFields) {
		for _, mnemonic := range mnemonics {
			c.SubclassOf = append(c.SubclassOf, ref(mnemonic))
		}
	}
}

// subclassOfCore makes the class a subclass of a core class, which is how the controlled
// vocabularies of the test data declare themselves to be vocabularies.
func subclassOfCore(mnemonics ...string) classOption {
	return func(c *core.ClassFields) {
		for _, mnemonic := range mnemonics {
			c.SubclassOf = append(c.SubclassOf, coreRef(mnemonic))
		}
	}
}

// abstract marks the class as one which is never instantiated directly and only groups its
// subclasses.
func abstract() classOption {
	return func(c *core.ClassFields) {
		c.AbstractClass = true
	}
}

// classAlternativeNames gives the class alternative names in both languages.
func classAlternativeNames(en, sl, pt string) classOption {
	return func(c *core.ClassFields) {
		c.AlternativeName = strings3(en, sl, pt)
	}
}

// displayLabel gives the class the templates rendering an instance's label, one per language.
func displayLabel(en, sl, pt string) classOption {
	return func(c *core.ClassFields) {
		c.DisplayLabelTemplate = strings3(en, sl, pt)
	}
}

// shortcuts gives the class search shortcuts, the saved searches offered from a document of the
// class.
func shortcuts(s ...core.SearchShortcut) classOption {
	return func(c *core.ClassFields) {
		c.SearchShortcut = append(c.SearchShortcut, s...)
	}
}

// class builds a test data class document. The fields schema is generated from the entity struct by
// the caller, and is nil when the caller was given no mnemonics.
func class(mnemonic, en, sl, pt string, fields *core.Fields, opts ...classOption) *core.Class {
	f := core.ClassFields{
		Name:                 strings3(en, sl, pt),
		ShortName:            nil,
		AlternativeName:      nil,
		Mnemonic:             mnemonic,
		Description:          nil,
		SubclassOf:           nil,
		AbstractClass:        false,
		DisplayLabelTemplate: nil,
		SearchShortcut:       nil,
		Fields:               fields,
	}
	for _, opt := range opts {
		opt(&f)
	}
	return &core.Class{
		ClassFields: f,
		DocumentFields: core.DocumentFields{
			ID:         []string{Namespace, mnemonic},
			InstanceOf: []core.Ref{coreRef("CLASS")},
		},
	}
}

// backlink returns a search shortcut listing the documents of the given test data class which refer
// to the document the shortcut is offered from, through the given test data property.
func backlink(prop, instanceOf, en, sl, pt string) core.SearchShortcut {
	return core.SearchShortcut{
		Value:          Namespace + "," + prop + "=self&" + core.Namespace + ",INSTANCE_OF=" + Namespace + "," + instanceOf,
		Name:           strings3(en, sl, pt),
		CreateShortcut: "",
	}
}

// backlinkCreate is a backlink shortcut which also offers creating a document of the class with the
// reference back already filled in.
func backlinkCreate(prop, instanceOf, en, sl, pt string) core.SearchShortcut {
	s := backlink(prop, instanceOf, en, sl, pt)
	s.CreateShortcut = "limit=" + Namespace + "," + instanceOf + "&" + Namespace + "," + prop + "=self"
	return s
}

// propID renders a test data property's ID the way a display label template has to spell it: as an
// identifierString call, because the template functions take identifiers and not mnemonics.
func propID(mnemonic string) string {
	return `(identifierString "` + identifier.From(Namespace, mnemonic).String() + `")`
}

// namePropID is the core name property rendered the way a display label template has to spell it.
// Every display label starts from the name, so it is the one core property the templates need.
//
//nolint:gochecknoglobals
var namePropID = `(identifierString "` + identifier.From(core.Namespace, "NAME").String() + `")`
