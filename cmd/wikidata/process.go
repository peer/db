package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/identifier"
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedError              = errors.Base("not supported")
	notSupportedDataValueTypeError = errors.BaseWrap(notSupportedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(notSupportedError, "not supported data type")

	// A set of properties which are ignored when referencing items.
	// We want properties to only reference other properties.
	ignoredPropertiesReferencingItems = map[string]bool{
		// Wikidata item of this property.
		// TODO: Handle somehow? Merge into property?
		"P1629": true,
		// Wikidata property example.
		"P1855": true,
		// applicable 'stated in' value.
		"P9073": true,
		// country.
		"P17": true,
		// issued by.
		"P2378": true,
		// creator.
		"P170": true,
		// maintained by.
		"P126": true,
		// maintained by WikiProject.
		"P6104": true,
		// has quality.
		"P1552": true,
		// type of unit for this property.
		// TODO: Handle somehow? Convert to PeerDB unit?
		"P2876": true,
		// used by.
		"P1535": true,
		// uses.
		"P2283": true,
		// part of.
		"P361": true,
		// applies to jurisdiction.
		"P1001": true,
		// sport.
		"P641": true,
		// operator.
		"P137": true,
		// standards body.
		"P1462": true,
		// language of work or name.
		"P407": true,
		// different from.
		// TODO: Handle somehow? Redirect to property instead?
		"P1889": true,
		// member of.
		"P463": true,
		// publisher.
		"P123": true,
		// on focus list of Wikimedia project.
		"P5008": true,
		// subject has role.
		"P2868": true,
		// facet of.
		"P1269": true,
	}

	// Some items we convert to properties. These properties are referencing
	// items, but we want to reference converted properties instead.
	propertiesReferencingConvertedItems = map[string]bool{
		// property constraint.
		"P2302": true,
		// instance of.
		"P31": true,
		// inverse label item.
		"P7087": true,
		// living people protection class.
		"P8274": true,
		// stability of property value.
		"P2668": true,
		// expected completeness.
		"P2429": true,
	}

	// Some items we skip. These properties are referencing items we skip.
	propertiesReferencingSkippedItems = map[string]bool{
		// property usage tracking category.
		"P2875": true,
		// category for value not in Wikidata.
		"P3713": true,
		// category for value same as Wikidata.
		"P3734": true,
		// category for value different from Wikidata.
		"P3709": true,
		// has list.
		"P2354": true,
		// corresponding template.
		"P2667": true,
		// topic's main template.
		"P1424": true,
	}

	// Properties for which we allow quantities.
	propertiesWithQuantities = map[string]bool{
		// number of records.
		"P4876": true,
	}
)

func getItemID(id string) search.Identifier {
	return search.Identifier(identifier.FromUUID(uuid.NewSHA1(NameSpaceWikidata, []byte(id))))
}

func getStandardPropertyClaimID(id, mnemonic string, i int) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					NameSpaceWikidata,
					[]byte(id),
				),
				[]byte(mnemonic),
			),
			[]byte(strconv.Itoa(i)),
		),
	))
}

func getPropertyClaimIDFromHash(id, prop, statementID, hash string) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					uuid.NewSHA1(
						NameSpaceWikidata,
						[]byte(id),
					),
					[]byte(prop),
				),
				[]byte(statementID),
			),
			[]byte(hash),
		),
	))
}

func getPropertyClaimID(id, prop, statementID, namespace string, i int) search.Identifier {
	return search.Identifier(identifier.FromUUID(
		uuid.NewSHA1(
			uuid.NewSHA1(
				uuid.NewSHA1(
					uuid.NewSHA1(
						uuid.NewSHA1(
							NameSpaceWikidata,
							[]byte(id),
						),
						[]byte(prop),
					),
					[]byte(statementID),
				),
				[]byte(namespace),
			),
			[]byte(strconv.Itoa(i)),
		),
	))
}

func deduplicate(data []string) []string {
	duplicates := make(map[string]bool, len(data))
	res := []string{}
	for _, d := range data {
		if !duplicates[d] {
			res = append(res, d)
			duplicates[d] = true
		}
	}
	return res
}

func getEnglishValues(values map[string]mediawiki.LanguageValue) []string {
	res := []string{}
	// Deterministic iteration over a map.
	languages := make([]string, 0, len(values))
	for language := range values {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if language == "en" || strings.HasPrefix(language, "en-") {
			res = append(res, values[language].Value)
		}
	}
	return deduplicate(res)
}

func getEnglishValuesSlice(values map[string][]mediawiki.LanguageValue) []string {
	res := []string{}
	// Deterministic iteration over a map.
	languages := make([]string, 0, len(values))
	for language := range values {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if language == "en" || strings.HasPrefix(language, "en-") {
			for _, v := range values[language] {
				res = append(res, v.Value)
			}
		}
	}
	return deduplicate(res)
}

// We have more precise claim types so this is not very precise (e.g., quantity is used
// both for amount and durations). It is good for the first pass but later on we augment
// claims about claim types using statistics on how are properties really used.
// TODO: Really collect statistics and augment claims about claim types.
// TODO: Determine automatically mnemonics.
func getPropertyClaimType(dataType mediawiki.DataType) string {
	switch dataType {
	case mediawiki.WikiBaseItem:
		return "ITEM_CLAIM_TYPE"
	case mediawiki.ExternalID:
		return "IDENTIFIER_CLAIM_TYPE"
	case mediawiki.String:
		return "STRING_CLAIM_TYPE"
	case mediawiki.Quantity:
		return "AMOUNT_CLAIM_TYPE"
	case mediawiki.Time:
		return "TIME_CLAIM_TYPE"
	case mediawiki.GlobeCoordinate:
		// Not supported.
		return ""
	case mediawiki.CommonsMedia:
		return "FILE_CLAIM_TYPE"
	case mediawiki.MonolingualText:
		return "TEXT_CLAIM_TYPE"
	case mediawiki.URL:
		return "REFERENCE_CLAIM_TYPE"
	case mediawiki.GeoShape:
		// Not supported.
		return ""
	case mediawiki.WikiBaseLexeme:
		// Not supported.
		return ""
	case mediawiki.WikiBaseSense:
		// Not supported.
		return ""
	case mediawiki.WikiBaseProperty:
		return "PROPERTY_CLAIM_TYPE"
	case mediawiki.Math:
		// Not supported.
		return ""
	case mediawiki.MusicalNotation:
		// Not supported.
		return ""
	case mediawiki.WikiBaseForm:
		// Not supported.
		return ""
	case mediawiki.TabularData:
		// Not supported.
		return ""
	}
	panic(errors.Errorf(`invalid data type: %d`, dataType))
}

func getConfidence(entityID, prop, statementID string, rank mediawiki.StatementRank) search.Confidence {
	switch rank {
	case mediawiki.Preferred:
		return 1.0
	case mediawiki.Normal:
		return 0.5
	case mediawiki.Deprecated:
		return -1.0
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// It does not return a valid reference: name is missing.
func getPropertyReference(prop string) search.PropertyReference {
	return search.PropertyReference{
		ID:    getItemID(prop),
		Score: 0.0,
	}
}

// It does not return a valid reference: name is missing.
func getItemReference(item string) search.ItemReference {
	return search.ItemReference{
		ID:    getItemID(item),
		Score: 0.0,
	}
}

func processSnak(entityID, prop, statementID, namespace string, confidence search.Confidence, snak mediawiki.Snak) (interface{}, errors.E) {
	var id search.Identifier
	if snak.Hash != "" {
		id = getPropertyClaimIDFromHash(entityID, prop, statementID, snak.Hash)
	} else {
		id = getPropertyClaimID(entityID, prop, statementID, namespace, 0)
	}

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return search.UnknownValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getPropertyReference(prop),
		}, nil
	case mediawiki.NoValue:
		return search.NoValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getPropertyReference(prop),
		}, nil
	}

	if snak.DataValue == nil {
		return nil, errors.Errorf(`nil data value`)
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch snak.DataType {
		case mediawiki.ExternalID:
			return search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:       getPropertyReference(prop),
				Identifier: string(value),
			}, nil
		case mediawiki.String:
			return search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:   getPropertyReference(prop),
				String: string(value),
			}, nil
		case mediawiki.CommonsMedia:
			return nil, errors.Errorf("%w: TODO", notSupportedError)
		case mediawiki.URL:
			return search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getPropertyReference(prop),
				IRI:  string(value),
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for StringValue: %d", snak.DataType)
		}
	case mediawiki.WikiBaseEntityIDValue:
		switch snak.DataType {
		case mediawiki.WikiBaseItem:
			if value.Type != mediawiki.ItemType {
				return nil, errors.Errorf("WikiBaseItem data type, but WikiBaseEntityIDValue has type %d, not ItemType", value.Type)
			}
			if ignoredPropertiesReferencingItems[prop] {
				// A special case for ignored references.
				return nil, errors.Errorf("%w: an ignored reference to an item: %s", notSupportedError, value.ID)
			} else if propertiesReferencingConvertedItems[prop] {
				// A special case for items we convert to properties.
				// TODO: Remember which items we have to convert to a property.
				return search.PropertyClaim{
					CoreClaim: search.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:  getPropertyReference(prop),
					Other: getPropertyReference(value.ID),
				}, nil
			} else if propertiesReferencingSkippedItems[prop] {
				// A special case for items we skip.
				// TODO: Remember which items we have to skip.
				return nil, errors.Errorf("%w: an item we skip: %s", notSupportedError, value.ID)
			}
			return search.ItemClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getPropertyReference(prop),
				Item: getItemReference(value.ID),
			}, nil
		case mediawiki.WikiBaseProperty:
			if value.Type != mediawiki.PropertyType {
				return nil, errors.Errorf("WikiBaseProperty data type, but WikiBaseEntityIDValue has type %d, not PropertyType", value.Type)
			}
			return search.PropertyClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:  getPropertyReference(prop),
				Other: getPropertyReference(value.ID),
			}, nil
		case mediawiki.WikiBaseLexeme:
			return nil, errors.Errorf("%w: WikiBaseLexeme", notSupportedDataTypeError)
		case mediawiki.WikiBaseSense:
			return nil, errors.Errorf("%w: WikiBaseSense", notSupportedDataTypeError)
		case mediawiki.WikiBaseForm:
			return nil, errors.Errorf("%w: WikiBaseForm", notSupportedDataTypeError)
		default:
			return nil, errors.Errorf("unexpected data type for WikiBaseEntityIDValue: %d", snak.DataType)
		}
	case mediawiki.GlobeCoordinateValue:
		return nil, errors.Errorf("%w: GlobeCoordinateValue", notSupportedDataValueTypeError)
	case mediawiki.MonolingualTextValue:
		switch snak.DataType {
		case mediawiki.MonolingualText:
			return search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:  getPropertyReference(prop),
				Plain: search.TranslatablePlainString{value.Language: value.Text},
				HTML:  search.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for MonolingualTextValue: %d", snak.DataType)
		}
	case mediawiki.QuantityValue:
		switch snak.DataType {
		case mediawiki.Quantity:
			if propertiesWithQuantities[prop] {
				return nil, errors.Errorf("%w: TODO", notSupportedError)
			}
			return nil, errors.New("TODO Quantity")
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType {
		case mediawiki.Time:
			return nil, errors.New("TODO Time")
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", snak.DataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func processItem(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
	return nil
}

func processProperty(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English content for now.
	if len(englishLabels) == 0 {
		// But properties should all have English label, so we warn here.
		fmt.Fprintf(os.Stderr, "property %s is missing a label in English\n", entity.ID)
		return nil
	}

	id := getItemID(entity.ID)

	// We simply use the first label we have.
	name := englishLabels[0]
	englishLabels = englishLabels[1:]

	// TODO: Set mnemonic if name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere.
	property := search.Property{
		CoreDocument: search.CoreDocument{
			ID: id,
			Name: search.Name{
				"en": name,
			},
			Score: 0.0,
		},
		Active: &search.PropertyClaimTypes{
			MetaClaimTypes: search.MetaClaimTypes{
				Identifier: search.IdentifierClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_ID", 0),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
							Confidence: 1.0,
						},
						Prop: search.GetStandardPropertyReference("WIKIDATA_PROPERTY_PAGE"),
						IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/Property:%s", entity.ID),
					},
				},
			},
		},
	}

	claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
	if claimTypeMnemonic != "" {
		property.Active.Property = append(property.Active.Property, search.PropertyClaim{
			CoreClaim: search.CoreClaim{
				ID: getStandardPropertyClaimID(entity.ID, claimTypeMnemonic, 0),
				// We have low confidence in this claim. Later on we augment it using statistics
				// on how are properties really used.
				Confidence: 0.0,
			},
			Prop:  search.GetStandardPropertyReference("IS"),
			Other: search.GetStandardPropertyReference(claimTypeMnemonic),
		})
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	if len(englishAliases) > 0 {
		englishLabels = append(englishLabels, englishAliases...)
		englishLabels = deduplicate(englishLabels)
	}

	if len(englishLabels) > 0 {
		property.CoreDocument.OtherNames = search.OtherNames{
			"en": englishLabels,
		}
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	if len(englishDescriptions) > 0 {
		simple := &property.Active.SimpleClaimTypes
		for i, description := range englishDescriptions {
			simple.Text = append(simple.Text, search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         getStandardPropertyClaimID(entity.ID, "DESCRIPTION", i),
					Confidence: 1.0,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				Plain: search.TranslatablePlainString{
					"en": description,
				},
				HTML: search.TranslatableHTMLString{
					"en": html.EscapeString(description),
				},
			})
		}
	}

	// Deterministic iteration over a map.
	props := make([]string, 0, len(entity.Claims))
	for prop := range entity.Claims {
		props = append(props, prop)
	}
	sort.Strings(props)

	for _, prop := range props {
		statements := entity.Claims[prop]
		for i, statement := range statements {
			if statement.ID == "" {
				// All statements should have an ID, so we warn here.
				fmt.Fprintf(os.Stderr, "statement %d of property %s for entity %s is missing an ID\n", i, prop, entity.ID)
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claim, err := processSnak(entity.ID, prop, statement.ID, "", confidence, statement.MainSnak)
			if errors.Is(err, notSupportedError) {
				// We know what we do not support, ignore.
				continue
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s has mainsnak that cannot be processed: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
			err = property.Add(claim)
			if err != nil {
				fmt.Fprintf(os.Stderr, "statement %s of property %s for entity %s cannot be added: %s\n", statement.ID, prop, entity.ID, err.Error())
				continue
			}
		}
	}

	return saveProperty(config, property)
}

func saveProperty(config *Config, property search.Property) errors.E {
	path := filepath.Join(config.OutputDir, "properties", fmt.Sprintf("%s.json", property.ID))
	file, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(property)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func processEntity(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
	switch entity.Type {
	case mediawiki.Item:
		return processItem(ctx, config, entity)
	case mediawiki.Property:
		return processProperty(ctx, config, entity)
	default:
		return errors.Errorf(`entity %s has invalid type: %d`, entity.ID, entity.Type)
	}
}
