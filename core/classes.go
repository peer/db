// Package core provides core classes, properties, and vocabularies.
package core

import (
	"gitlab.com/tozd/go/errors"
)

// Classes returns core classes.
func Classes() ([]any, errors.E) {
	documents := []any{}

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []StringWithLanguage{{
				Value: "class",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "razred",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "CLASS",
			Description:     nil,
			SubclassOf:      nil,
			Fields:          nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "CLASS"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
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
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "attribute",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "atribut",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "characteristic",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "značilnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic: "PROPERTY",
			Description: []RawHTMLWithLanguage{{
				Value: "A document describes a property.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Dokument opisuje lastnost.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			SubclassOf: nil,
			Fields:     nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PROPERTY"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []StringWithLanguage{{
				Value: "document",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "dokument",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DOCUMENT",
			Description:     nil,
			SubclassOf:      nil,
			Fields:          nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "DOCUMENT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []StringWithLanguage{{
				Value: "vocabulary",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "slovar",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "code book",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "šifrant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:    "VOCABULARY",
			Description: nil,
			SubclassOf:  nil,
			Fields:      nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VOCABULARY"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []StringWithLanguage{{
				Value: "language",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "jezik",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "LANGUAGE",
			Description:     nil,
			SubclassOf: []Ref{{
				ID: []string{Namespace, "VOCABULARY"},
			}},
			Fields: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []StringWithLanguage{{
				Value: "unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "enota",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []StringWithLanguage{{
				Value: "unit of measurement",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "enota mere",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "measurement unit",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "unit of measure",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "merska enota",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:    "UNIT",
			Description: nil,
			SubclassOf: []Ref{{
				ID: []string{Namespace, "VOCABULARY"},
			}},
			Fields: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	documents = append(documents, &Class{
		ClassFields: ClassFields{
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
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "VALUE_TYPE",
			Description:     nil,
			SubclassOf: []Ref{{
				ID: []string{Namespace, "VOCABULARY"},
			}},
			Fields: nil,
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	return documents, nil
}
