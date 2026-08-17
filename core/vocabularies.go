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
			}, {
				Value: "inglês",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "esloveno",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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

	documents = append(documents, &Language{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "Portuguese",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "portugalščina",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "português",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"pt-PT"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "litro",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The litre volume unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota liter za prostornino.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de volume litro.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "quilograma por quilograma",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The kilogram per kilogram ratio unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota kilogram na kilogram za razmerje.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de razão quilograma por quilograma.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "quilograma",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The kilogram mass unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota kilogram za maso.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de massa quilograma.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "quilograma por metro cúbico",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The kilogram per cubic metre density unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota kilogram na kubični meter za gostoto.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de densidade quilograma por metro cúbico.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "metro",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The metre length unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota meter za dolžino.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de comprimento metro.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "metro quadrado",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The square metre area unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota kvadratni meter za površino.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de área metro quadrado.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "metro por segundo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The metre per second velocity unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota meter na sekundo za hitrost.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de velocidade metro por segundo.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "volt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The volt electric potential unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota volt za električno napetost.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de potencial elétrico volt.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "watt",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The watt power unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota vat za moč.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de potência watt.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "pascal",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The pascal pressure unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota pascal za tlak.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de pressão pascal.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "coulomb",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The coulomb electric charge unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota coulomb za električni naboj.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de carga elétrica coulomb.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "joule",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The joule energy unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota džul za energijo.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de energia joule.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "grau Celsius",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The Celsius temperature unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota stopinja Celzija za temperaturo.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de temperatura Celsius.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "radiano",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The radian angle unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota radian za kot.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de ângulo radiano.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "hertz",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The hertz frequency unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota herc za frekvenco.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de frequência hertz.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "dólar",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The dollar currency unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota dolar za valuto.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade monetária dólar.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "byte",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The byte data size unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota bajt za velikost podatkov.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de tamanho de dados byte.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "pixel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The pixel digital image measurement unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota piksel za merjenje digitalnih slik.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de medição de imagem digital pixel.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "segundo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The second time unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota sekunda za čas.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de tempo segundo.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "decibel",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>The decibel sound intensity unit.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Enota decibel za jakost zvoka.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>A unidade de intensidade sonora decibel.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "texto simples",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "texto",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "identificador",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "quantidade",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "intervalo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "tempo",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "período",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "ligação",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "ficheiro",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "referência",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "etiqueta",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "nenhum",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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
			}, {
				Value: "desconhecido",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
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

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "create",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "ustvarjanje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "criação",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"ACTION_CREATE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_CREATE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "read",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "branje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "leitura",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"ACTION_READ"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_READ"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "update",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "posodabljanje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "atualização",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"ACTION_UPDATE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_UPDATE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "delete",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "brisanje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "eliminação",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"ACTION_DELETE"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_DELETE"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "historic read",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "zgodovinsko branje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "leitura histórica",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>Reading historical versions.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Branje zgodovinskih verzij.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>Leitura de versões históricas.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Code: []Identifier{"ACTION_READ_HISTORIC"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_READ_HISTORIC"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "permissions update",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "posodabljanje dovoljenj",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "atualização de permissões",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: []RawHTMLWithLanguage{{
				Value: "<p>Updating permissions.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "<p>Posodabljanje dovoljenj.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "<p>Atualização de permissões.</p>",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Code: []Identifier{"ACTION_UPDATE_PERMISSIONS"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_UPDATE_PERMISSIONS"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	documents = append(documents, &PermissionAction{
		VocabularyFields: VocabularyFields{
			Name: []StringWithLanguage{{
				Value: "bulk read",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "en-GB"},
				}},
			}, {
				Value: "množično branje",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "sl-SI"},
				}},
			}, {
				Value: "leitura em massa",
				InLanguage: []Ref{{
					ID: []string{Namespace, "LANGUAGE", "pt-PT"},
				}},
			}},
			Description: nil,
			Code:        []Identifier{"ACTION_READ_BULK"},
		},
		DocumentFields: DocumentFields{
			ID: []string{Namespace, "PERMISSION_ACTIONS", "ACTION_READ_BULK"},
			InstanceOf: []Ref{{
				ID: []string{Namespace, "PERMISSION_ACTIONS"},
			}},
		},
	})

	return documents, nil
}
