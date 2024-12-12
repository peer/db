package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/krolaw/zipstream"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
)

const (
	progressPrintRate = 30 * time.Second
)

//nolint:gochecknoglobals
var NameSpaceFood = uuid.MustParse("55945768-34e9-4584-9310-cf78602a4aa7")

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

func getPathAndURL(cacheDir, url string) (string, string) {
	_, err := os.Stat(url)
	if os.IsNotExist(err) {
		return filepath.Join(cacheDir, path.Base(url)), url
	}
	return url, ""
}

func structName(name string) string {
	i := strings.LastIndex(name, ".")
	return strings.ToLower(name[i+1:])
}

func getFoods(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]BrandedFood, errors.E) {
	cachedPath, url := getPathAndURL(cacheDir, url)

	var cachedReader io.Reader
	var cachedSize int64

	cachedFile, err := os.Open(cachedPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.WithStack(err)
		}
		// File does not exists. Continue.
	} else {
		defer cachedFile.Close()
		cachedReader = cachedFile
		cachedSize, err = cachedFile.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		_, err = cachedFile.Seek(0, io.SeekStart)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	if cachedReader == nil {
		// File does not already exist. We download the file and optionally save it.
		req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		downloadReader, errE := x.NewRetryableResponse(httpClient, req)
		if errE != nil {
			return nil, errE
		}
		defer downloadReader.Close()
		cachedSize = downloadReader.Size()
		cachedFile, err := os.Create(cachedPath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer func() {
			info, err := os.Stat(cachedPath)
			if err != nil || downloadReader.Size() != info.Size() {
				// Incomplete file. Delete.
				_ = os.Remove(cachedPath)
			}
		}()
		defer cachedFile.Close()
		cachedReader = io.TeeReader(downloadReader, cachedFile)
	}

	progress := es.Progress(logger, nil, nil, nil, structName(fmt.Sprintf("%T", BrandedFood{}))+" download progress") //nolint:exhaustruct
	countingReader := &x.CountingReader{Reader: cachedReader}
	ticker := x.NewTicker(ctx, countingReader, cachedSize, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	zipReader := zipstream.NewReader(countingReader)
	for file, err := zipReader.Next(); !errors.Is(err, io.EOF); file, err = zipReader.Next() {
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

	return nil, errors.New(`"brandedDownload.json" file not found`)
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
					ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", fdcid, "INGREDIENT", i),
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

func makeDoc(food BrandedFood, ingredients Ingredients) (document.D, errors.E) { //nolint:maintidx
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "FDCID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("FDCID"),
					Value: strconv.Itoa(food.FDCID),
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "GTIN", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("GTIN"),
					Value: food.GTIN,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "TYPE", 0, "BRANDED_FOOD", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("BRANDED_FOOD"),
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "DATA_SOURCE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("DATA_SOURCE"),
					String: food.DataSource,
				},
			},
		},
	}

	if s := strings.TrimSpace(food.BrandedFoodCategory); s != "" {
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "CATEGORY", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "PUBLICATION_DATE", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "AVAILABLE_DATE", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "MODIFIED_DATE", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "DISCONTINUED_DATE", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "DESCRIPTION", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "INGREDIENTS", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "DESCRIPTION", 1),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "CAFFEINE_STATEMENT", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "MARKET_COUNTRY", 0),
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
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "TRADE_CHANNEL", i),
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
						ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "TRADE_CHANNEL", i),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "BRAND_OWNER", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "BRAND_NAME", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "SUBBRAND_NAME", 0),
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
			ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "SERVING_SIZE", 0),
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
				ID:         document.GetID(NameSpaceFood, "BRANDED_FOOD", food.FDCID, "SERVING_SIZE_DESCRIPTION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("SERVING_SIZE"),
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

func index(config *Config) errors.E {
	ctx, stop, httpClient, store, esClient, esProcessor, errE := es.Standalone(
		config.Logger, string(config.Postgres.URL), config.Elastic.URL, config.Postgres.Schema, config.Elastic.Index, config.Elastic.SizeField,
	)
	if errE != nil {
		return errE
	}
	defer stop()

	foods, errE := getFoods(ctx, httpClient, config.Logger, config.CacheDir, config.DataURL)
	if errE != nil {
		return errE
	}

	count := x.Counter(0)
	progress := es.Progress(config.Logger, esProcessor, nil, nil, "indexing")
	ticker := x.NewTicker(ctx, &count, int64(len(document.CoreProperties))+int64(len(foods)), progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	errE = peerdb.SaveCoreProperties(ctx, config.Logger, store, esClient, esProcessor, config.Elastic.Index)
	if errE != nil {
		return errE
	}

	for _, food := range foods {
		if ctx.Err() != nil {
			break
		}

		ingredients, errE := getIngredients(config.IngredientsDir, food)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}

		doc, errE := makeDoc(food, ingredients)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}

		count.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}
	}

	return nil
}