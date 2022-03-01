package wikipedia

import (
	"context"
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

const (
	highConfidence   = 1.0
	mediumConfidence = 0.5
	lowConfidence    = 0.0
	noConfidence     = -1.0
)

var (
	NameSpaceWikidata = uuid.MustParse("8f8ba777-bcce-4e45-8dd4-a328e6722c82")

	notSupportedDataValueTypeError = errors.BaseWrap(SilentSkippedError, "not supported data value type")
	notSupportedDataTypeError      = errors.BaseWrap(SilentSkippedError, "not supported data type")
	NotFoundFileError              = errors.Base("not found file")

	nonMainWikipediaNamespaces = []string{
		"User:",
		"Wikipedia:",
		"File:",
		"MediaWiki:",
		"Template:",
		"Help:",
		"Category:",
		"Portal:",
		"Draft:",
		"TimedText:",
		"Module:",
	}
)

func GetWikidataDocumentID(id string) search.Identifier {
	return search.GetID(NameSpaceWikidata, id)
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
	switch dataType {
	case mediawiki.WikiBaseItem:
		return "RELATION_CLAIM_TYPE"
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
		return "RELATION_CLAIM_TYPE"
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
		return highConfidence
	case mediawiki.Normal:
		return mediumConfidence
	case mediawiki.Deprecated:
		return noConfidence
	}
	panic(errors.Errorf(`statement %s of property %s for entity %s has invalid rank: %d`, statementID, prop, entityID, rank))
}

// It does not return a valid reference: name is set to the ID itself for the language "XX".
func getDocumentReference(id string) search.DocumentReference {
	return search.DocumentReference{
		ID: GetWikidataDocumentID(id),
		Name: map[string]string{
			"XX": id,
		},
		Score: 0.0,
	}
}

type wikimediaCommonsFile struct {
	Reference search.DocumentReference
	Type      string
	URL       string
	Preview   []string
}

func getDocumentFromES(ctx context.Context, esClient *elastic.Client, property, id string) (*search.Document, *elastic.SearchHit, errors.E) {
	searchResult, err := esClient.Search("docs").Query(elastic.NewNestedQuery("active.id",
		elastic.NewBoolQuery().Must(
			elastic.NewTermQuery("active.id.prop._id", search.GetStandardPropertyID(property)),
			elastic.NewTermQuery("active.id.id", id),
		),
	)).SeqNoPrimaryTerm(true).Do(ctx)
	if err != nil {
		// Caller should add details to the error.
		return nil, nil, errors.WithStack(err)
	}

	// There might be multiple hits because IDs are not unique (we remove zeroes).
	for _, hit := range searchResult.Hits.Hits {
		var document search.Document
		err = x.UnmarshalWithoutUnknownFields(hit.Source, &document)
		if err != nil {
			// Caller should add details to the error.
			return nil, nil, errors.WithStack(err)
		}

		// ID is not stored in the document, so we set it here ourselves.
		document.ID = search.Identifier(hit.Id)

		found := false
		for _, claim := range document.Get(search.GetStandardPropertyID(property)) {
			if c, ok := claim.(*search.IdentifierClaim); ok && c.Identifier == id {
				found = true
				break
			}
		}

		// If this hit is not precisely for this name, we continue with the next one.
		if !found {
			continue
		}

		return &document, hit, nil
	}

	// Caller should add details to the error.
	return nil, nil, errors.WithStack(NotFoundFileError)
}

func getWikimediaCommonsFileReferenceFromES(ctx context.Context, esClient *elastic.Client, name string) (*wikimediaCommonsFile, errors.E) {
	document, _, err := getDocumentFromES(ctx, esClient, "WIKIMEDIA_COMMONS_FILE_NAME", name)
	if err != nil {
		errors.Details(err)["file"] = name
		return nil, err
	}

	for _, claim := range document.Get(search.GetStandardPropertyID("DATA")) {
		if c, ok := claim.(*search.FileClaim); ok {
			file := &wikimediaCommonsFile{
				Reference: search.DocumentReference{
					ID:     document.ID,
					Name:   document.Name,
					Score:  document.Score,
					Scores: document.Scores,
				},
				Type:    c.Type,
				URL:     c.URL,
				Preview: c.Preview,
			}
			return file, nil
		}
	}

	errE := errors.New("Wikimedia commons file document is missing a DATA file claim")
	errors.Details(errE)["file"] = name
	return nil, errE
}

func getWikimediaCommonsFileReference(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, cache *Cache, idArgs []interface{}, name string,
) (*wikimediaCommonsFile, errors.E) {
	maybeFile, ok := cache.Get(name)
	if ok {
		if maybeFile == nil {
			errE := errors.WithStack(NotFoundFileError)
			errors.Details(errE)["file"] = name
			return nil, errE
		}
		return maybeFile.(*wikimediaCommonsFile), nil
	}

	file, err := getWikimediaCommonsFileReferenceFromES(ctx, esClient, name)
	if errors.Is(err, NotFoundFileError) {
		// Passthrough.
	} else if err != nil {
		return nil, err
	} else {
		cache.Add(name, file)
		return file, nil
	}

	// We could not find a file. Maybe there is a redirect?
	// We do not check DescriptionURL because all Wikimedia Commons
	// files should be from Wikimedia Commons.
	ii, err := getImageInfoForFilename(ctx, httpClient, "commons.wikimedia.org", name)
	if err != nil {
		errE := errors.WithMessage(err, "checking for redirect")
		errors.Details(errE)["file"] = name
		return nil, errE
	} else if ii.Redirect == "" {
		// No redirect.
		cache.Add(name, nil)
		errE := errors.WithStack(NotFoundFileError)
		errors.Details(errE)["file"] = name
		return nil, errE
	}

	maybeFile, ok = cache.Get(ii.Redirect)
	if ok {
		if maybeFile == nil {
			errE := errors.WithStack(NotFoundFileError)
			errors.Details(errE)["file"] = name
			errors.Details(errE)["redirect"] = ii.Redirect
			return nil, errE
		}
		return maybeFile.(*wikimediaCommonsFile), nil
	}

	file, err = getWikimediaCommonsFileReferenceFromES(ctx, esClient, ii.Redirect)
	if err != nil {
		if errors.Is(err, NotFoundFileError) {
			cache.Add(name, nil)
			cache.Add(ii.Redirect, nil)
		}
		errE := errors.WithMessage(err, "after redirect")
		errors.Details(errE)["file"] = name
		errors.Details(errE)["redirect"] = ii.Redirect
		return nil, errE
	}

	log.Warn().Interface("entity", idArgs[0]).Interface("path", idArgs[1:]).Str("file", name).Str("redirect", ii.Redirect).Msg("referencing a file which redirects")

	cache.Add(name, file)
	cache.Add(ii.Redirect, file)
	return file, nil
}

func processSnak( //nolint:ireturn,nolintlint
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, cache *Cache, skippedCommonsFiles *sync.Map,
	prop string, idArgs []interface{}, confidence search.Confidence, snak mediawiki.Snak,
) (search.Claim, errors.E) {
	id := search.GetID(NameSpaceWikidata, idArgs...)

	switch snak.SnakType {
	case mediawiki.Value:
		// Process later down.
	case mediawiki.SomeValue:
		return &search.UnknownValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getDocumentReference(prop),
		}, nil
	case mediawiki.NoValue:
		return &search.NoValueClaim{
			CoreClaim: search.CoreClaim{
				ID:         id,
				Confidence: confidence,
			},
			Prop: getDocumentReference(prop),
		}, nil
	}

	if snak.DataValue == nil {
		return nil, errors.New("nil data value")
	}

	switch value := snak.DataValue.Value.(type) {
	case mediawiki.ErrorValue:
		return nil, errors.New(string(value))
	case mediawiki.StringValue:
		switch snak.DataType { //nolint:exhaustive
		case mediawiki.ExternalID:
			return &search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:       getDocumentReference(prop),
				Identifier: string(value),
			}, nil
		case mediawiki.String:
			return &search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:   getDocumentReference(prop),
				String: string(value),
			}, nil
		case mediawiki.CommonsMedia:
			// First we make sure we do not have spaces.
			filename := strings.ReplaceAll(string(value), " ", "_")
			// The first letter has to be upper case.
			filename = FirstUpperCase(filename)

			file, err := getWikimediaCommonsFileReference(ctx, log, httpClient, esClient, cache, idArgs, filename)
			if err != nil {
				if errors.Is(err, NotFoundFileError) {
					if _, ok := skippedCommonsFiles.Load(filename); ok {
						errE := errors.WithStack(errors.BaseWrap(SilentSkippedError, "not found skipped file"))
						errors.Details(errE)["file"] = filename
						return nil, errE
					}
				}
				return nil, err
			}

			// After here we should not be using "filename" anymore because we might figure out that
			// it redirects to another file. So only "file" should be used.

			args := append([]interface{}{}, idArgs...)
			args = append(args, file.Reference.ID, 0)
			claimID := search.GetID(NameSpaceWikidata, args...)
			return &search.FileClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
					Meta: &search.ClaimTypes{
						Is: search.IsClaims{
							{
								CoreClaim: search.CoreClaim{
									ID:         claimID,
									Confidence: highConfidence,
								},
								To: file.Reference,
							},
						},
					},
				},
				Prop:    getDocumentReference(prop),
				Type:    file.Type,
				URL:     file.URL,
				Preview: file.Preview,
			}, nil
		case mediawiki.URL:
			return &search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				IRI:  string(value),
			}, nil
		case mediawiki.GeoShape:
			return nil, errors.Errorf("%w: GeoShape", notSupportedDataTypeError)
		case mediawiki.Math:
			return nil, errors.Errorf("%w: Math", notSupportedDataTypeError)
		case mediawiki.MusicalNotation:
			return nil, errors.Errorf("%w: MusicalNotation", notSupportedDataTypeError)
		case mediawiki.TabularData:
			return nil, errors.Errorf("%w: TabularData", notSupportedDataTypeError)
		default:
			return nil, errors.Errorf("unexpected data type for StringValue: %d", snak.DataType)
		}
	case mediawiki.WikiBaseEntityIDValue:
		switch snak.DataType { //nolint:exhaustive
		case mediawiki.WikiBaseItem:
			if value.Type != mediawiki.ItemType {
				return nil, errors.Errorf("WikiBaseItem data type, but WikiBaseEntityIDValue has type %d, not ItemType", value.Type)
			}
			return &search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				To:   getDocumentReference(value.ID),
			}, nil
		case mediawiki.WikiBaseProperty:
			if value.Type != mediawiki.PropertyType {
				return nil, errors.Errorf("WikiBaseProperty data type, but WikiBaseEntityIDValue has type %d, not PropertyType", value.Type)
			}
			return &search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				To:   getDocumentReference(value.ID),
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
		switch snak.DataType { //nolint:exhaustive
		case mediawiki.MonolingualText:
			if value.Language != "en" && !strings.HasPrefix(value.Language, "en-") {
				return nil, errors.WithStack(errors.BaseWrap(SilentSkippedError, "limited only to English"))
			}
			return &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop: getDocumentReference(prop),
				HTML: search.TranslatableHTMLString{value.Language: html.EscapeString(value.Text)},
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for MonolingualTextValue: %d", snak.DataType)
		}
	case mediawiki.QuantityValue:
		switch snak.DataType { //nolint:exhaustive
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
			claim := search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:             getDocumentReference(prop),
				Amount:           amount,
				UncertaintyLower: uncertaintyLower,
				UncertaintyUpper: uncertaintyUpper,
			}

			if value.Unit == "1" {
				claim.Unit = search.AmountUnitNone
			} else {
				// For now we store the amount as-is and convert to the same unit later on
				// using the unit we store into meta claims.
				claim.Unit = search.AmountUnitCustom
				args := append([]interface{}{}, idArgs...)
				args = append(args, "UNIT", 0)
				claimID := search.GetID(NameSpaceWikidata, args...)
				var unitID string
				if strings.HasPrefix(value.Unit, "http://www.wikidata.org/entity/") {
					unitID = strings.TrimPrefix(value.Unit, "http://www.wikidata.org/entity/")
				} else if strings.HasPrefix(value.Unit, "https://www.wikidata.org/wiki/") {
					unitID = strings.TrimPrefix(value.Unit, "https://www.wikidata.org/wiki/")
				} else {
					return nil, errors.Errorf("unsupported unit URL: %s", value.Unit)
				}
				claim.CoreClaim.Meta = &search.ClaimTypes{
					Relation: search.RelationClaims{
						{
							CoreClaim: search.CoreClaim{
								ID:         claimID,
								Confidence: highConfidence,
							},
							Prop: search.GetStandardPropertyReference("UNIT"),
							To:   getDocumentReference(unitID),
						},
					},
				}
			}

			return &claim, nil
		default:
			return nil, errors.Errorf("unexpected data type for QuantityValue: %d", snak.DataType)
		}
	case mediawiki.TimeValue:
		switch snak.DataType { //nolint:exhaustive
		case mediawiki.Time:
			// TODO: Convert timestamps in Julian calendar to ones in Gregorian calendar.
			return &search.TimeClaim{
				CoreClaim: search.CoreClaim{
					ID:         id,
					Confidence: confidence,
				},
				Prop:      getDocumentReference(prop),
				Timestamp: search.Timestamp(value.Time),
				Precision: search.TimePrecision(value.Precision),
			}, nil
		default:
			return nil, errors.Errorf("unexpected data type for TimeValue: %d", snak.DataType)
		}
	}

	return nil, errors.Errorf(`unknown data value type: %+v`, snak.DataValue.Value)
}

func addQualifiers(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, cache *Cache, skippedCommonsFiles *sync.Map,
	claim search.Claim, entityID, prop, statementID string,
	qualifiers map[string][]mediawiki.Snak, qualifiersOrder []string,
) errors.E {
	for _, p := range qualifiersOrder {
		for i, qualifier := range qualifiers[p] {
			qualifierClaim, err := processSnak(
				ctx, log, httpClient, esClient, cache, skippedCommonsFiles, p,
				[]interface{}{entityID, prop, statementID, "qualifier", p, i},
				mediumConfidence, qualifier,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			err = claim.AddMeta(qualifierClaim)
			if err != nil {
				log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("qualifier").Str(p).Int(i)).
					Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
			}
		}
	}
	return nil
}

// addReference uses the first snak of a reference to construct a claim and all other snaks are added as meta claims of that first claim.
func addReference(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, cache *Cache, skippedCommonsFiles *sync.Map,
	claim search.Claim, entityID, prop, statementID string, i int, reference mediawiki.Reference,
) errors.E {
	var referenceClaim search.Claim

	for _, p := range reference.SnaksOrder {
		for j, snak := range reference.Snaks[p] {
			c, err := processSnak(
				ctx, log, httpClient, esClient, cache, skippedCommonsFiles, p, []interface{}{entityID, prop, statementID, "reference", i, p, j}, mediumConfidence, snak,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(p).Int(j)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(p).Int(j)).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			if referenceClaim == nil {
				referenceClaim = c
			} else {
				err = referenceClaim.AddMeta(c)
				if err != nil {
					log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i).Str(p).Int(j)).
						Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
				}
			}
		}
	}

	if referenceClaim == nil {
		return nil
	}

	err := claim.AddMeta(referenceClaim)
	if err != nil {
		log.Error().Str("entity", entityID).Array("path", zerolog.Arr().Str(prop).Str(statementID).Str("reference").Int(i)).
			Err(err).Fields(errors.AllDetails(err)).Msg("meta claim cannot be added")
	}

	return nil
}

// Wikipedia entities can reference only Wikimedia Commons files and not Wikipedia files. So we need only skippedCommonsFiles.
func ConvertEntity(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, cache *Cache,
	skippedCommonsFiles *sync.Map, entity mediawiki.Entity,
) (*search.Document, errors.E) {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English content for now.
	if len(englishLabels) == 0 {
		if entity.Type == mediawiki.Property {
			// But properties should all have English label, so we warn here.
			log.Warn().Str("entity", entity.ID).Msg("property is missing a label in English")
		}
		return nil, errors.WithStack(errors.BaseWrap(SilentSkippedError, "limited only to English"))
	}

	id := GetWikidataDocumentID(entity.ID)

	// We simply use the first label we have.
	name := englishLabels[0]
	englishLabels = englishLabels[1:]

	// TODO: Set mnemonic if a property and the name is unique (it should be).
	// TODO: Store last item revision and last modification time somewhere.
	document := search.Document{
		CoreDocument: search.CoreDocument{
			ID: id,
			Name: search.Name{
				"en": name,
			},
			Score: 0.0,
		},
	}

	if entity.Type == mediawiki.Property {
		document.Active = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_PROPERTY_ID", 0),
						Confidence: highConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_PROPERTY_PAGE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIDATA_PROPERTY_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/Property:%s", entity.ID),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "PROPERTY", 0),
						Confidence: highConfidence,
					},
					To: search.GetStandardPropertyReference("PROPERTY"),
				},
			},
		}
	} else if entity.Type == mediawiki.Item {
		document.Active = &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_ITEM_ID", 0),
						Confidence: highConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIDATA_ITEM_ID"),
					Identifier: entity.ID,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "WIKIDATA_ITEM_PAGE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIDATA_ITEM_PAGE"),
					IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/%s", entity.ID),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikidata, entity.ID, "ITEM", 0),
						Confidence: highConfidence,
					},
					To: search.GetStandardPropertyReference("ITEM"),
				},
			},
		}
	} else {
		return nil, errors.Errorf(`entity has invalid type: %d`, entity.Type)
	}

	siteLink, ok := entity.SiteLinks["enwiki"]
	if ok {
		url := siteLink.URL
		if url == "" {
			url = fmt.Sprintf("https://en.wikipedia.org/wiki/%s", siteLink.Title)
		}
		for _, namespace := range nonMainWikipediaNamespaces {
			if strings.HasPrefix(siteLink.Title, namespace) {
				// Only items have sitelinks. We want only items related to main Wikipedia articles (main namespace).
				errE := errors.WithStack(errors.BaseWrap(SilentSkippedError, "`limited only to items related to main Wikipedia articles"))
				errors.Details(errE)["title"] = siteLink.Title
				return nil, errE
			}
		}
		document.Active.Identifier = append(document.Active.Identifier, search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikidata, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE_TITLE", 0),
				Confidence: highConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE_TITLE"),
			Identifier: siteLink.Title,
		})
		document.Active.Reference = append(document.Active.Reference, search.ReferenceClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikidata, entity.ID, "ENGLISH_WIKIPEDIA_ARTICLE", 0),
				Confidence: highConfidence,
			},
			Prop: search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_ARTICLE"),
			IRI:  url,
		})
	}

	if entity.DataType != nil {
		claimTypeMnemonic := getPropertyClaimType(*entity.DataType)
		if claimTypeMnemonic != "" {
			document.Active.Is = append(document.Active.Is, search.IsClaim{
				CoreClaim: search.CoreClaim{
					ID: search.GetID(NameSpaceWikidata, entity.ID, claimTypeMnemonic, 0),
					// We have low confidence in this claim. Later on we augment it using statistics
					// on how are properties really used.
					Confidence: lowConfidence,
				},
				To: search.GetStandardPropertyReference(claimTypeMnemonic),
			})
		}
	}

	englishAliases := getEnglishValuesSlice(entity.Aliases)
	if len(englishAliases) > 0 {
		englishLabels = append(englishLabels, englishAliases...)
		englishLabels = deduplicate(englishLabels)
	}

	if len(englishLabels) > 0 {
		document.CoreDocument.OtherNames = search.OtherNames{
			"en": englishLabels,
		}
	}

	englishDescriptions := getEnglishValues(entity.Descriptions)
	if len(englishDescriptions) > 0 {
		for i, description := range englishDescriptions {
			document.Active.Text = append(document.Active.Text, search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceWikidata, entity.ID, "DESCRIPTION", i),
					Confidence: highConfidence,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
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
		for i, statement := range entity.Claims[prop] {
			if statement.ID == "" {
				// All statements should have an ID, so we warn here.
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Int(i)).Msg("missing a statement ID")
				continue
			}

			confidence := getConfidence(entity.ID, prop, statement.ID, statement.Rank)
			claim, err := processSnak(
				ctx, log, httpClient, esClient, cache, skippedCommonsFiles, prop, []interface{}{entity.ID, prop, statement.ID, "mainsnak"}, confidence, statement.MainSnak,
			)
			if errors.Is(err, SilentSkippedError) {
				log.Debug().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			} else if err != nil {
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("mainsnak")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			err = addQualifiers(ctx, log, httpClient, esClient, cache, skippedCommonsFiles, claim, entity.ID, prop, statement.ID, statement.Qualifiers, statement.QualifiersOrder)
			if err != nil {
				log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("qualifiers")).
					Err(err).Fields(errors.AllDetails(err)).Send()
				continue
			}
			for i, reference := range statement.References {
				err = addReference(ctx, log, httpClient, esClient, cache, skippedCommonsFiles, claim, entity.ID, prop, statement.ID, i, reference)
				if err != nil {
					log.Warn().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID).Str("reference").Int(i)).
						Err(err).Fields(errors.AllDetails(err)).Send()
					continue
				}
			}
			err = document.Add(claim)
			if err != nil {
				log.Error().Str("entity", entity.ID).Array("path", zerolog.Arr().Str(prop).Str(statement.ID)).
					Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
			}
		}
	}

	return &document, nil
}
