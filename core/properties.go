package core

import (
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

// Properties returns core properties.
//
//nolint:maintidx
func Properties(_ zerolog.Logger) ([]any, errors.E) {
	documents := []any{}

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []PropertyName{{
				Name: "subentity of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "pod-entiteta od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "SUBENTITY_OF",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "instance of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "instanca od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []PropertyName{{
				Name: "is",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "je",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "kind",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "vrsta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "form",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "oblika",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "category",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "kategorija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:    "INSTANCE_OF",
			Description: nil,
			Instruction: nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
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
			Name: []PropertyName{{
				Name: "subclass of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "pod-razred od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "SUBCLASS_OF",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
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
			Name: []PropertyName{{
				Name: "subproperty of",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "pod-lastnost od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "SUBPROPERTY_OF",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "SUBENTITY_OF"},
			}},
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
			Name: []PropertyName{{
				Name: "distinct from",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "različen od",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DISTINCT_FROM",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "naming",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "poimenovanje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "NAMING",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "NAME",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
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
			Name: []PropertyName{{
				Name: "short name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "kratko ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "SHORT_NAME",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
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
			Name: []PropertyName{{
				Name: "alternative name",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "alternativno ime",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "ALTERNATIVE_NAME",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
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
			Name: []PropertyName{{
				Name: "title",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "naslov",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "TITLE",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
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
			Name: []PropertyName{{
				Name: "description",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "opis",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DESCRIPTION",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "instruction",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "navodilo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "INSTRUCTION",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "mnemonic",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "mnemonik",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "MNEMONIC",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "in language",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "v jeziku",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "IN_LANGUAGE",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "VARIANT",
			Description: []PropertyDescription{{
				Description: "A variant has an unique ID. All values of a variant share this ID.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Description: "Varianta ima enoličen ID. Vse vrednosti variante si delijo ta ID.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:   nil,
			SubpropertyOf: nil,
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
			Name: []PropertyName{{
				Name: "default variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "privzeta varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DEFAULT_VARIANT",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "selected variant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "izbrana varianta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "SELECTED_VARIANT",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf:   nil,
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
			Name: []PropertyName{{
				Name: "list",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "seznam",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "LIST",
			Description: []PropertyDescription{{
				Description: "A list has an unique ID, even a list with just one element. All elements of a list share this ID.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Description: "Seznam ima enoličen ID, celo seznam s samo enim elementom. Vsi elementi seznama si delijo ta ID.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:   nil,
			SubpropertyOf: nil,
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
			Name: []PropertyName{{
				Name: "order in list",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "vrstni red v seznamu",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "ORDER_IN_LIST",
			Description: []PropertyDescription{{
				Description: "Order of an element inside its list. Smaller number is earlier in the list.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Description: "Vrstni red elementa v seznamu. Manjša vrednost je prej v seznamu.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:   nil,
			SubpropertyOf: nil,
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
			Name: []PropertyName{{
				Name: "code",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "koda",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "CODE",
			Description:     nil,
			Instruction:     nil,
			SubpropertyOf: []Ref{{
				ID: []string{Namespace, "NAMING"},
			}},
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
			Name: []PropertyName{{
				Name: "unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "enota",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []PropertyName{{
				Name: "unit of measurement",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "enota mere",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "measurement unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "unit of measure",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "merska enota",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic: "UNIT",
			Description: []PropertyDescription{{
				Description: "Unit associated with an amount.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Description: "Enota številčne vrednosti.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Instruction:   nil,
			SubpropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	documents = append(documents, &Property{
		PropertyFields: PropertyFields{
			Name: []PropertyName{{
				Name: "media type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "tip medija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []PropertyName{{
				Name: "MIME type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "tip MIME",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "IMT",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "Internet media type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "internetni tip medija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "content type",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "tip vsebine",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:      "MEDIA_TYPE",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "MEDIA_TYPE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PROPERTY"},
			}},
		},
	})

	return documents, nil
}
