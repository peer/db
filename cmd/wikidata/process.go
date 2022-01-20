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
)

func getItemID(id string) string {
	return identifier.FromUUID(uuid.NewSHA1(NameSpaceWikidata, []byte(id)))
}

func getStandardPropertyClaimID(id, mnemonic string, i int) string {
	return identifier.FromUUID(
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
	)
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
		return "IS_CLAIM_TYPE"
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
	panic(errors.Errorf(`unknown data type: %d`, dataType))
}

func processItem(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
	return nil
}

func processProperty(ctx context.Context, config *Config, entity mediawiki.Entity) errors.E {
	englishLabels := getEnglishValues(entity.Labels)
	// We are processing just English content for now.
	if len(englishLabels) == 0 {
		// But properties should all have English label, so we warn here.
		fmt.Fprintf(os.Stderr, "property %s is missing a label in English", entity.ID)
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
			ID: search.Identifier(id),
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
							ID:         search.Identifier(getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_ID", 0)),
							Confidence: 1.0,
						},
						Prop:       search.GetStandardPropertyReference("WIKIDATA_PROPERTY_ID"),
						Identifier: entity.ID,
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.Identifier(getStandardPropertyClaimID(entity.ID, "WIKIDATA_PROPERTY_PAGE", 0)),
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
		property.Active.MetaClaimTypes.Is = append(property.Active.MetaClaimTypes.Is, search.IsClaim{
			CoreClaim: search.CoreClaim{
				ID: search.Identifier(getStandardPropertyClaimID(entity.ID, claimTypeMnemonic, 0)),
				// We have low confidence in this claim. Later on we augment it using statistics
				// on how are properties really used.
				Confidence: 0.0,
			},
			Prop: search.GetStandardPropertyReference(claimTypeMnemonic),
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
		for i, description := range englishDescriptions {
			property.Active.SimpleClaimTypes.Text = append(property.Active.SimpleClaimTypes.Text, search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.Identifier(getStandardPropertyClaimID(entity.ID, "DESCRIPTION", i)),
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
		return errors.Errorf(`unknown entity type: %d`, entity.Type)
	}
}
