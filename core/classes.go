// Package core provides core classes, properties, and vocabularies.
package core

import (
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

// Classes returns core classes.
func Classes(_ zerolog.Logger) ([]any, errors.E) {
	documents := []any{}

	documents = append(documents, &Class{
		ClassFields: ClassFields{
			Name: []ClassName{{
				Name: "class",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "razred",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "CLASS",
			Description:     nil,
			SubclassOf:      nil,
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
			Name: []ClassName{{
				Name: "property",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "lastnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []ClassName{{
				Name: "attribute",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "atribut",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Name: "characteristic",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "značilnost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic: "PROPERTY",
			Description: []ClassDescription{{
				Description: "A document describes a property.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Description: "Dokument opisuje lastnost.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			SubclassOf: nil,
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
			Name: []ClassName{{
				Name: "document",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "dokument",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName:       nil,
			AlternativeName: nil,
			Mnemonic:        "DOCUMENT",
			Description:     nil,
			SubclassOf:      nil,
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
			Name: []ClassName{{
				Name: "vocabulary",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "slovar",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			ShortName: nil,
			AlternativeName: []ClassName{{
				Name: "code book",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "šifrant",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Mnemonic:    "VOCABULARY",
			Description: nil,
			SubclassOf:  nil,
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
			Name: []ClassName{{
				Name: "language",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "jezik",
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
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "CLASS"},
			}},
		},
	})

	return documents, nil
}
