package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/eprel"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	KilowattHoursToJoules = 3.6e6
)

// Null is used to address fields returned by the EPREL API whose values are currently always null
// so we do not know what values they could have and cannot map them to better Go types.
// Upon JSON unmarshaling, the Null struct will automatically check if the field is null or not.
// That means that if we ever get non-null data from the API, JSON unmarshaling will fail and we will
// be notified about it and can change the field type from Null to what it actually is.
type Null struct{}

var (
	// Silence the lint error.
	// See: https://github.com/mvdan/unparam/issues/52
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

type EPREL struct {
	Disabled bool                 `default:"false"                          help:"Do not import EPREL data. Default: false."`
	APIKey   kong.FileContentFlag `                env:"EPREL_API_KEY_PATH" help:"File with EPREL API key. Environment variable: ${env}." placeholder:"PATH" required:""`
}

//nolint:tagliatelle
type ProductGroup struct {
	Code       string `json:"code"`
	URLCode    string `json:"url_code"`
	Name       string `json:"name"`
	Regulation string `json:"regulation"`
}

type PlacementCountry struct {
	// TODO: Map Country to MARKET_COUNTRY existing property claim.
	// 			 See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2358255193.
	Country     string `json:"country"`
	OrderNumber int    `json:"orderNumber"`
}

//nolint:tagliatelle
type WasherDrierProduct struct {
	// TODO: Map all timestamp fields to a custom type.
	// 			 See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2357945179.

	// Not mapping this field, as we do not use it.
	AllowEPRELLabelGeneration bool `json:"allowEprelLabelGeneration"`
	// Not mapping this field, as we do not use it.
	Blocked bool `json:"blocked"`
	// TODO: Move ContactDetails to a separate document.
	//       See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2424229750
	ContactDetails ContactDetails `json:"contactDetails"`
	// TODO: Move EPRELContactID together with ContactDetails to a separate document.
	EPRELContactID int64 `json:"contactId,omitempty"`
	// TODO: Move cycles to a separate document.
	Cycles []Cycle `json:"cycles"`
	// Not mapping this field, as we do not use it.
	EcoLabel                   bool   `json:"ecoLabel"`
	EcoLabelRegistrationNumber string `json:"ecoLabelRegistrationNumber"`

	EnergyAnnualWash          float64 `json:"energyAnnualWash"`
	EnergyAnnualWashAndDry    float64 `json:"energyAnnualWashAndDry"`
	EnergyClass               string  `json:"energyClass"`
	EnergyClassImage          string  `json:"energyClassImage"`
	EnergyClassImageWithScale string  `json:"energyClassImageWithScale"`
	// TODO: Use the range to normalize the EnergyClass value?
	//       It is a range of possible classes at the time the class has been assigned.
	//       See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2412120710
	EnergyClassRange string `json:"energyClassRange"`
	EnergyLabelID    int    `json:"energyLabelId"`

	EPRELRegistrationNumber string `json:"eprelRegistrationNumber"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	ExportDateTimestamp int64 `json:"exportDateTS"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	FirstPublicationDate []int `json:"firstPublicationDate"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	FirstPublicationDateTimestamp int64 `json:"firstPublicationDateTS"`
	// Not mapping this field, as we do not use it.
	FormType string `json:"formType"`
	// Not mapping this field, as we do not use it.
	GeneratedLabels Null `json:"generatedLabels"`

	ImplementingAct string `json:"implementingAct"`
	ImportedOn      int64  `json:"importedOn"`
	// Value is always true. Not mapping this field as this is not useful.
	LastVersion     bool   `json:"lastVersion"`
	ModelIdentifier string `json:"modelIdentifier"`

	NoiseDry  float64 `json:"noiseDry"`
	NoiseSpin float64 `json:"noiseSpin"`
	NoiseWash float64 `json:"noiseWash"`

	// Not mapping this field, as we use the TS version of this field.
	OnMarketEndDate          []int `json:"onMarketEndDate"`
	OnMarketEndDateTimestamp int64 `json:"onMarketEndDateTS"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	OnMarketFirstStartDate []int `json:"onMarketFirstStartDate"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	OnMarketFirstStartDateTimestamp int64 `json:"onMarketFirstStartDateTS"`
	// Not mapping this field, as we use the TS version of this field.
	OnMarketStartDate          []int `json:"onMarketStartDate"`
	OnMarketStartDateTimestamp int64 `json:"onMarketStartDateTS"`
	// TODO: We may add this to the org/company/contact document in the future.
	//       See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2424837827
	OrgVerificationStatus string `json:"orgVerificationStatus"`
	// TODO: Map Organisation to a separate document.
	//       See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2424229750
	Organisation       Organisation       `json:"organisation"`
	OtherIdentifiers   []OtherIdentifiers `json:"otherIdentifiers"`
	PlacementCountries []PlacementCountry `json:"placementCountries"`

	ProductGroup string `json:"productGroup"`
	// TODO: Figure out what this field is.
	ProductModelCoreID int `json:"productModelCoreId"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	PublishedOnDate []int `json:"publishedOnDate"`
	// Not mapping because it is internal to EPREL publishing process.
	// See: https://gitlab.com/peerdb/peerdb/-/merge_requests/3#note_2502211072
	PublishedOnDateTimestamp int64 `json:"publishedOnDateTS"`

	// TODO: Map RegistrantNature to organization/company/contacts document.
	RegistrantNature string `json:"registrantNature"`
	// Value is always "PUBLISHED". Not mapping this field as this is not useful.
	Status              string `json:"status"`
	SupplierOrTrademark string `json:"supplierOrTrademark"`
	// This is an EPREL internal ID, so it is not useful. Not mapping.
	TrademarkID int `json:"trademarkId"`
	// Value is always null. Not mapping this field as this is not useful.
	TrademarkOwner Null `json:"trademarkOwner,omitempty"`
	// Value is always "VERIFIED". Not mapping this field as this is not useful.
	TrademarkVerificationStatus string `json:"trademarkVerificationStatus"`

	UploadedLabels []string `json:"uploadedLabels"`
	// Not mapping this field, as we do not use it.
	VersionID int `json:"versionId"`
	// In theory, VersionNumber should probably be an integer, but we observe float values (3.001, 1.001), so we leave it as float.
	// Not mapping this field, as we do not use it.
	VersionNumber float64 `json:"versionNumber"`
	// Not mapping this field, as we do not use it.
	VisibleToUnitedKingdomMarketSurveillanceAuthority bool `json:"visibleToUkMsa"`

	WaterAnnualWash       float64 `json:"waterAnnualWash"`
	WaterAnnualWashAndDry float64 `json:"waterAnnualWashAndDry"`
}

//nolint:tagliatelle
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

//nolint:tagliatelle
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
	url := "https://eprel.ec.europa.eu/api/product-groups"

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = url
		return nil, errE
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = url
	}
	defer resp.Body.Close()

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

//nolint:maintidx
func makeWasherDrierDoc(washerDrier WasherDrierProduct) (document.D, errors.E) {
	if !washerDrier.LastVersion {
		// Currently last version is always true in EPREL API responses.
		return document.D{}, errors.New("last version is false")
	}
	if washerDrier.Status != "PUBLISHED" {
		// Currently status is always "PUBLISHED" in EPREL API responses.
		return document.D{}, errors.New("status is not PUBLISHED")
	}
	if washerDrier.TrademarkVerificationStatus != "VERIFIED" {
		// Currently trademark verification status is always "VERIFIED" in EPREL API responses.
		return document.D{}, errors.New("trademark verification status is not VERIFIED")
	}

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
					String: strings.ReplaceAll(washerDrier.EnergyClass, "P", "+"),
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "IMPLEMENTING_ACT", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("IMPLEMENTING_ACT"),
					String: washerDrier.ImplementingAct,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "PRODUCT_GROUP", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("CATEGORY"),
					String: washerDrier.ProductGroup,
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
					Timestamp: document.Timestamp(time.Unix(washerDrier.OnMarketStartDateTimestamp, 0)),
					Precision: document.TimePrecisionDay,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "ON_MARKET_END_DATE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:      document.GetCorePropertyReference("ON_MARKET_END_DATE"),
					Timestamp: document.Timestamp(time.Unix(washerDrier.OnMarketEndDateTimestamp, 0)),
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

	for i, placementCountry := range washerDrier.PlacementCountries {
		country := strings.TrimSpace(placementCountry.Country)
		if country != "" {
			errE := doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "PLACEMENT_COUNTRY", i),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("MARKET_COUNTRY"),
				String: country,
			})
			if errE != nil {
				return doc, errE
			}
		}
	}

	for i, uploadedLabel := range washerDrier.UploadedLabels {
		uploadedLabel = strings.TrimSpace(uploadedLabel)

		if uploadedLabel != "" {
			var mediaType string
			if strings.HasSuffix(strings.ToLower(uploadedLabel), ".pdf") {
				mediaType = "application/pdf"
			} else if strings.HasSuffix(strings.ToLower(uploadedLabel), ".jpg") ||
				strings.HasSuffix(strings.ToLower(uploadedLabel), ".jpeg") {
				mediaType = "image/jpeg"
			} else if strings.HasSuffix(strings.ToLower(uploadedLabel), ".png") {
				mediaType = "image/png"
			} else if strings.HasSuffix(strings.ToLower(uploadedLabel), ".svg") {
				mediaType = "image/svg+xml"
			} else {
				/* There are some weird file extensions, that are probably errors. We shouldn't add them as claims as they don't represent valid data.
				'.',
				'. d39h500x1cw_10131230_(ku-epr)_60x125_cbox_',
				'.50583513_17mb211s 24550lb andersson led2445hdsdvd_10129099_(ku-epr)_60x125_cbox_',
				'.50591146_17mb140tc 32552dlb techwood 32hdtet2s2_10131635_(ku-epr)_60x125_cbox_sp',
				'.50596124_17mb181tc 32553dlb elbe xtv-3203-wifi_10131759_(ku-epr)_60x125_cbox_spe',
				'.50597390_17mb181tc 32550dlb kendo 32 led 5211 w_10131822_(ku-epr)_60x125_cbox_sp',
				'.50597450_17mb181tc 22502lb finlux 22-fwdf-5161_10131837_(ku-epr)_60x125_cbox_spe',
				'.50598402_17mb181tc 24550lb finlux 24-fwe-5760_10131910_(ku-epr)_60x125_cbox_spek',
				'.ai',
				'.funken d43u551n1cw_10128456_(ku-epr)_60x125_cbox_spe',
				'.funken d50v900m4cwh_10128899_(ku-epr)_96x200_cbox_spe',
				'.funken tfa55u550uhd_10131605_(ku-epr)_96x200_cbox_sp',
				'.jfif',
				'.p',
				'.pd'
				*/
				continue
			}
			errE := doc.Add(&document.FileClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "UPLOADED_LABEL", i),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("UPLOADED_LABEL"),
				MediaType: mediaType,
				URL:       "https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel,
				Preview:   []string{"https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel},
			})
			if errE != nil {
				return doc, errE
			}
		}
	}

	for i, otherIdentifier := range washerDrier.OtherIdentifiers {
		if strings.TrimSpace(otherIdentifier.ModelIdentifier) != "" {
			errE := doc.Add(&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "EPREL_OTHER_IDENTIFIER", i),
					Confidence: document.HighConfidence,
					// Add metadata about the identifier type.
					Meta: &document.ClaimTypes{
						String: document.StringClaims{
							{
								CoreClaim: document.CoreClaim{
									ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", washerDrier.EPRELRegistrationNumber, "EPREL_OTHER_IDENTIFIER", i, "TYPE", 0),
									Confidence: document.HighConfidence,
								},
								Prop:   document.GetCorePropertyReference("EPREL_OTHER_IDENTIFIER_TYPE"),
								String: otherIdentifier.Type,
							},
						},
					},
				},
				Prop:  document.GetCorePropertyReference("EPREL_OTHER_IDENTIFIER"),
				Value: otherIdentifier.ModelIdentifier,
			})
			if errE != nil {
				return doc, errE
			}
		}
	}

	return doc, nil
}

func (e EPREL) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
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

	washerDriers, errE := eprel.GetWasherDriers[WasherDrierProduct](ctx, httpClient, apiKey)
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
