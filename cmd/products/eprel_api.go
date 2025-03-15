package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

type EPREL struct {
	Disabled bool                 `default:"false"                          help:"Do not import EPREL data. Default: false."`
	APIKey   kong.FileContentFlag `                env:"EPREL_API_KEY_PATH" help:"File with EPREL API key. Environment variable: ${env}." placeholder:"PATH" required:""`
}

type WasherDrierResponse struct {
	Size   int                  `json:"size"`
	Offset int                  `json:"offset"`
	Hits   []WasherDrierProduct `json:"hits"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type ProductGroup struct {
	Code       string `json:"code"`
	URLCode    string `json:"url_code"`
	Name       string `json:"name"`
	Regulation string `json:"regulation"`
}

type PlacementCountry struct {
	// TODO: Map Country to MARKET_COUNTRY existing property claim. See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2358255193
	Country     string `json:"country"`
	OrderNumber int    `json:"orderNumber"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type WasherDrierProduct struct {
	// TODO: Map all timestamp fields to a custom type. See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2357945179
	AllowEprelLabelGeneration bool           `json:"allowEprelLabelGeneration"`
	Blocked                   bool           `json:"blocked"`
	ContactDetails            ContactDetails `json:"contactDetails"`
	ContactID                 int            `json:"contactId"`
	Cycles                    []Cycle        `json:"cycles"`

	EcoLabel                   bool   `json:"ecoLabel"`
	EcoLabelRegistrationNumber string `json:"ecoLabelRegistrationNumber"`

	EnergyAnnualWash          float64 `json:"energyAnnualWash"`
	EnergyAnnualWashAndDry    float64 `json:"energyAnnualWashAndDry"`
	EnergyClass               string  `json:"energyClass"`
	EnergyClassImage          string  `json:"energyClassImage"`
	EnergyClassImageWithScale string  `json:"energyClassImageWithScale"`
	EnergyClassRange          string  `json:"energyClassRange"`
	EnergyLabelID             int     `json:"energyLabelId"`

	EprelRegistrationNumber       string      `json:"eprelRegistrationNumber"`
	ExportDateTimestamp           int64       `json:"exportDateTS"`
	FirstPublicationDate          []int       `json:"firstPublicationDate"`
	FirstPublicationDateTimestamp int64       `json:"firstPublicationDateTS"`
	FormType                      string      `json:"formType"`
	GeneratedLabels               interface{} `json:"generatedLabels,omitempty"`

	ImplementingAct string `json:"implementingAct"`
	ImportedOn      int64  `json:"importedOn"`
	LastVersion     bool   `json:"lastVersion"`
	ModelIdentifier string `json:"modelIdentifier"`

	NoiseDry  float64 `json:"noiseDry"`
	NoiseSpin float64 `json:"noiseSpin"`
	NoiseWash float64 `json:"noiseWash"`

	OnMarketEndDate                 []int `json:"onMarketEndDate"`
	OnMarketEndDateTimestamp        int64 `json:"onMarketEndDateTS"`
	OnMarketFirstStartDate          []int `json:"onMarketFirstStartDate"`
	OnMarketFirstStartDateTimestamp int64 `json:"onMarketFirstStartDateTS"`
	OnMarketStartDate               []int `json:"onMarketStartDate"`
	OnMarketStartDateTimestamp      int64 `json:"onMarketStartDateTS"`

	OrgVerificationStatus string       `json:"orgVerificationStatus"`
	Organisation          Organisation `json:"organisation"`
	// TODO: We do not know the real type here.
	//       So we are using []string to let it blow up once we encounter a product with other identifiers.
	OtherIdentifiers   []string           `json:"otherIdentifiers,omitempty"`
	PlacementCountries []PlacementCountry `json:"placementCountries,omitempty"`

	ProductGroup             string `json:"productGroup"`
	ProductModelCoreID       int    `json:"productModelCoreId"`
	PublishedOnDate          []int  `json:"publishedOnDate"`
	PublishedOnDateTimestamp int64  `json:"publishedOnDateTS"`

	RegistrantNature            string      `json:"registrantNature"`
	Status                      string      `json:"status"`
	SupplierOrTrademark         string      `json:"supplierOrTrademark"`
	TrademarkID                 int         `json:"trademarkId"`
	TrademarkOwner              interface{} `json:"trademarkOwner,omitempty"`
	TrademarkVerificationStatus string      `json:"trademarkVerificationStatus"`

	UploadedLabels                                    []string `json:"uploadedLabels"`
	VersionID                                         int      `json:"versionId"`
	VersionNumber                                     float64  `json:"versionNumber"`
	VisibleToUnitedKingdomMarketSurveillanceAuthority bool     `json:"visibleToUkMsa"`

	WaterAnnualWash       float64 `json:"waterAnnualWash"`
	WaterAnnualWashAndDry float64 `json:"waterAnnualWashAndDry"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type ContactDetails struct {
	Address              string      `json:"addressBloc,omitempty"`
	City                 string      `json:"city"`
	ContactByReferenceID interface{} `json:"contactByReferenceId,omitempty"`
	ContactReference     string      `json:"contactReference"`
	Country              string      `json:"country"`
	DefaultContact       bool        `json:"defaultContact"`
	Email                string      `json:"email"`
	ID                   int         `json:"id"`
	Municipality         string      `json:"municipality,omitempty"`
	OrderNumber          string      `json:"orderNumber,omitempty"`
	Phone                string      `json:"phone"`
	PostalCode           string      `json:"postalCode"`
	Province             string      `json:"province,omitempty"`
	ServiceName          string      `json:"serviceName"`
	Status               string      `json:"status"`
	Street               string      `json:"street"`
	StreetNumber         string      `json:"streetNumber"`
	WebSiteURL           string      `json:"webSiteURL,omitempty"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type Cycle struct {
	CapacityDry             float64 `json:"capacityDry"`
	CapacityWash            float64 `json:"capacityWash"`
	EnergyConsWash          float64 `json:"energyConsWash"`
	EnergyConsWashAndDry    float64 `json:"energyConsWashAndDry"`
	ID                      int     `json:"id"`
	OrderNumber             float64 `json:"orderNumber"`
	OtherCycle              bool    `json:"otherCycle"`
	OtherCycleLabel         string  `json:"otherCycleLabel,omitempty"`
	SpinMax                 float64 `json:"spinMax"`
	WashTime                float64 `json:"washTime"`
	WashingPerformanceClass string  `json:"washingPerformanceClass"`
	WaterConsWD             float64 `json:"waterConsWD"`
	WaterConsWash           float64 `json:"waterConsWash"`
	WaterExtractionEff      float64 `json:"waterExtractionEff"`
}

type Organisation struct {
	CloseDate         string `json:"closeDate,omitempty"`
	CloseStatus       string `json:"closeStatus,omitempty"`
	FirstName         string `json:"firstName,omitempty"`
	IsClosed          bool   `json:"isClosed"`
	LastName          string `json:"lastName,omitempty"`
	OrganisationName  string `json:"organisationName"`
	OrganisationTitle string `json:"organisationTitle"`
	Website           string `json:"website,omitempty"`
}

func getProductGroups(ctx context.Context, httpClient *retryablehttp.Client) ([]string, errors.E) {
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, "https://eprel.ec.europa.eu/api/product-groups", nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	var result []ProductGroup
	errE := x.DecodeJSONWithoutUnknownFields(resp.Body, &result)
	if errE != nil {
		return nil, errE
	}

	urlCodes := make([]string, 0, len(result))
	for _, item := range result {
		urlCodes = append(urlCodes, item.URLCode)
	}

	return urlCodes, nil
}

func getWasherDriers(ctx context.Context, httpClient *retryablehttp.Client, apiKey string) ([]WasherDrierProduct, errors.E) {
	var allWasherDriers []WasherDrierProduct
	limit := 100
	page := 1

	var totalSize int

	for {
		baseURL := "https://eprel.ec.europa.eu/api/products/washerdriers"
		params := url.Values{}
		params.Add("_limit", strconv.Itoa(limit))
		params.Add("_page", strconv.Itoa(page))

		url := fmt.Sprintf("%s?%s", baseURL, params.Encode())

		req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		req.Header.Set("X-Api-Key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer resp.Body.Close()

		var result WasherDrierResponse
		errE := x.DecodeJSONWithoutUnknownFields(resp.Body, &result)
		if errE != nil {
			return nil, errE
		}

		if len(result.Hits) == 0 {
			break
		}

		if page == 1 {
			totalSize = result.Size
		}

		allWasherDriers = append(allWasherDriers, result.Hits...)
		page++
	}

	if len(allWasherDriers) != totalSize {
		return nil, errors.Errorf("expected %d washer-driers but got %d", totalSize, len(allWasherDriers))
	}

	return allWasherDriers, nil
}

func makeWasherDrierDoc(washerDrier WasherDrierProduct) document.D {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber),
			Score: document.HighConfidence,
		},
		Claims: &document.ClaimTypes{
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "TYPE", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("WASHER_DRIER"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{
						"en": html.EscapeString(fmt.Sprintf("%s %s",
							strings.TrimSpace(washerDrier.SupplierOrTrademark),
							strings.TrimSpace(washerDrier.ModelIdentifier))),
					},
				},
			},
			File: document.FileClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "ENERGY_CLASS_IMAGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ENERGY_CLASS_IMAGE"),
					MediaType: "image/svg+xml",
					URL: "https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" +
						url.PathEscape(washerDrier.EnergyClassImage),
					Preview: nil,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "ENERGY_CLASS_IMAGE_WITH_SCALE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ENERGY_CLASS_IMAGE_WITH_SCALE"),
					MediaType: "image/svg+xml",
					URL: "https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" +
						url.PathEscape(washerDrier.EnergyClassImageWithScale),
					Preview: nil,
				},
			},
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "EPREL_REGISTRATION_NUMBER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("EPREL_REGISTRATION_NUMBER"),
					Value: washerDrier.EprelRegistrationNumber,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "MODEL_IDENTIFIER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("MODEL_IDENTIFIER"),
					Value: washerDrier.ModelIdentifier,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "CONTACT_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("CONTACT_ID"),
					Value: strconv.FormatInt(int64(washerDrier.ContactID), 10),
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EprelRegistrationNumber, "ENERGY_LABEL_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("ENERGY_LABEL_ID"),
					Value: strconv.FormatInt(int64(washerDrier.EnergyLabelID), 10),
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EcoLabelRegistrationNumber, "SUPPLIER_OR_TRADEMARK", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("SUPPLIER_OR_TRADEMARK"),
					String: washerDrier.SupplierOrTrademark,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EnergyClass, "ENERGY_CLASS", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_CLASS"),
					String: washerDrier.EnergyClass,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EnergyClassRange, "ENERGY_CLASS_RANGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_CLASS_RANGE"),
					String: washerDrier.EnergyClassRange,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EnergyLabelID, "IMPLEMENTING_ACT", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("IMPLEMENTING_ACT"),
					String: washerDrier.ImplementingAct,
				},
			},
		},
	}

	if s := strings.TrimSpace(washerDrier.EcoLabelRegistrationNumber); s != "" {
		errE := doc.Add(&document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EcoLabelRegistrationNumber, "ECOLABEL_REGISTRATION_NUMBER", 0),
				Confidence: document.HighConfidence,
			},
			Prop:  document.GetCorePropertyReference("ECOLABEL_REGISTRATION_NUMBER"),
			Value: washerDrier.EcoLabelRegistrationNumber,
		})
		if errE != nil {
			return doc
		}
	}

	return doc
}

func (e EPREL) Run(
	ctx context.Context,
	config *Config,
	httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if e.Disabled {
		return nil
	}

	apiKey := strings.TrimSpace(string(e.APIKey))
	if apiKey == "" {
		return errors.New("missing EPREL API key")
	}

	// Check ElasticSearch config.
	config.Logger.Info().
		Str("elastic_url", config.Elastic.URL).
		Str("elastic_index", config.Elastic.Index).
		Msg("ElasticSearch configuration")

	washerDriers, errE := getWasherDriers(ctx, httpClient, apiKey)
	if errE != nil {
		return errE
	}

	config.Logger.Info().Int("count", len(washerDriers)).Msg("retrieved EPREL washer-driers data")

	description := "EPREL washer-driers processing"
	progress := es.Progress(config.Logger, nil, nil, nil, description)
	indexingSize.Add(int64(len(washerDriers)))

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, x.NewCounter(int64(len(washerDriers))), indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	for i, washerDrier := range washerDriers {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		config.Logger.Debug().
			Int("index", i).
			Str("id", washerDrier.EprelRegistrationNumber).
			Msg("processing EPREL washer-driers record")

		doc := makeWasherDrierDoc(washerDrier)

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = washerDrier.EprelRegistrationNumber
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", len(washerDriers)).
		Msg(description + " done")

	return nil
}
