package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

const (
	KilowattHoursToJoules = 3.6e6
)

/*
We are defining a custom struct, Null, to address fields passed by the EPREL API whose values are currently always null so we do not know how to best type them.
Upon unmarshaling, the Null struct will automatically check if the field is null or not. That means that once we actually get non-null data from the API,
we'll be notified about it and can change the field type from Null to what it actually is.
*/
type Null struct{}

var (
	// Assertions also silence this lint error: https://github.com/mvdan/unparam/issues/52
	_ json.Unmarshaler = (*Null)(nil)
	_ json.Marshaler   = (*Null)(nil)
)

func (n *Null) UnmarshalJSON(data []byte) error {
	if !bytes.Equal(data, []byte("null")) {
		return errors.New("only null value is excepted")
	}
	return nil
}

func (n Null) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

/*
We are defining a custom struct, EnergyClass, so that we can apply custom unmarshaling behavior to it.
The values of the energyClass field in the EPREL API can be 'A', 'B', 'C', 'D', 'E', 'F', 'G'
and 'AP', 'APP', 'APPP', and 'APPPP', where EPREL has replaced '+' with 'P'.
We want to replace 'P' with '+' in our dataset, since that is the correct value.
*/
type EnergyClass string

func (ec *EnergyClass) UnmarshalJSON(data []byte) error {
	var eprelEnergyClass string
	errE := x.UnmarshalWithoutUnknownFields(data, &eprelEnergyClass)
	if errE != nil {
		return errE
	}
	peerDBEnergyClass := strings.ReplaceAll(eprelEnergyClass, "P", "+")
	*ec = EnergyClass(peerDBEnergyClass)
	return nil
}

/*
We are defining a custom struct, Status, so that we can make sure that the status field in the EPREL API is always "PUBLISHED".
If the status is not "PUBLISHED", an error will be returned so we know we'll need to look into this again.
*/
type Status string

func (s *Status) UnmarshalJSON(data []byte) error {
	var eprelStatus string
	errE := x.UnmarshalWithoutUnknownFields(data, &eprelStatus)
	if errE != nil {
		return errE
	}
	if eprelStatus != "PUBLISHED" {
		return errors.New("status is not PUBLISHED")
	}
	*s = Status(eprelStatus)
	return nil
}

/*
We are defining a custom struct, TrademarkVerificationStatus, so that we can make
sure that the trademarkVerificationStatus field in the EPREL API is always "VERIFIED".
If the status is not "VERIFIED", an error will be returned so we know we'll
need to look into this again.
*/
type TrademarkVerificationStatus string

func (tvs *TrademarkVerificationStatus) UnmarshalJSON(data []byte) error {
	var eprelTrademarkVerificationStatus string
	errE := x.UnmarshalWithoutUnknownFields(data, &eprelTrademarkVerificationStatus)
	if errE != nil {
		return errE
	}
	if eprelTrademarkVerificationStatus != "VERIFIED" {
		return errors.New("trademark verification status is not VERIFIED")
	}
	*tvs = TrademarkVerificationStatus(eprelTrademarkVerificationStatus)
	return nil
}

/*
We are defining a custom struct, EpochTime, so that we can
unmarshall the timestamp fields in the EPREL API from epochs to dates.
*/

type EpochTime time.Time

// Silence "error is always nil" lint error for the MarshalJSON() method.
var _ json.Marshaler = (*EpochTime)(nil)

func (et EpochTime) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(time.Time(et).Unix(), 10)), nil
}

func (et *EpochTime) UnmarshalJSON(data []byte) error {
	i, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return errors.WithStack(err)
	}
	*(*time.Time)(et) = time.Unix(i, 0)
	return nil
}

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
	// TODO: Map Country to MARKET_COUNTRY existing property claim. See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2358255193.
	Country     string `json:"country"`
	OrderNumber int    `json:"orderNumber"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type WasherDrierProduct struct {
	// TODO: Map all timestamp fields to a custom type. See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2357945179.
	AllowEPRELLabelGeneration bool `json:"allowEprelLabelGeneration"` // Did not map this field, as we will not use it.
	Blocked                   bool `json:"blocked"`                   // Did not map this field, as we will not use it.
	// TODO: Move ContactDetails to a separate document.
	ContactDetails ContactDetails `json:"contactDetails"` // Did not map this field, as we will not use it.
	EPRELContactID int64          `json:"contactId,omitempty"`
	// TODO: Move cycles to a separate document.
	Cycles                     []Cycle `json:"cycles"`
	EcoLabel                   bool    `json:"ecoLabel"` // Did not map this field, as we will not use it.
	EcoLabelRegistrationNumber string  `json:"ecoLabelRegistrationNumber"`

	EnergyAnnualWash          float64     `json:"energyAnnualWash"`
	EnergyAnnualWashAndDry    float64     `json:"energyAnnualWashAndDry"`
	EnergyClass               EnergyClass `json:"energyClass"`
	EnergyClassImage          string      `json:"energyClassImage"`
	EnergyClassImageWithScale string      `json:"energyClassImageWithScale"`
	EnergyClassRange          string      `json:"energyClassRange"`
	EnergyLabelID             int         `json:"energyLabelId"`

	EPRELRegistrationNumber       string `json:"eprelRegistrationNumber"`
	ExportDateTimestamp           int64  `json:"exportDateTS"`
	FirstPublicationDate          []int  `json:"firstPublicationDate"`   // Not mapping, see: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.
	FirstPublicationDateTimestamp int64  `json:"firstPublicationDateTS"` // Not mapping, see: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.
	FormType                      string `json:"formType"`               // Did not map this field, as we will not use it.
	GeneratedLabels               Null   `json:"generatedLabels"`        // Did not map this field, as we will not use it.

	ImplementingAct string `json:"implementingAct"`
	ImportedOn      int64  `json:"importedOn"`
	LastVersion     bool   `json:"lastVersion"` // Did not map this field, as we will not use it.
	ModelIdentifier string `json:"modelIdentifier"`

	NoiseDry  float64 `json:"noiseDry"`
	NoiseSpin float64 `json:"noiseSpin"`
	NoiseWash float64 `json:"noiseWash"`

	OnMarketEndDate                 []int     `json:"onMarketEndDate"` // Not mapping, as using the TS version of this field.
	OnMarketEndDateTimestamp        EpochTime `json:"onMarketEndDateTS"`
	OnMarketFirstStartDate          []int     `json:"onMarketFirstStartDate"`   // Not mapping, see https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.
	OnMarketFirstStartDateTimestamp int64     `json:"onMarketFirstStartDateTS"` // Not mapping, see https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.
	OnMarketStartDate               []int     `json:"onMarketStartDate"`        // Not mapping, as using the TS version of this field.
	OnMarketStartDateTimestamp      EpochTime `json:"onMarketStartDateTS"`
	// TODO: OrgVerificationStatus - We may add this to the org/company/contact doc in the future. https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2424837827
	OrgVerificationStatus string `json:"orgVerificationStatus"`
	// TODO: Map Organisation to a separate document.
	Organisation       Organisation       `json:"organisation"` // Not mapped, as we will not use it.
	OtherIdentifiers   []OtherIdentifiers `json:"otherIdentifiers"`
	PlacementCountries []PlacementCountry `json:"placementCountries"`

	ProductGroup             string `json:"productGroup"`
	ProductModelCoreID       int    `json:"productModelCoreId"`
	PublishedOnDate          []int  `json:"publishedOnDate"`   // Not mapping, see https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.
	PublishedOnDateTimestamp int64  `json:"publishedOnDateTS"` // Not mapping, see https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072.

	// TODO: Map RegistrantNature to organization/company/contacts doc.
	RegistrantNature            string                      `json:"registrantNature"` // Not mapping this as we will not use it in this doc.
	Status                      Status                      `json:"status"`           // Status is always "PUBLISHED", not mapping as this is not useful to us.
	SupplierOrTrademark         string                      `json:"supplierOrTrademark"`
	TrademarkID                 int                         `json:"trademarkId"`
	TrademarkOwner              Null                        `json:"trademarkOwner,omitempty"`    // Value is always NIL, not mapping as this is not useful to us.
	TrademarkVerificationStatus TrademarkVerificationStatus `json:"trademarkVerificationStatus"` // Values is always VERIFIED, not mapping as this not useful to us.

	UploadedLabels []string `json:"uploadedLabels"`
	VersionID      int      `json:"versionId"` // Not mapped as we will not use this field.
	// In theory, VersionNumber should probably be an integer, but we observe float values (3.001, 1.001), so we leave it as float.
	VersionNumber                                     float64 `json:"versionNumber"`  // Not mapped as we will not use this field.
	VisibleToUnitedKingdomMarketSurveillanceAuthority bool    `json:"visibleToUkMsa"` // Not mapped as we will not use this field.

	WaterAnnualWash       float64 `json:"waterAnnualWash"`
	WaterAnnualWashAndDry float64 `json:"waterAnnualWashAndDry"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type ContactDetails struct {
	Address              string `json:"addressBloc,omitempty"`
	City                 string `json:"city"`
	ContactByReferenceID Null   `json:"contactByReferenceId"`
	ContactReference     string `json:"contactReference"`
	Country              string `json:"country"`
	DefaultContact       bool   `json:"defaultContact"`
	Email                string `json:"email"`
	ID                   int    `json:"id"`
	Municipality         string `json:"municipality,omitempty"`
	OrderNumber          Null   `json:"orderNumber"`
	Phone                string `json:"phone"`
	PostalCode           string `json:"postalCode"`
	Province             string `json:"province,omitempty"`
	ServiceName          string `json:"serviceName"`
	Status               string `json:"status"`
	Street               string `json:"street"`
	StreetNumber         string `json:"streetNumber"`
	WebSiteURL           string `json:"webSiteURL,omitempty"`
}

//nolint:tagliatelle // JSON tags must match external EPREL API format.
type Cycle struct {
	CapacityDry                 float64 `json:"capacityDry"`
	CapacityWash                float64 `json:"capacityWash"`
	EnergyConsumptionWash       float64 `json:"energyConsWash"`
	EnergyConsumptionWashAndDry float64 `json:"energyConsWashAndDry"`
	ID                          int     `json:"id"`
	OrderNumber                 int     `json:"orderNumber"`
	OtherCycle                  bool    `json:"otherCycle"`
	OtherCycleLabel             string  `json:"otherCycleLabel,omitempty"`
	SpinMax                     float64 `json:"spinMax"`
	WashTime                    float64 `json:"washTime"`
	WashingPerformanceClass     string  `json:"washingPerformanceClass"`
	WaterConsumptionWashAndDry  float64 `json:"waterConsWD"`
	WaterConsumptionWash        float64 `json:"waterConsWash"`
	WaterExtractionEfficiency   float64 `json:"waterExtractionEff"`
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

type OtherIdentifiers struct {
	OrderNumber     int    `json:"orderNumber"`
	ModelIdentifier string `json:"modelIdentifier"`
	Type            string `json:"type"`
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
		defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

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
		errE := errors.New("unexpected number of washer driers")
		errors.Details(errE)["expected"] = totalSize
		errors.Details(errE)["got"] = len(allWasherDriers)
		return nil, errE
	}

	return allWasherDriers, nil
}

//nolint:maintidx // Reason: function is large but logically cohesive and tested
func makeWasherDrierDoc(washerDrier WasherDrierProduct) (document.D, errors.E) {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber),
			Score: document.HighConfidence,
		},
		Claims: &document.ClaimTypes{
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "TYPE", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("WASHER_DRIER"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "NAME", 0),
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
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_CLASS_IMAGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ENERGY_CLASS_IMAGE"),
					MediaType: "image/svg+xml",
					URL:       "https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" + washerDrier.EnergyClassImage,
					Preview:   []string{"https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" + washerDrier.EnergyClassImage},
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_CLASS_IMAGE_WITH_SCALE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ENERGY_CLASS_IMAGE_WITH_SCALE"),
					MediaType: "image/svg+xml",
					URL:       "https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" + washerDrier.EnergyClassImageWithScale,
					Preview:   []string{"https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/" + washerDrier.EnergyClassImageWithScale},
				},
			},
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "EPREL_REGISTRATION_NUMBER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("EPREL_REGISTRATION_NUMBER"),
					Value: washerDrier.EPRELRegistrationNumber,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "MODEL_IDENTIFIER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("MODEL_IDENTIFIER"),
					Value: washerDrier.ModelIdentifier,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_LABEL_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("ENERGY_LABEL_ID"),
					Value: strconv.Itoa(washerDrier.EnergyLabelID),
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "SUPPLIER_OR_TRADEMARK", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("SUPPLIER_OR_TRADEMARK"),
					String: washerDrier.SupplierOrTrademark,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_CLASS", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_CLASS"),
					String: string(washerDrier.EnergyClass),
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_CLASS_RANGE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_CLASS_RANGE"),
					String: washerDrier.EnergyClassRange,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "IMPLEMENTING_ACT", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("IMPLEMENTING_ACT"),
					String: washerDrier.ImplementingAct,
				},
			},
			Amount: document.AmountClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_ANNUAL_WASH", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_ANNUAL_WASH"),
					Amount: washerDrier.EnergyAnnualWash * KilowattHoursToJoules,
					Unit:   document.AmountUnitJoule,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ENERGY_ANNUAL_WASH_AND_DRY", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("ENERGY_ANNUAL_WASH_AND_DRY"),
					Amount: washerDrier.EnergyAnnualWashAndDry * KilowattHoursToJoules,
					Unit:   document.AmountUnitJoule,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "WATER_ANNUAL_WASH", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("WATER_ANNUAL_WASH"),
					Amount: washerDrier.WaterAnnualWash,
					Unit:   document.AmountUnitLitre,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "WATER_ANNUAL_WASH_AND_DRY", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("WATER_ANNUAL_WASH_AND_DRY"),
					Amount: washerDrier.WaterAnnualWashAndDry,
					Unit:   document.AmountUnitLitre,
				},
			},
			Time: document.TimeClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ON_MARKET_START_DATE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ON_MARKET_START_DATE"),
					Timestamp: document.Timestamp(washerDrier.OnMarketStartDateTimestamp),
					Precision: document.TimePrecisionDay,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ON_MARKET_END_DATE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ON_MARKET_END_DATE"),
					Timestamp: document.Timestamp(washerDrier.OnMarketEndDateTimestamp),
					Precision: document.TimePrecisionDay,
				},
			},
		},
	}

	if s := strings.TrimSpace(washerDrier.EcoLabelRegistrationNumber); s != "" {
		errE := doc.Add(&document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ECOLABEL_REGISTRATION_NUMBER", 0),
				Confidence: document.HighConfidence,
			},
			Prop:  document.GetCorePropertyReference("ECOLABEL_REGISTRATION_NUMBER"),
			Value: washerDrier.EcoLabelRegistrationNumber,
		})
		if errE != nil {
			return doc, errE
		}
	}

	// We assume EPRELContactID values start at 1 and continue up. Otherwise, we would need to change the field definition to pointer int (*int).
	if washerDrier.EPRELContactID != 0 {
		errE := doc.Add(&document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "EPREL_CONTACT_ID", 0),
				Confidence: document.HighConfidence,
			},
			Prop:  document.GetCorePropertyReference("EPREL_CONTACT_ID"),
			Value: strconv.Itoa(int(washerDrier.EPRELContactID)),
		})
		if errE != nil {
			return doc, errE
		}
	}

	if washerDrier.NoiseDry > 0 {
		errE := doc.Add(&document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "NOISE_DRY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("NOISE_DRY"),
			Amount: washerDrier.NoiseDry,
			Unit:   document.AmountUnitDecibel,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if washerDrier.NoiseSpin > 0 {
		errE := doc.Add(&document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "NOISE_SPIN", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("NOISE_SPIN"),
			Amount: washerDrier.NoiseSpin,
			Unit:   document.AmountUnitDecibel,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if washerDrier.NoiseWash > 0 {
		errE := doc.Add(&document.AmountClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "NOISE_WASH", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("NOISE_WASH"),
			Amount: washerDrier.NoiseWash,
			Unit:   document.AmountUnitDecibel,
		})
		if errE != nil {
			return doc, errE
		}
	}

	return doc, nil
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
			Str("id", washerDrier.EPRELRegistrationNumber).
			Msg("processing EPREL washer-driers record")

		doc, errE := makeWasherDrierDoc(washerDrier)
		if errE != nil {
			errors.Details(errE)["id"] = washerDrier.EPRELRegistrationNumber
			return errE
		}

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = washerDrier.EPRELRegistrationNumber
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", len(washerDriers)).
		Msg(description + " done")

	return nil
}
