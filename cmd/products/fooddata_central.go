package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/krolaw/zipstream"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	DefaultFoodDataCentralDataURL = "https://fdc.nal.usda.gov/fdc-datasets/FoodData_Central_branded_food_json_2024-04-18.zip"
)

//nolint:lll
type FoodDataCentral struct {
	Disabled       bool   `default:"false"                            help:"Do not import FoodDataCentral data. Default: false."`
	DataURL        string `default:"${defaultFoodDataCentralDataURL}" help:"URL of FoodCentral dataset to use. It can be a local file path, too. Default: ${defaultFoodDataCentralDataURL}." name:"data"        placeholder:"URL"`
	IngredientsDir string `                                           help:"Path to a directory with JSONs with parsed ingredients."                                                         name:"ingredients" placeholder:"DIR" type:"path"`
}

type Nutrient struct {
	ID       int    `json:"id"`
	Number   string `json:"number"`
	Name     string `json:"name"`
	Rank     int    `json:"rank"`
	UnitName string `json:"unitName"`
}

type FoodNutrientSource struct {
	ID          int    `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type FoodNutrientDerivation struct {
	Code               string             `json:"code"`
	Description        string             `json:"description"`
	FoodNutrientSource FoodNutrientSource `json:"foodNutrientSource"`
}

type FoodNutrient struct {
	Type                   string                 `json:"type"`
	ID                     int                    `json:"id"`
	Nutrient               Nutrient               `json:"nutrient"`
	FoodNutrientDerivation FoodNutrientDerivation `json:"foodNutrientDerivation"`
	Amount                 float64                `json:"amount"`
}

type FoodAttributeType struct {
	ID          int    `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type FoodAttribute struct {
	ID                int               `json:"id"`
	Name              string            `json:"name"`
	Value             string            `json:"value"`
	FoodAttributeType FoodAttributeType `json:"foodAttributeType"`
}

type LabelNutrient struct {
	Value float64 `json:"value"`
}

type LabelNutrients struct {
	Fat           LabelNutrient `json:"fat"`
	SaturatedFat  LabelNutrient `json:"saturatedFat"`
	TransFat      LabelNutrient `json:"transFat"`
	Cholesterol   LabelNutrient `json:"cholesterol"`
	Sodium        LabelNutrient `json:"sodium"`
	Carbohydrates LabelNutrient `json:"carbohydrates"`
	Fiber         LabelNutrient `json:"fiber"`
	Sugars        LabelNutrient `json:"sugars"`
	Protein       LabelNutrient `json:"protein"`
	Calcium       LabelNutrient `json:"calcium"`
	Iron          LabelNutrient `json:"iron"`
	Calories      LabelNutrient `json:"calories"`
	Potassium     LabelNutrient `json:"potassium"`
	AddedSugar    LabelNutrient `json:"addedSugar"`
	VitaminD      LabelNutrient `json:"vitaminD"`
}

type Microbe struct {
	MicrobeCode   string `json:"microbeCode"`
	Method        string `json:"method"`
	MinValue      int    `json:"minValue"`
	UnitOfMeasure string `json:"uom"`
}

type FoodUpdate struct {
	FoodClass       string          `json:"foodClass"`
	Description     string          `json:"description"`
	FoodAttributes  []FoodAttribute `json:"foodAttributes"`
	DataType        string          `json:"dataType"`
	FDCID           int             `json:"fdcId"`
	PublicationDate string          `json:"publicationDate"`
}

type BrandedFood struct {
	FoodClass           string `json:"foodClass"` // Always "Branded".
	DataType            string `json:"dataType"`  // Always "Branded".
	DataSource          string `json:"dataSource"`
	BrandedFoodCategory string `json:"brandedFoodCategory,omitempty"` // Can be an empty string.
	GPCClassCode        int    `json:"gpcClassCode,omitempty"`

	FDCID int    `json:"fdcId"`
	GTIN  string `json:"gtinUpc"`

	PublicationDate  string `json:"publicationDate"`
	AvailableDate    string `json:"availableDate"`
	ModifiedDate     string `json:"modifiedDate"`
	DiscontinuedDate string `json:"discontinuedDate,omitempty"`

	Description       string `json:"description"`
	Ingredients       string `json:"ingredients,omitempty"`      // Can be an empty string.
	ShortDescription  string `json:"shortDescription,omitempty"` // Can be an empty string.
	CaffeineStatement string `json:"caffeineStatement,omitempty"`

	MarketCountry string   `json:"marketCountry"`
	TradeChannels []string `json:"tradeChannels"`
	BrandOwner    string   `json:"brandOwner"`
	BrandName     string   `json:"brandName,omitempty"`    // Can be an empty string.
	SubbrandName  string   `json:"subbrandName,omitempty"` // Can be an empty string.

	ServingSize              float64 `json:"servingSize"`
	ServingSizeUnit          string  `json:"servingSizeUnit"`
	HouseholdServingFullText string  `json:"householdServingFullText,omitempty"` // Can be an empty string.
	PackageWeight            string  `json:"packageWeight,omitempty"`            // Can be an empty string.
	PreparationStateCode     string  `json:"preparationStateCode,omitempty"`     // Can be an empty string.

	FoodNutrients  []FoodNutrient  `json:"foodNutrients"`
	FoodAttributes []FoodAttribute `json:"foodAttributes"`
	LabelNutrients LabelNutrients  `json:"labelNutrients"`
	Microbes       []Microbe       `json:"microbes"`
	FoodUpdateLog  []FoodUpdate    `json:"foodUpdateLog"`
}

type Ingredient struct {
	Name        string       `json:"name"`
	Ingredients []Ingredient `json:"ingredients,omitempty"`
	Meta        []string     `json:"meta,omitempty"`
}

type Ingredients struct {
	Ingredients []Ingredient `json:"ingredients"`
	Meta        []string     `json:"meta,omitempty"`
}

func getFoods(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]BrandedFood, errors.E) {
	reader, _, errE := indexer.CachedDownload(ctx, httpClient, logger, cacheDir, url)
	if errE != nil {
		return nil, errE
	}
	defer reader.Close()

	zipReader := zipstream.NewReader(reader)
	var file *zip.FileHeader
	var err error
	for file, err = zipReader.Next(); err == nil; file, err = zipReader.Next() {
		if file.Name == "brandedDownload.json" {
			var result struct {
				BrandedFoods []BrandedFood `json:"BrandedFoods"` //nolint:tagliatelle
			}
			// TODO: We should stream results as they are downloaded/decompressed/decoded like go-mediawiki package does.
			errE := x.DecodeJSONWithoutUnknownFields(zipReader, &result)
			if errE != nil {
				return nil, errE
			}
			return result.BrandedFoods, nil
		}
	}

	if errors.Is(err, io.EOF) {
		return nil, errors.New(`"brandedDownload.json" file not found`)
	}

	return nil, errors.WithStack(err)
}

func getIngredients(ingredientsDir string, food BrandedFood) (Ingredients, errors.E) {
	if ingredientsDir == "" {
		return Ingredients{}, nil //nolint:exhaustruct
	}

	p := filepath.Join(ingredientsDir, fmt.Sprintf("%d.json", food.FDCID))
	file, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Ingredients{}, nil //nolint:exhaustruct
	} else if err != nil {
		return Ingredients{}, errors.WithStack(err)
	}
	defer file.Close()
	var result Ingredients
	errE := x.DecodeJSONWithoutUnknownFields(file, &result)
	if errE != nil {
		return Ingredients{}, errE
	}
	return result, nil
}

func addIngredients(doc *document.D, fdcid, i int, ingredients []Ingredient) (int, errors.E) {
	var errE errors.E
	for _, ingredient := range ingredients {
		if s := strings.TrimSpace(ingredient.Name); s != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", fdcid, "INGREDIENT", i),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("INGREDIENT"),
				String: strings.ToLower(s),
			})
			if errE != nil {
				return i, errE
			}
			i++
		}

		i, errE = addIngredients(doc, fdcid, i, ingredient.Ingredients)
		if errE != nil {
			return i, errE
		}
	}

	return i, nil
}

func makeFoodDataCentralDoc(food BrandedFood, ingredients Ingredients) (document.D, errors.E) { //nolint:maintidx
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "FDCID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("FDCID"),
					Value: strconv.Itoa(food.FDCID),
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "GTIN", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("GTIN"),
					Value: food.GTIN,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "TYPE", 0, "BRANDED_FOOD", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("BRANDED_FOOD"),
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "DATA_SOURCE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("DATA_SOURCE"),
					String: food.DataSource,
				},
			},
		},
	}

	if s := strings.TrimSpace(food.BrandedFoodCategory); s != "" {
		// TODO: Should this be a text claim? Or a relation claim.
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "CATEGORY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("CATEGORY"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.PublicationDate); s != "" {
		t, err := time.Parse("1/2/2006", s)
		if err != nil {
			errE := errors.WithMessage(err, "error parsing publication date")
			errors.Details(errE)["date"] = s
			return doc, errE
		}
		errE := doc.Add(&document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "PUBLICATION_DATE", 0),
				Confidence: document.HighConfidence,
			},
			Prop:      document.GetCorePropertyReference("PUBLICATION_DATE"),
			Timestamp: document.Timestamp(t),
			Precision: document.TimePrecisionDay,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.AvailableDate); s != "" {
		t, err := time.Parse("1/2/2006", s)
		if err != nil {
			errE := errors.WithMessage(err, "error parsing available date")
			errors.Details(errE)["date"] = s
			return doc, errE
		}
		errE := doc.Add(&document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "AVAILABLE_DATE", 0),
				Confidence: document.HighConfidence,
			},
			Prop:      document.GetCorePropertyReference("AVAILABLE_DATE"),
			Timestamp: document.Timestamp(t),
			Precision: document.TimePrecisionDay,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.ModifiedDate); s != "" {
		t, err := time.Parse("1/2/2006", s)
		if err != nil {
			errE := errors.WithMessage(err, "error parsing modified date")
			errors.Details(errE)["date"] = s
			return doc, errE
		}
		errE := doc.Add(&document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "MODIFIED_DATE", 0),
				Confidence: document.HighConfidence,
			},
			Prop:      document.GetCorePropertyReference("MODIFIED_DATE"),
			Timestamp: document.Timestamp(t),
			Precision: document.TimePrecisionDay,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.DiscontinuedDate); s != "" {
		t, err := time.Parse("1/2/2006", s)
		if err != nil {
			errE := errors.WithMessage(err, "error parsing discontinued date")
			errors.Details(errE)["date"] = s
			return doc, errE
		}
		errE := doc.Add(&document.TimeClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "DISCONTINUED_DATE", 0),
				Confidence: document.HighConfidence,
			},
			Prop:      document.GetCorePropertyReference("DISCONTINUED_DATE"),
			Timestamp: document.Timestamp(t),
			Precision: document.TimePrecisionDay,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.Description); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "DESCRIPTION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("DESCRIPTION"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.Ingredients); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "INGREDIENTS", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("INGREDIENTS"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.ShortDescription); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "DESCRIPTION", 1),
				Confidence: document.MediumConfidence,
			},
			Prop: document.GetCorePropertyReference("DESCRIPTION"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.CaffeineStatement); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "CAFFEINE_STATEMENT", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("CAFFEINE_STATEMENT"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.MarketCountry); s != "" {
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "MARKET_COUNTRY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("MARKET_COUNTRY"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	for i, tradeChannel := range food.TradeChannels {
		if s := strings.TrimSpace(tradeChannel); s != "" {
			if s == "NO_TRADE_CHANNEL" {
				errE := doc.Add(&document.NoValueClaim{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "TRADE_CHANNEL", i),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TRADE_CHANNEL"),
				})
				if errE != nil {
					return doc, errE
				}
			} else {
				errE := doc.Add(&document.StringClaim{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "TRADE_CHANNEL", i),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("TRADE_CHANNEL"),
					String: strings.ReplaceAll(strings.ToLower(s), "_", " "),
				})
				if errE != nil {
					return doc, errE
				}
			}
		}
	}

	if s := strings.TrimSpace(food.BrandOwner); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "BRAND_OWNER", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("BRAND_OWNER"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.BrandName); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "BRAND_NAME", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("BRAND_NAME"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(food.SubbrandName); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "SUBBRAND_NAME", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("SUBBRAND_NAME"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	var unit document.AmountUnit
	var factor float64
	switch food.ServingSizeUnit {
	case "g", "GM", "GRM", "MC", "IU", "MG": // Gram.
		unit = document.AmountUnitKilogram
		factor = 0.001
	case "ml", "MLT": // Millilitre.
		unit = document.AmountUnitLitre
		factor = 0.001
	default:
		errE := errors.New("unsupported serving size unit")
		errors.Details(errE)["unit"] = food.ServingSizeUnit
		return doc, errE
	}

	errE := doc.Add(&document.AmountClaim{
		CoreClaim: document.CoreClaim{
			ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "SERVING_SIZE", 0),
			Confidence: document.HighConfidence,
		},
		Prop:   document.GetCorePropertyReference("SERVING_SIZE"),
		Amount: factor * food.ServingSize,
		Unit:   unit,
	})
	if errE != nil {
		return doc, errE
	}

	if s := strings.TrimSpace(food.HouseholdServingFullText); s != "" {
		errE = doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "BRANDED_FOOD", food.FDCID, "SERVING_SIZE_DESCRIPTION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("SERVING_SIZE_DESCRIPTION"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	_, errE = addIngredients(&doc, food.FDCID, 0, ingredients.Ingredients)
	if errE != nil {
		return doc, errE
	}

	return doc, nil
}

func (f FoodDataCentral) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if f.Disabled {
		return nil
	}

	foods, errE := getFoods(ctx, httpClient, config.Logger, config.CacheDir, f.DataURL)
	if errE != nil {
		return errE
	}

	description := indexer.StructName(BrandedFood{}) + " processing" //nolint:exhaustruct
	progress := es.Progress(config.Logger, nil, nil, nil, description)
	indexingSize.Add(int64(len(foods)))

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, x.NewCounter(int64(len(foods))), indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	for _, food := range foods {
		if ctx.Err() != nil {
			break
		}

		ingredients, errE := getIngredients(f.IngredientsDir, food)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}

		doc, errE := makeFoodDataCentralDoc(food, ingredients)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", len(foods)).
		Msg(description + " done")

	return nil
}
