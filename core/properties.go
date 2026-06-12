package core

import (
	"gitlab.com/tozd/go/errors"
)

// Properties returns core properties.
//
//nolint:maintidx
func Properties() ([]any, errors.E) {
	documents := []any{} //nolint:prealloc

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "subentity of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "pod-entiteta od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SUBENTITY_OF",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SUBENTITY_OF"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "instance of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "instanca od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "is",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "je",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "kind",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "vrsta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "form",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "oblika",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "category",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kategorija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:               "INSTANCE_OF",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "INSTANCE_OF"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "subclass of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "pod-razred od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SUBCLASS_OF",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SUBCLASS_OF"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "subproperty of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "pod-lastnost od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SUBPROPERTY_OF",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SUBPROPERTY_OF"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "inverse property of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "inverzna lastnost od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "INVERSE_PROPERTY_OF",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "INVERSE_PROPERTY_OF"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "abstract class",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "abstrakten razred",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "ABSTRACT_CLASS",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "ABSTRACT_CLASS"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "distinct from",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "različen od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "DISTINCT_FROM",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			// "distinct from" is symmetric, so it is its own inverse.
			InversePropertyOf: &Ref{
				ID: []string{Namespace, "DISTINCT_FROM"},
			},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "DISTINCT_FROM"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "naming",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "poimenovanje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "NAMING",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "NAMING"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "NAME",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "NAME"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "short name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kratko ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SHORT_NAME",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SHORT_NAME"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "alternative name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "alternativno ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "ALTERNATIVE_NAME",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "ALTERNATIVE_NAME"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "title",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "naslov",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "TITLE",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "TITLE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "description",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "opis",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "DESCRIPTION",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "DESCRIPTION"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "instruction",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "navodilo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "INSTRUCTION",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "INSTRUCTION"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "mnemonic",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "mnemonik",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "MNEMONIC",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "MNEMONIC"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "in language",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "v jeziku",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "IN_LANGUAGE",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "IN_LANGUAGE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "in location",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "na lokaciji",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "in timezone",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "v časovnem pasu",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:               "IN_LOCATION",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "IN_LOCATION"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "VARIANT",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>A variant has a unique ID. All values of a variant share this ID.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Varianta ima enoličen ID. Vse vrednosti variante si delijo ta ID.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VARIANT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "default variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "privzeta varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "DEFAULT_VARIANT",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "DEFAULT_VARIANT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "selected variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "izbrana varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SELECTED_VARIANT",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SELECTED_VARIANT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "list",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "seznam",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "LIST",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>A list has a unique ID, even a list with just one element. All elements of a list share this ID.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Seznam ima enoličen ID, celo seznam s samo enim elementom. Vsi elementi seznama si delijo ta ID.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LIST"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "order in list",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "vrstni red v seznamu",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "ORDER_IN_LIST",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>Order of an element inside its list. Smaller number is earlier in the list.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Vrstni red elementa v seznamu. Manjša vrednost je prej v seznamu.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "ORDER_IN_LIST"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "code",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "koda",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "CODE",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "CODE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "media type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "tip medija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "MIME type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "tip MIME",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "IMT",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Internet media type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "internetni tip medija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "content type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "tip vsebine",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:               "MEDIA_TYPE",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "MEDIA_TYPE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "in unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "v enoti",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "in unit of measurement",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "v enoti mere",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "in measurement unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "in unit of measure",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "v merski enoti",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic: "IN_UNIT",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>Unit associated with an amount.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota številčne vrednosti.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "IN_UNIT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "setting",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "nastavitev",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SETTING",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf:          nil,
			InversePropertyOf:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SETTING"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "section",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "razdelek",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SECTION",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SECTION"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "field",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "polje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "FIELD",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "FIELD"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "field values",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "vrednosti polja",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "FIELD_VALUES",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "FIELD_VALUES"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "display label template",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "predloga prikazane oznake",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DISPLAY_LABEL_TEMPLATE",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>A Go text/template template used to render the display label of the document.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Predloga Go text/template za izpis prikazane oznake dokumenta.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "DISPLAY_LABEL_TEMPLATE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "identifier link template",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "predloga povezave identifikatorja",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "IDENTIFIER_LINK_TEMPLATE",
			Description: []RawHTMLWithLanguage{{
				Value: "<p>An RFC 6570 Level 1 URI template with one parameter {identifier} used to construct a link from an identifier value.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Predloga URI po RFC 6570 ravni 1 s parametrom {identifier} za sestavo povezave iz vrednosti identifikatorja.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "IDENTIFIER_LINK_TEMPLATE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "property",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "lastnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "HAS_PROPERTY",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "HAS_PROPERTY"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "value type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "tip vrednosti",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "HAS_VALUE_TYPE",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "HAS_VALUE_TYPE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "sub-field",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "pod-polje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SUB_FIELD",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SUB_FIELD"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "search shortcut",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "bližnjica iskanja",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "SEARCH_SHORTCUT",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "SEARCH_SHORTCUT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "fields",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "polja",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "FIELDS",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "FIELDS"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "inverse property",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "inverzna lastnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "INVERSE_PROPERTY",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "INVERSE_PROPERTY"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []StringWithLanguage{{
				Value: "cardinality",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kardinalnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:              nil,
			AlternativeName:        nil,
			Mnemonic:               "CARDINALITY",
			Description:            nil,
			Instruction:            nil,
			IdentifierLinkTemplate: "",
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SETTING"},
			}},
			InversePropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "CARDINALITY"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	return documents, nil
}
