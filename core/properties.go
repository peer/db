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
			ShortName:     nil,
			Mnemonic:      "SUBENTITY_OF",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:   nil,
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
			ShortName:   nil,
			Mnemonic:    "SUBCLASS_OF",
			Description: nil,
			Instruction: nil,
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
			ShortName:   nil,
			Mnemonic:    "SUBPROPERTY_OF",
			Description: nil,
			Instruction: nil,
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
			ShortName:     nil,
			Mnemonic:      "DISTINCT_FROM",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "NAMING",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:   nil,
			Mnemonic:    "NAME",
			Description: nil,
			Instruction: nil,
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
			ShortName:   nil,
			Mnemonic:    "SHORT_NAME",
			Description: nil,
			Instruction: nil,
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
			Name: []PropertyName{
				{
					Name: "title",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "naslov",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "TITLE",
			Description: nil,
			Instruction: nil,
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
			ShortName:     nil,
			Mnemonic:      "DESCRIPTION",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "INSTRUCTION",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "MNEMONIC",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "IN_LANGUAGE",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "VARIANT",
			Description:   nil,
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
			ShortName:     nil,
			Mnemonic:      "DEFAULT_VARIANT",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			ShortName:     nil,
			Mnemonic:      "SELECTED_VARIANT",
			Description:   nil,
			Instruction:   nil,
			SubpropertyOf: nil,
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
			Name: []PropertyName{
				{
					Name: "code",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "en-GB"},
					}},
				},
				{
					Name: "koda",
					InLanguage: []Ref{{
						ID: []string{Namespace, "LANGUAGE", "sl-SI"},
					}},
				},
			},
			ShortName:   nil,
			Mnemonic:    "CODE",
			Description: nil,
			Instruction: nil,
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

	return documents, nil
}
