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
			Name: []ClassName{
				{
					Name: "class",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "razred",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "CLASS",
			Description: nil,
			SubclassOf:  nil,
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
			Name: []ClassName{
				{
					Name: "property",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "lastnost",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "PROPERTY",
			Description: nil,
			SubclassOf:  nil,
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
			Name: []ClassName{
				{
					Name: "document",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "dokument",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "DOCUMENT",
			Description: nil,
			SubclassOf:  nil,
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
			Name: []ClassName{
				{
					Name: "vocabulary",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "slovar",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
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
			Name: []ClassName{
				{
					Name: "language",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "jezik",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "LANGUAGE",
			Description: nil,
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
