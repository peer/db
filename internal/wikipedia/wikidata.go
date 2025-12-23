package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	WikidataReference                 = "Wikidata"
	WikimediaCommonsEntityReference   = "CommonsEntity"
	WikimediaCommonsFileReference     = "CommonsFile"
	WikipediaCategoryReference        = "WikipediaCategory"
	WikipediaTemplateReference        = "WikipediaTemplate"
	WikimediaCommonsCategoryReference = "CommonsCategory"
	WikimediaCommonsTemplateReference = "CommonsTemplate"
)

//nolint:gochecknoglobals
var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	errNotSupportedDataValueType = errors.BaseWrap(ErrSilentSkipped, "not supported data value type")
	errNotSupportedDataType      = errors.BaseWrap(ErrSilentSkipped, "not supported data type")
	ErrNotFound                  = errors.Base("not found")

	// Besides main namespace we allow also templates, modules, and categories.
	nonMainNamespaces = []string{
		"User:",
		"Wikipedia:",
		"File:",
		"MediaWiki:",
		"Help:",
		"Portal:",
		"Draft:",
		"TimedText:",
	}

	dataTypeToClaimTypeMap = map[mediawiki.DataType]string{
		mediawiki.WikiBaseItem: "RELATION_CLAIM_TYPE",
		mediawiki.ExternalID:   "IDENTIFIER_CLAIM_TYPE",
		mediawiki.String:       "STRING_CLAIM_TYPE",
		mediawiki.Quantity:     "AMOUNT_CLAIM_TYPE",
		mediawiki.Time:         "TIME_CLAIM_TYPE",
		// Not supported.
		mediawiki.GlobeCoordinate: "",
		mediawiki.CommonsMedia:    "FILE_CLAIM_TYPE",
		mediawiki.MonolingualText: "TEXT_CLAIM_TYPE",
		mediawiki.URL:             "REFERENCE_CLAIM_TYPE",
		// Not supported.
		mediawiki.GeoShape: "",
		// Not supported.
		mediawiki.WikiBaseLexeme: "",
		// Not supported.
		mediawiki.WikiBaseSense:    "",
		mediawiki.WikiBaseProperty: "RELATION_CLAIM_TYPE",
		// Not supported.
		mediawiki.Math: "",
		// Not supported.
		mediawiki.MusicalNotation: "",
		// Not supported.
		mediawiki.WikiBaseForm: "",
		// Not supported.
		mediawiki.TabularData: "",
	}

	claimTypeToDataTypesMap = map[string][]mediawiki.DataType{}
)

func init() { //nolint:gochecknoinits
	for dataType, claimType := range dataTypeToClaimTypeMap {
		// We skip if not supported.
		if claimType == "" {
			continue
		}
		claimTypeToDataTypesMap[claimType] = append(claimTypeToDataTypesMap[claimType], dataType)
	}
}

func GetWikidataDocumentID(id string) identifier.Identifier {
	return document.GetID(NameSpaceWikidata, id)
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

// We have more precise claim types so this is not very precise. It is good for the
// first pass but later on we augment claims about claim types using statistics on
// how are properties really used.
// TODO: Really collect statistics and augment claims about claim types.
// TODO: Determine automatically mnemonics.
func getPropertyClaimType(dataType mediawiki.DataType) string {
	claimType, ok := dataTypeToClaimTypeMap[dataType]
	if !ok {
		panic(errors.Errorf(`invalid data type: %d`, dataType))
	}
	return claimType
}

func getConfidence(entityID, prop, statementID string, rank mediawiki.StatementRank) document.Confidence {
	switch rank {
	case mediawiki.Preferred:
		return document.HighConfidence
	case mediawiki.Normal:
		return document.MediumConfidence
	case mediawiki.Deprecated:
		return document.NoConfidence
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// getDocumentReference does not return a valid reference, but it encodes original
// ID into the _temp field to be resolved later. It panics for unsupported IDs.
func getDocumentReference(id, source string) document.Reference {
	if strings.HasPrefix(id, "M") {
		return document.Reference{
			ID:        nil,
			Temporary: []string{WikimediaCommonsEntityReference, id},
		}
	} else if strings.HasPrefix(id, "P") || strings.HasPrefix(id, "Q") {
		return document.Reference{
			ID:        nil,
			Temporary: []string{WikidataReference, id},
		}
	} else if strings.HasPrefix(id, "Category:") {
		switch source {
		case "ENGLISH_WIKIPEDIA":
			return document.Reference{
				ID:        nil,
				Temporary: []string{WikipediaCategoryReference, id},
			}
		case "WIKIMEDIA_COMMONS":
			return document.Reference{
				ID:        nil,
				Temporary: []string{WikimediaCommonsCategoryReference, id},
			}
		}
	} else if strings.HasPrefix(id, "Template:") || strings.HasPrefix(id, "Module:") {
		switch source {
		case "ENGLISH_WIKIPEDIA":
			return document.Reference{
				ID:        nil,
				Temporary: []string{WikipediaTemplateReference, id},
			}
		case "WIKIMEDIA_COMMONS":
			return document.Reference{
				ID:        nil,
				Temporary: []string{WikimediaCommonsTemplateReference, id},
			}
		}
	} else if strings.HasPrefix(id, "File:") {
		return document.Reference{
			ID:        nil,
			Temporary: []string{WikimediaCommonsFileReference, id},
		}
	}

	panic(errors.Errorf("unsupported ID for source \"%s\": %s", source, id))
}

func getDocumentFromByProp(
	ctx context.Context, s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	index string, esClient *elastic.Client, property, id string,
) (*document.D, store.Version, errors.E) {
	searchResult, err := esClient.Search(index).FetchSource(false).AllowPartialSearchResults(false).
		Query(elastic.NewNestedQuery("claims.id",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.id.prop.id", document.GetCorePropertyID(property).String()),
				elastic.NewTermQuery("claims.id.id", id),
			),
		)).Do(ctx)
	if err != nil {
		// Caller should add details to the error.
		return nil, store.Version{}, errors.WithStack(err)
	}

	// There might be multiple hits because IDs are not unique (we remove zeroes and do a case insensitive matching).
	for _, hit := range searchResult.Hits.Hits {
		doc, version, errE := getDocumentFromByID(ctx, s, identifier.String(hit.Id))
		if errE != nil {
			// Caller should add details to the error.
			return nil, store.Version{}, errE
		}

		found := false
		for _, claim := range doc.Get(document.GetCorePropertyID(property)) {
			if c, ok := claim.(*document.IdentifierClaim); ok && c.Value == id {
				found = true
				break
			}
		}

		// If this hit is not precisely for this name, we continue with the next one.
		if !found {
			continue
		}

		return doc, version, nil
	}

	// Caller should add details to the error.
	return nil, store.Version{}, errors.WithStack(ErrNotFound)
}

func getDocumentFromByID(
	ctx context.Context,
	s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	id identifier.Identifier,
) (*document.D, store.Version, errors.E) {
	data, _, version, errE := s.GetLatest(ctx, id)
	if errors.Is(errE, store.ErrValueNotFound) {
		// Caller should add details to the error.
		return nil, store.Version{}, errors.WithStack(ErrNotFound)
	} else if errE != nil {
		return nil, store.Version{}, errE
	}

	var doc document.D
	errE = x.UnmarshalWithoutUnknownFields(data, &doc)
	if errE != nil {
		// Caller should add details to the error.
		return nil, store.Version{}, errE
	}

	return &doc, version, nil
}

func GetWikidataItem(
	ctx context.Context,
	s *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	index string, esClient *elastic.Client, id string,
) (*document.D, store.Version, errors.E) {
	doc, version, errE := getDocumentFromByProp(ctx, s, index, esClient, "WIKIDATA_ITEM_ID", id)
	if errE != nil {
		errors.Details(errE)["entity"] = id
		return nil, store.Version{}, errE
	}

	return doc, version, nil
}

func clampConfidence(c document.Score) document.Score {
	if c < 0 {
		// max(c, document.HighNegationConfidence).
		if c < document.HighNegationConfidence {
			return document.HighNegationConfidence
		}
		return c
	}
	// min(c, document.HighConfidence).
	if c < document.HighConfidence {
		return c
	}
	return document.HighConfidence
}

func resolveDataTypeFromPropertyDocument(doc *document.D, prop string, valueType *mediawiki.WikiBaseEntityType) (mediawiki.DataType, errors.E) {
	for _, claim := range doc.Get(document.GetCorePropertyID("TYPE")) {
		if c, ok := claim.(*document.RelationClaim); ok {
			for claimType, dataTypes := range claimTypeToDataTypesMap {
				if c.To.ID != nil && *c.To.ID == document.GetCorePropertyID(claimType) {
					if len(dataTypes) == 1 {
						return dataTypes[0], nil
					} else if claimType == "RELATION_CLAIM_TYPE" && valueType != nil {
						switch *valueType {
						case mediawiki.PropertyType:
							return mediawiki.WikiBaseProperty, nil
						case mediawiki.ItemType:
							return mediawiki.WikiBaseItem, nil
						case mediawiki.LexemeType, mediawiki.FormType, mediawiki.SenseType:
							fallthrough
						default:
							err := errors.Errorf("%w: not supported value type", errNotSupportedDataType)
							errors.Details(err)["prop"] = prop
							return 0, err
						}
					} else {
						err := errors.New("multiple data types, but not RELATION_CLAIM_TYPE")
						errors.Details(err)["prop"] = prop
						return 0, err
					}
				}
			}
		} else {
			err := errors.New("TYPE claim which is not relation claim")
			errors.Details(err)["prop"] = prop
			return 0, err
		}
	}
	err := errors.Errorf("%w: no suitable TYPE claim found", errNotSupportedDataType)
	errors.Details(err)["prop"] = prop
	return 0, err
}

func getDataTypeForProperty(
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	cache *es.Cache, prop string, valueType *mediawiki.WikiBaseEntityType,
) (mediawiki.DataType, errors.E) {
	id := GetWikidataDocumentID(prop)

	cachedDoc, ok := cache.Get(id)
	if ok {
		if cachedDoc == nil {
			err := errors.WithStack(ErrNotFound)
			errors.Details(err)["prop"] = prop
			return 0, err
		}
		return resolveDataTypeFromPropertyDocument(cachedDoc, prop, valueType)
	}

	doc, _, err := getDocumentFromByID(ctx, store, id)
	if errors.Is(err, ErrNotFound) {
		cache.Add(id, nil)
		errors.Details(err)["prop"] = prop
		return 0, err
	} else if err != nil {
		errors.Details(err)["prop"] = prop
		return 0, err
	}

	cache.Add(doc.ID, doc)

	return resolveDataTypeFromPropertyDocument(doc, prop, valueType)
}

func getWikiBaseEntityType(value interface{}) *mediawiki.WikiBaseEntityType {
	wikiBaseEntityValue, ok := value.(mediawiki.WikiBaseEntityIDValue)
	if !ok {
		return nil
	}
	return &wikiBaseEntityValue.Type
}

func processSnak( //nolint:ireturn,nolintlint,maintidx
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	cache *es.Cache, namespace uuid.UUID, prop string, idArgs []interface{}, confidence document.Confidence, snak mediawiki.Snak,
) ([]document.Claim, errors.E) {
	id := document.GetID(namespace, idArgs...)

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return []document.Claim{
			&document.UnknownValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop, ""),
			},
		}, nil
	case mediawiki.NoValue:
		return []document.Claim{
			&document.NoValueClaim{
				CoreClaim: document.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop, ""),
			},
		}, nil
	}

	if snak.DataValue == nil {
		return nil, errors.New("nil data value")
	}

	var dataType mediawiki.DataType
	if snak.DataType == nil {
		// Wikimedia Commons might not have the datatype field set, so we have to fetch it ourselves.
		// See: https://phabricator.wikimedia.org/T311977
		var err errors.E
		dataType, err = getDataTypeForProperty(ctx, store, cache, prop, getWikiBaseEntityType(snak.DataValue.Value))
		if err != nil {
			return nil, errors.WithMessagef(err, "unable to resolve data type for property with value %T", snak.DataValue.Value)
		}
	} else {
		dataType = *snak.DataType
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.ExternalID:
			return []document.Claim{
				&document.IdentifierClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:  getDocumentReference(prop, ""),
					Value: string(value),
				},
			}, nil
		case mediawiki.String:
			return []document.Claim{
				&document.StringClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:   getDocumentReference(prop, ""),
					String: string(value),
				},
			}, nil
		case mediawiki.CommonsMedia:
			// First we make sure we do not have underscores.
			title := strings.ReplaceAll(string(value), "_", " ")
			// The first letter has to be upper case.
			title = FirstUpperCase(title)
			title = "File:" + title

			args := append([]interface{}{}, idArgs...)
			args = append(args, "TYPE", 0, title, 0)
			claimID := document.GetID(namespace, args...)

			return []document.Claim{
				// An invalid claim we post-process later.
				&document.FileClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
						Meta: &document.ClaimTypes{
							Relation: document.RelationClaims{
								{
									CoreClaim: document.CoreClaim{
										ID:         claimID,
										Confidence: document.HighConfidence,
									},
									Prop: document.GetCorePropertyReference("TYPE"),
									To:   getDocumentReference(title, ""),
								},
							},
						},
					},
					Prop:      getDocumentReference(prop, ""),
					MediaType: "invalid/invalid",
					URL:       "https://xx.invalid",
					Preview:   nil,
				},
			}, nil
		case mediawiki.URL:
			return []document.Claim{
				&document.ReferenceClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					IRI:  string(value),
				},
			}, nil
		case mediawiki.GeoShape:
			return nil, errors.Errorf("%w: GeoShape", errNotSupportedDataType)
		case mediawiki.Math:
			return nil, errors.Errorf("%w: Math", errNotSupportedDataType)
		case mediawiki.MusicalNotation:
			return nil, errors.Errorf("%w: MusicalNotation", errNotSupportedDataType)
		case mediawiki.TabularData:
			return nil, errors.Errorf("%w: TabularData", errNotSupportedDataType)
		default:
			return nil, errors.Errorf("unexpected data type for StringValue: %d", dataType)
		}
	case mediawiki.WikiBaseEntityIDValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.WikiBaseItem:
			if value.Type != mediawiki.ItemType {
				return nil, errors.Errorf("WikiBaseItem data type, but WikiBaseEntityIDValue has type %d, not ItemType", value.Type)
			}
			return []document.Claim{
				&document.RelationClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					To:   getDocumentReference(value.ID, ""),
				},
			}, nil
		case mediawiki.WikiBaseProperty:
			if value.Type != mediawiki.PropertyType {
				return nil, errors.Errorf("WikiBaseProperty data type, but WikiBaseEntityIDValue has type %d, not PropertyType", value.Type)
			}
			return []document.Claim{
				&document.RelationClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					To:   getDocumentReference(value.ID, ""),
				},
			}, nil
		case mediawiki.WikiBaseLexeme:
			return nil, errors.Errorf("%w: WikiBaseLexeme", errNotSupportedDataType)
		case mediawiki.WikiBaseSense:
			return nil, errors.Errorf("%w: WikiBaseSense", errNotSupportedDataType)
		case mediawiki.WikiBaseForm:
			return nil, errors.Errorf("%w: WikiBaseForm", errNotSupportedDataType)
		default:
			return nil, errors.Errorf("unexpected data type for WikiBaseEntityIDValue: %d", dataType)
		}
	case mediawiki.GlobeCoordinateValue:
		return nil, errors.Errorf("%w: GlobeCoordinateValue", errNotSupportedDataValueType)
	case mediawiki.MonolingualTextValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.MonolingualText:
			if value.Language != "en" && !strings.HasPrefix(value.Language, "en-") {
				return nil, errors.WithStack(errors.BaseWrap(ErrSilentSkipped, "limited only to English"))
			}
			return []document.Claim{
				&document.TextClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop: getDocumentReference(prop, ""),
					HTML: document.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
				},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for MonolingualTextValue: %d", dataType)
		}
	case mediawiki.QuantityValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.Quantity:
			amount, exact := value.Amount.Float64()
			if !exact && math.IsInf(amount, 0) {
				return nil, errors.Errorf("amount cannot be represented by float64: %s", value.Amount.String())
			}
			var uncertaintyLower, uncertaintyUpper *float64
			if value.LowerBound != nil && value.UpperBound != nil {
				l, exact := value.LowerBound.Float64()
				if !exact && math.IsInf(l, 0) {
					return nil, errors.Errorf("lower bound cannot be represented by float64: %s", value.LowerBound.String())
				}
				uncertaintyLower = &l
				u, exact := value.UpperBound.Float64()
				if !exact && math.IsInf(u, 0) {
					return nil, errors.Errorf("upper bound cannot be represented by float64: %s", value.UpperBound.String())
				}
				uncertaintyUpper = &u
				if *uncertaintyLower > amount {
					return nil, errors.Errorf("lower bound %f cannot be larger than the amount %f", *uncertaintyLower, amount)
				}
				if *uncertaintyUpper < amount {
					return nil, errors.Errorf("upper bound %f cannot be smaller than the amount %f", *uncertaintyUpper, amount)
				}
			} else if value.LowerBound != nil || value.UpperBound != nil {
				return nil, errors.Errorf("both lower and upper bounds have to be provided, or none, not just one")
			}

			var unit document.AmountUnit
			var metaClaims *document.ClaimTypes
			if value.Unit == "1" {
				unit = document.AmountUnitNone
			} else {
				// For now we store the amount as-is and convert to the same unit later on
				// using the unit we store into meta claims.
				// TODO: Implement unit post-processing.
				unit = document.AmountUnitCustom
				args := append([]interface{}{}, idArgs...)
				args = append(args, "UNIT", 0)
				claimID := document.GetID(NameSpaceWikidata, args...)
				var unitID string
				if strings.HasPrefix(value.Unit, "http://www.wikidata.org/entity/") {
					unitID = strings.TrimPrefix(value.Unit, "http://www.wikidata.org/entity/")
				} else if strings.HasPrefix(value.Unit, "https://www.wikidata.org/wiki/") {
					unitID = strings.TrimPrefix(value.Unit, "https://www.wikidata.org/wiki/")
				} else {
					return nil, errors.Errorf("unsupported unit URL: %s", value.Unit)
				}
				metaClaims = &document.ClaimTypes{
					Relation: document.RelationClaims{
						{
							CoreClaim: document.CoreClaim{
								ID:         claimID,
								Confidence: document.HighConfidence,
							},
							Prop: document.GetCorePropertyReference("UNIT"),
							To:   getDocumentReference(unitID, ""),
						},
					},
				}
			}

			claims := []document.Claim{
				&document.AmountClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
						Meta:       metaClaims,
					},
					Prop:   getDocumentReference(prop, ""),
					Amount: amount,
					Unit:   unit,
				},
			}
			if uncertaintyLower != nil && uncertaintyUpper != nil {
				// We lower the confidence of the original claim.
				claims[0].(*document.AmountClaim).Confidence *= 0.9 //nolint:forcetypeassert,errcheck
				claims = append(
					claims,
					&document.AmountRangeClaim{
						CoreClaim: document.CoreClaim{
							ID: id,
							// We raise the confidence of the range claim.
							Confidence: clampConfidence(confidence * 1.1), //nolint:mnd
							Meta:       metaClaims,
						},
						Prop:  getDocumentReference(prop, ""),
						Lower: *uncertaintyLower,
						Upper: *uncertaintyUpper,
						Unit:  unit,
					},
				)
			}

			return claims, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", dataType)
		}
	case mediawiki.TimeValue:
		switch dataType { //nolint:exhaustive
		case mediawiki.Time:
			return []document.Claim{
				// TODO: Convert timestamps in Julian calendar to ones in Gregorian calendar.
				&document.TimeClaim{
					CoreClaim: document.CoreClaim{
						ID:         id,
						Confidence: confidence,
					},
					Prop:      getDocumentReference(prop, ""),
					Timestamp: document.Timestamp(value.Time),
					Precision: document.TimePrecision(value.Precision),
				},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", dataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func addQualifiers(
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	cache *es.Cache, namespace uuid.UUID, claim document.Claim, entityID, prop, statementID string, qualifiers map[string][]mediawiki.Snak, qualifiersOrder []string,
) errors.E { //nolint:unparam
	for _, p := range qualifiersOrder {
		for i, qualifier := range qualifiers[p] {
			qualifierClaims, errE := processSnak(
				ctx, store, cache, namespace, p, []interface{}{entityID, prop, statementID, "qualifier", p, i}, document.MediumConfidence, qualifier,
			)
			if errors.Is(errE, ErrSilentSkipped) {
				logger.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(errE).Send()
				continue
			} else if errE != nil {
				logger.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(errE).Send()
				continue
			}
			for j, qualifierClaim := range qualifierClaims {
				errE = claim.Add(qualifierClaim)
				if errE != nil {
					logger.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i).Int(j)).
						Err(errE).Msg("meta claim cannot be added")
				}
			}
		}
	}
	return nil
}

// addReference operates in two modes. In the first mode, when there is only one snak type per reference, it just converts those snaks to claims.
// In the second mode, when there are multiple snak types, it wraps them into a temporary WIKIDATA_REFERENCE claim which will be processed later.
// TODO: Implement post-processing of temporary WIKIDATA_REFERENCE claims.
func addReference(
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	cache *es.Cache, namespace uuid.UUID, claim document.Claim, entityID, prop, statementID string, i int, reference mediawiki.Reference,
) errors.E { //nolint:unparam
	// Edge case.
	if len(reference.SnaksOrder) == 0 {
		return nil
	}

	var referenceClaim document.Claim

	if len(reference.SnaksOrder) == 1 {
		referenceClaim = claim
	} else {
		referenceClaim = &document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(namespace, entityID, prop, statementID, "reference", i, "WIKIDATA_REFERENCE", 0),
				Confidence: document.NoConfidence,
			},
			Prop: document.GetCorePropertyReference("WIKIDATA_REFERENCE"),
			HTML: document.TranslatableHTMLString{
				"XX": html.EscapeString("A temporary group of multiple Wikidata reference statements for later processing."),
			},
		}
	}

	for _, property := range reference.SnaksOrder {
		for j, snak := range reference.Snaks[property] {
			cs, errE := processSnak(
				ctx, store, cache, namespace, property, []interface{}{entityID, prop, statementID, "reference", i, property, j}, document.MediumConfidence, snak,
			)
			if errors.Is(errE, ErrSilentSkipped) {
				logger.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j)).
					Err(errE).Send()
				continue
			} else if errE != nil {
				logger.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j)).
					Err(errE).Send()
				continue
			}
			for k, c := range cs {
				errE = referenceClaim.Add(c)
				if errE != nil {
					logger.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(property).Int(j).Int(k)).
						Err(errE).Msg("meta claim cannot be added")
				}
			}
		}
	}

	if len(reference.SnaksOrder) > 1 {
		errE := claim.Add(referenceClaim)
		if errE != nil {
			logger.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i)).
				Err(errE).Msg("meta claim cannot be added")
		}
	}

	return nil
}

// ConvertEntity converts both Wikidata entities and Wikimedia Commons entities.
// Entities can reference only Wikimedia Commons files and not Wikipedia files.
func ConvertEntity( //nolint:maintidx
	ctx context.Context, logger zerolog.Logger,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	cache *es.Cache, namespace uuid.UUID, entity mediawiki.Entity,
) (*document.D, errors.E) {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English Wikidata entities for now.
	// We do not require from Wikimedia Commons to have labels, so we skip the check for them.
	if len(englishLabels) == 0 && entity.Type != mediawiki.MediaInfo {
		if entity.Type == mediawiki.Property {
			// But properties should all have English label, so we warn here.
			logger.Warn().Str("entity", entity.ID).Msg("property is missing a label in English")
		}
		return nil, errors.WithStack(errors.BaseWrap(ErrSilentSkipped, "limited only to English"))
	}

	var id identifier.Identifier
	var name string
	var filename string
	if entity.Type == mediawiki.MediaInfo {
		filename = strings.TrimPrefix(entity.Title, "File:")
		filename = strings.ReplaceAll(filename, " ", "_")
		filename = FirstUpperCase(filename)

		id = document.GetID(NameSpaceWikimediaCommonsFile, filename)

		// We make a name from the title by removing prefix and file extension.
		name = strings.TrimPrefix(entity.Title, "File:")
		name = strings.TrimSuffix(name, path.Ext(name))
	} else {
		id = GetWikidataDocumentID(entity.ID)

		// We simply use the first label we have. We do not remove it from englishLabels
		// so that deduplication works correctly, but we check later on that we are not
		// adding any NAME claim equal to name.
		name = englishLabels[0]

		// Remove prefix if it is a template, module, or category entity.
		name = strings.TrimPrefix(name, "Template:")
		name = strings.TrimPrefix(name, "Module:")
		name = strings.TrimPrefix(name, "Category:")
	}

	// TODO: Set mnemonic if it is a property and the name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere. To every claim from input entity?
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    id,
			Score: document.LowConfidence,
		},
	}

	errE := doc.Add(&document.TextClaim{
		CoreClaim: document.CoreClaim{
			ID: document.GetID(namespace, entity.ID, "NAME", 0),
			// The first added English label is added as a high confidence claim.
			Confidence: document.HighConfidence,
		},
		Prop: document.GetCorePropertyReference("NAME"),
		HTML: document.TranslatableHTMLString{
			"en": html.EscapeString(name),
		},
	})
	if errE != nil {
		logger.Error().Str("entity", entity.ID).Err(errE).Msg("claim cannot be added")
	}

	switch entity.Type {
	case mediawiki.Property:
		doc.Claims = &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("WIKIDATA_PROPERTY_ID"),
					Value: entity.ID,
				},
			},
			Reference: document.ReferenceClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("WIKIDATA_PROPERTY_PAGE"),
					IRI:  "https://www.wikidata.org/wiki/Property:" + entity.ID,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "TYPE", 0, "PROPERTY", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("PROPERTY"),
				},
			},
		}
	case mediawiki.Item:
		doc.Claims = &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "WIKIDATA_ITEM_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("WIKIDATA_ITEM_ID"),
					Value: entity.ID,
				},
			},
			Reference: document.ReferenceClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "WIKIDATA_ITEM_PAGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("WIKIDATA_ITEM_PAGE"),
					IRI:  "https://www.wikidata.org/wiki/" + entity.ID,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "TYPE", 0, "ITEM", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("ITEM"),
				},
			},
		}
	case mediawiki.MediaInfo:
		// It is expected that this document will be merged with another document with standard
		// file claims, so the claims here are just a set of additional claims to be added and
		// are missing standard file claims.
		doc.Claims = &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(namespace, entity.ID, "WIKIMEDIA_COMMONS_ENTITY_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("WIKIMEDIA_COMMONS_ENTITY_ID"),
					Value: entity.ID,
				},
			},
		}
	default:
		return nil, errors.Errorf(`entity has invalid type: %d`, entity.Type)
	}

	// Exists for Wikidata entities.
	for _, site := range []struct {
		Wiki           string
		MnemonicPrefix string
		Domain         string
	}{
		{
			"enwiki",
			"ENGLISH_WIKIPEDIA",
			"en.wikipedia.org",
		},
		{
			"commonswiki",
			"WIKIMEDIA_COMMONS",
			"commons.wikimedia.org",
		},
	} {
		siteLink, ok := entity.SiteLinks[site.Wiki]
		if ok {
			url := siteLink.URL
			if url == "" {
				// First we make sure we do not have spaces.
				urlTitle := strings.ReplaceAll(siteLink.Title, " ", "_")
				// The first letter has to be upper case.
				urlTitle = FirstUpperCase(urlTitle)
				url = fmt.Sprintf("https://%s/wiki/%s", site.Domain, urlTitle)
			}
			for _, namespace := range nonMainNamespaces {
				if strings.HasPrefix(siteLink.Title, namespace) {
					// Only items have sitelinks. We want only items related to main articles (main namespace),
					// templates, modules, and categories.
					errE := errors.WithStack(errors.BaseWrap(ErrSilentSkipped, "`limited only to items related to main articles, templates, and categories"))
					errors.Details(errE)["wiki"] = site.Wiki
					errors.Details(errE)["title"] = siteLink.Title
					return nil, errE
				}
			}
			doc.Claims.Identifier = append(doc.Claims.Identifier, document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(namespace, entity.ID, site.MnemonicPrefix+"_PAGE_TITLE", 0),
					Confidence: document.HighConfidence,
				},
				Prop:  document.GetCorePropertyReference(site.MnemonicPrefix + "_PAGE_TITLE"),
				Value: siteLink.Title,
			})
			doc.Claims.Reference = append(doc.Claims.Reference, document.ReferenceClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(namespace, entity.ID, site.MnemonicPrefix+"_PAGE", 0),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference(site.MnemonicPrefix + "_PAGE"),
				IRI:  url,
			})
			// Here we add English Wikipedia page title to labels to be included as another NAME claim on the document.
			if site.Wiki == "enwiki" {
				englishLabels = append(englishLabels, siteLink.Title)
			}
		}
	}

	if entity.DataType != nil {
		// Which claim type should be used with this property?
		// We use this in resolveDataTypeFromPropertyDocument, too.
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			doc.Claims.Relation = append(doc.Claims.Relation, document.RelationClaim{
				CoreClaim: document.CoreClaim{
					ID: document.GetID(namespace, entity.ID, "TYPE", 0, claimTypeMnemonic, 0),
					// We have low confidence in this claim. Later on we augment it using statistics
					// on how are properties really used.
					// TODO: Decide what should really be confidence here or implement "later on" part described above.
					Confidence: document.LowConfidence,
				},
				Prop: document.GetCorePropertyReference("TYPE"),
				To:   document.GetCorePropertyReference(claimTypeMnemonic),
			})
		}
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	englishLabels = append(englishLabels, englishAliases...)
	for i, label := range englishLabels {
		// Remove prefix if it is a template, module, or category entity.
		label = strings.TrimPrefix(label, "Template:")
		label = strings.TrimPrefix(label, "Module:")
		label = strings.TrimPrefix(label, "Category:")
		englishLabels[i] = label
	}
	englishLabels = deduplicate(englishLabels)
	for i, label := range englishLabels {
		// NAME claim with name value has already been added, so we skip it.
		if label == name {
			continue
		}

		doc.Claims.Text = append(doc.Claims.Text, document.TextClaim{
			CoreClaim: document.CoreClaim{
				// We add +1 to i to make sure we do not repeat claim ID (we use the same form for name value NAME claim).
				ID: document.GetID(namespace, entity.ID, "NAME", i+1),
				// Other English labels and aliases are added with the medium confidence.
				Confidence: document.MediumConfidence,
			},
			Prop: document.GetCorePropertyReference("NAME"),
			HTML: document.TranslatableHTMLString{
				"en": html.EscapeString(label),
			},
		})
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	for i, description := range englishDescriptions {
		doc.Claims.Text = append(doc.Claims.Text, document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(namespace, entity.ID, "DESCRIPTION", i),
				Confidence: document.MediumConfidence,
			},
			Prop: document.GetCorePropertyReference("DESCRIPTION"),
			HTML: document.TranslatableHTMLString{
				"en": html.EscapeString(description),
			},
		})
	}

	// Deterministic iteration over a map.
	props := make([]string, 0, len(entity.Claims))
	for prop := range entity.Claims {
		props = append(props, prop)
	}
	sort.Strings(props)

	for _, prop := range props {
		for i, statement := range entity.Claims[prop] {
			if statement.ID == "" {
				// All statements should have an ID, so we warn here.
				logger.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Int(i)).Msg("missing a statement ID")
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claims, errE := processSnak(
				ctx, store, cache, namespace, prop, []interface{}{entity.ID, prop, statement.ID, "mainsnak"}, confidence, statement.MainSnak,
			)
			if errors.Is(errE, ErrSilentSkipped) {
				logger.Debug().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(errE).Send()
				continue
			} else if errE != nil {
				logger.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(errE).Send()
				continue
			}
			for j, claim := range claims {
				errE = addQualifiers(
					ctx, logger, store, cache, namespace, claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder,
				)
				if errE != nil {
					logger.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j).Str("qualifiers")).
						Err(errE).Send()
					continue
				}
				for i, reference := range statement.References {
					errE = addReference(ctx, logger, store, cache, namespace, claim, entity.ID, prop, statement.ID, i, reference)
					if errE != nil {
						logger.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j).Str("reference").Int(i)).
							Err(errE).Send()
						continue
					}
				}
				errE = doc.Add(claim)
				if errE != nil {
					logger.Error().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Int(j)).
						Err(errE).Msg("claim cannot be added")
				}
			}
		}
	}

	return &doc, nil
}
