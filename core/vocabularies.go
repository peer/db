package core

import (
	"gitlab.com/tozd/go/errors"
)

// Vocabularies returns core vocabularies.
func Vocabularies() ([]any, errors.E) { //nolint:maintidx
	documents := []any{} //nolint:prealloc

	documents = append(documents, &Language{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "English",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "angleščina",
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
			Name: []StringWithLanguage{{
				Value: "Slovenian",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "slovenščina",
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
			Name: []StringWithLanguage{{
				Value: "litre",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "liter",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The litre volume unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota liter za prostornino.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"l"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "l"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "kilogram per kilogram",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kilogram na kilogram",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The kilogram per kilogram ratio unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota kilogram na kilogram za razmerje.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"kg/kg"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "kg/kg"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "kilogram",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kilogram",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The kilogram mass unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota kilogram za maso.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"kg"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "kg"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "kilogram per cubic metre",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kilogram na kubični meter",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The kilogram per cubic metre density unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota kilogram na kubični meter za gostoto.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"kg/m³"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "kg/m³"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "metre",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "meter",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The metre length unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota meter za dolžino.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"m"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "m"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "square metre",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "kvadratni meter",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The square metre area unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota kvadratni meter za površino.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"m²"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "m²"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "metre per second",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "meter na sekundo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The metre per second velocity unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota meter na sekundo za hitrost.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"m/s"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "m/s"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "volt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "volt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The volt electric potential unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota volt za električno napetost.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"V"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "V"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "watt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "vat",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The watt power unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota vat za moč.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"W"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "W"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "pascal",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "pascal",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The pascal pressure unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota pascal za tlak.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"Pa"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "Pa"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "coulomb",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "coulomb",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The coulomb electric charge unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota coulomb za električni naboj.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"C"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "C"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "joule",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "džul",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The joule energy unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota džul za energijo.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"J"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "J"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "degree Celsius",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "stopinja Celzija",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The Celsius temperature unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota stopinja Celzija za temperaturo.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"°C"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "°C"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "radian",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "radian",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The radian angle unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota radian za kot.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"rad"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "rad"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "hertz",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "herc",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The hertz frequency unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota herc za frekvenco.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"Hz"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "Hz"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "dollar",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "dolar",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The dollar currency unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota dolar za valuto.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"$"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "$"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "byte",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "bajt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The byte data size unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota bajt za velikost podatkov.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"B"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "B"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "pixel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "piksel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The pixel digital image measurement unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota piksel za merjenje digitalnih slik.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"px"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "px"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "second",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "sekunda",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The second time unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota sekunda za čas.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"s"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "s"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &Unit{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "decibel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "decibel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "The decibel sound intensity unit.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "Enota decibel za jakost zvoka.",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Code: []Identifier{"dB"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "UNIT", "dB"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "UNIT"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "plain text",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "enostavno besedilo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"STRING"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "STRING"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "text",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "besedilo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"HTML"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "HTML"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "identifier",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "identifikator",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"IDENTIFIER"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "IDENTIFIER"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "amount",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "količina",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"AMOUNT"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "AMOUNT"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "interval",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "interval",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"AMOUNT_INTERVAL"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "AMOUNT_INTERVAL"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "time",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "čas",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"TIME"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "TIME"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "period",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "obdobje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"TIME_INTERVAL"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "TIME_INTERVAL"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "link",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "povezava",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"LINK"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "LINK"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "file",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "datoteka",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"FILE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "FILE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "reference",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "referenca",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"REFERENCE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "REFERENCE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "label",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "oznaka",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"HAS"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "HAS"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "none",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "neobstoječa vrednost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"NONE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "NONE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	documents = append(documents, &ValueType{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "unknown",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "neznana vrednost",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"UNKNOWN"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "VALUE_TYPE", "UNKNOWN"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "VALUE_TYPE"},
			}},
		},
	})

	return documents, nil
}
