package core

import (
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

// Vocabularies returns core vocabularies.
func Vocabularies(_ zerolog.Logger) ([]any, errors.E) { //nolint:maintidx
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
			Description: nil,
			Code:        []Identifier{"en-GB"},
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
			Description: nil,
			Code:        []Identifier{"sl-SI"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE", "sl-SI"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "LANGUAGE"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "litre",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "liter",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The litre volume unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota liter za prostornino.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"l"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "l"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "kilogram per kilogram",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "kilogram na kilogram",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The kilogram per kilogram ratio unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota kilogram na kilogram za razmerje.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"kg/kg"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "kg/kg"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "kilogram",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "kilogram",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The kilogram mass unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota kilogram za maso.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"kg"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "kg"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "kilogram per cubic metre",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "kilogram na kubični meter",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The kilogram per cubic metre density unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota kilogram na kubični meter za gostoto.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"kg/m³"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "kg/m³"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "metre",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "meter",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The metre length unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota meter za dolžino.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"m"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "m"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "square metre",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "kvadratni meter",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The square metre area unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota kvadratni meter za površino.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"m²"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "m²"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "metre per second",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "meter na sekundo",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The metre per second velocity unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota meter na sekundo za hitrost.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"m/s"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "m/s"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "volt",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "volt",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The volt electric potential unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota volt za električno napetost.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"V"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "V"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "watt",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "vat",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The watt power unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota vat za moč.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"W"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "W"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "pascal",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "pascal",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The pascal pressure unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota pascal za tlak.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"Pa"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "Pa"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "coulomb",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "coulomb",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The coulomb electric charge unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota coulomb za električni naboj.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"C"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "C"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "joule",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "džul",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The joule energy unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota džul za energijo.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"J"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "J"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "degree Celsius",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "stopinja Celzija",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The Celsius temperature unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota stopinja Celzija za temperaturo.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"°C"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "°C"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "radian",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "radian",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The radian angle unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota radian za kot.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"rad"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "rad"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "hertz",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "herc",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The hertz frequency unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota herc za frekvenco.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"Hz"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "Hz"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "dollar",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "dolar",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The dollar currency unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota dolar za valuto.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"$"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "$"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "byte",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "bajt",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The byte data size unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota bajt za velikost podatkov.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"B"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "B"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "pixel",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "piksel",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The pixel digital image measurement unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota piksel za merjenje digitalnih slik.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"px"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "px"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "second",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "sekunda",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The second time unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota sekunda za čas.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"s"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "s"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []VocabularyName{{
				Name:       "decibel",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Name:       "decibel",
				InLanguage: []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Description: []VocabularyDescription{{
				Description: "The decibel sound intensity unit.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "en-GB"}}},
			}, {
				Description: "Enota decibel za jakost zvoka.",
				InLanguage:  []Ref{{ID: []string{Namespace, "LANGUAGE", "sl-SI"}}},
			}},
			Code: []Identifier{"dB"},
		},
		DocumentFields: DocumentFields{
			ID:         []string{Namespace, "UNIT", "dB"},
			InstanceOf: []Ref{{ID: []string{Namespace, "UNIT"}}},
		},
	})

	return documents, nil
}
