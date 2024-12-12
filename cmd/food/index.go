package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/krolaw/zipstream"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

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
	FoodClass                string          `json:"foodClass"`
	Description              string          `json:"description"`
	FoodNutrients            []FoodNutrient  `json:"foodNutrients"`
	FoodAttributes           []FoodAttribute `json:"foodAttributes"`
	ModifiedDate             string          `json:"modifiedDate"`
	AvailableDate            string          `json:"availableDate"`
	DiscontinuedDate         string          `json:"discontinuedDate,omitempty"`
	MarketCountry            string          `json:"marketCountry"`
	BrandOwner               string          `json:"brandOwner"`
	BrandName                string          `json:"brandName,omitempty"`
	SubbrandName             string          `json:"subbrandName,omitempty"`
	GTIN                     string          `json:"gtinUpc"`
	DataSource               string          `json:"dataSource"`
	Ingredients              string          `json:"ingredients"`
	ServingSize              float64         `json:"servingSize"`
	ServingSizeUnit          string          `json:"servingSizeUnit"`
	HouseholdServingFullText string          `json:"householdServingFullText"`
	LabelNutrients           LabelNutrients  `json:"labelNutrients"`
	PackageWeight            string          `json:"packageWeight,omitempty"`
	TradeChannels            []string        `json:"tradeChannels"`
	Microbes                 []Microbe       `json:"microbes"`
	GPCClassCode             int             `json:"gpcClassCode,omitempty"`
	PreparationStateCode     string          `json:"preparationStateCode,omitempty"`
	ShortDescription         string          `json:"shortDescription,omitempty"`
	CaffeineStatement        string          `json:"caffeineStatement,omitempty"`
	BrandedFoodCategory      string          `json:"brandedFoodCategory"`
	DataType                 string          `json:"dataType"`
	FDCID                    int             `json:"fdcId"`
	PublicationDate          string          `json:"publicationDate"`
	FoodUpdateLog            []FoodUpdate    `json:"foodUpdateLog"`
}

type Ingredient struct {
	Name       string       `json:"name"`
	Ingredient []Ingredient `json:"ingredients,omitempty"`
	Meta       []string     `json:"meta,omitempty"`
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

func index(config *Config) errors.E {
	ctx, stop, httpClient, _, _, _, errE := es.Standalone( //nolint:dogsled
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

	for _, food := range foods {
		_, errE := getIngredients(config.IngredientsDir, food)
		if errE != nil {
			errors.Details(errE)["id"] = food.FDCID
			return errE
		}
	}

	return nil
}
