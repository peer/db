package core

import (
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

// Vocabularies returns core vocabularies.
func Vocabularies(_ zerolog.Logger) ([]any, errors.E) {
	documents := []any{}

	documents = append(documents, &Language{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name: "English",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "angleščina",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"en-GB"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE", "en-GB"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "LANGUAGE"},
			}},
		},
	})

	documents = append(documents, &Language{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name: "Slovenian",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Name: "slovenščina",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"sl-SI"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE", "sl-SI"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "LANGUAGE"},
			}},
		},
	})

	return documents, nil
}
