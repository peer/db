package main

import (
	"context"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/eprel"
	"gitlab.com/peerdb/peerdb/internal/es"
)

func TestGetProductGroups(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil

	urlCodes, errE := getProductGroups(ctx, httpClient)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Assert that we got results.
	assert.NotEmpty(t, urlCodes, "product groups list should not be empty")

	// Assert that washerdriers are present.
	// TODO: Add more once we process other product groups.
	expectedGroups := []string{
		"washerdriers",
	}

	for _, expected := range expectedGroups {
		assert.Contains(t, urlCodes, expected, "product groups should contain %s", expected)
	}
}

func getAPIKey(t *testing.T) string {
	t.Helper()

	if os.Getenv("EPREL_API_KEY_PATH") == "" {
		t.Skip("EPREL_API_KEY_PATH is not available")
	}

	key, err := os.ReadFile(os.Getenv("EPREL_API_KEY_PATH"))
	require.NoError(t, err)

	return strings.TrimSpace(string(key))
}

func TestGetWasherDriers(t *testing.T) {
	t.Parallel()

	apiKey := getAPIKey(t)

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	httpClient := es.NewHTTPClient(cleanhttp.DefaultPooledClient(), logger)

	ctx := context.Background()
	washerDriers, errE := eprel.GetWasherDriers[WasherDrierProduct](ctx, httpClient, apiKey)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.NotEmpty(t, washerDriers)
}

func createTestWasherDrier(t *testing.T) WasherDrierProduct {
	t.Helper()

	return WasherDrierProduct{
		EPRELRegistrationNumber:    "132300",
		ModelIdentifier:            "F94J8VH2WD",
		EPRELContactID:             1234,
		EnergyLabelID:              998462,
		EcoLabelRegistrationNumber: "1234",
		EnergyClass:                "APPP",
		EnergyClassImage:           "A-Left-DarkGreen.png",
		EnergyClassImageWithScale:  "A-Left-DarkGreen-WithAGScale.svg",
		EnergyClassRange:           "A_G",
		ImplementingAct:            "EC_96_60",
		SupplierOrTrademark:        "LG Electronics Inc.",
		AllowEPRELLabelGeneration:  false,
		Blocked:                    false,
		ContactDetails: ContactDetails{
			Address:              "",
			City:                 "",
			ContactByReferenceID: Null{},
			ContactReference:     "",
			Country:              "",
			DefaultContact:       false,
			Email:                "",
			ID:                   0,
			Municipality:         "",
			OrderNumber:          Null{},
			Phone:                "",
			PostalCode:           "",
			Province:             "",
			ServiceName:          "",
			Status:               "",
			Street:               "",
			StreetNumber:         "",
			WebSiteURL:           "",
		},
		Cycles:                          []Cycle{},
		EcoLabel:                        false,
		EnergyAnnualWash:                10,
		EnergyAnnualWashAndDry:          10,
		ExportDateTimestamp:             0,
		FirstPublicationDate:            []int{},
		FirstPublicationDateTimestamp:   0,
		FormType:                        "",
		GeneratedLabels:                 Null{},
		ImportedOn:                      0,
		LastVersion:                     false,
		NoiseDry:                        65.0,
		NoiseSpin:                       72.0,
		NoiseWash:                       58.0,
		OnMarketEndDate:                 []int{},
		OnMarketEndDateTimestamp:        EpochTime(time.Unix(1540512000, 0)),
		OnMarketFirstStartDate:          []int{},
		OnMarketFirstStartDateTimestamp: 0,
		OnMarketStartDate:               []int{},
		OnMarketStartDateTimestamp:      EpochTime(time.Unix(1540512000, 0)),
		OrgVerificationStatus:           "",
		Organisation: Organisation{
			CloseDate:         "",
			CloseStatus:       "",
			FirstName:         "",
			IsClosed:          false,
			LastName:          "",
			OrganisationName:  "",
			OrganisationTitle: "",
			Website:           "",
		},
		OtherIdentifiers:            []OtherIdentifiers{},
		PlacementCountries:          []PlacementCountry{},
		ProductGroup:                "",
		ProductModelCoreID:          0,
		PublishedOnDate:             []int{},
		PublishedOnDateTimestamp:    0,
		RegistrantNature:            "",
		Status:                      "PUBLISHED",
		TrademarkID:                 0,
		TrademarkOwner:              Null{},
		TrademarkVerificationStatus: "VERIFIED",
		UploadedLabels:              []string{},
		VersionID:                   0,
		VersionNumber:               0,
		VisibleToUnitedKingdomMarketSurveillanceAuthority: false,
		WaterAnnualWash:       11200,
		WaterAnnualWashAndDry: 21000,
	}
}

type washerDrierTestCase struct {
	name      string
	propName  string
	claimType string
	getValue  func(t *testing.T, claim document.Claim) string
	expected  string
}

func getWasherDrierTestCases(t *testing.T, washerDrier WasherDrierProduct) []washerDrierTestCase {
	t.Helper()

	return []washerDrierTestCase{
		{
			"Type",
			"TYPE",
			"relation",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				rel, ok := c.(*document.RelationClaim)
				require.True(t, ok, "type property is not a relation claim")
				return rel.To.ID.String()
			},
			document.GetCorePropertyID("WASHER_DRIER").String(),
		},
		{
			"Name",
			"NAME",
			"text",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				textClaim, ok := c.(*document.TextClaim)
				require.True(t, ok, "name property is not a text claim")
				return textClaim.HTML["en"]
			},
			html.EscapeString(fmt.Sprintf("%s %s",
				strings.TrimSpace(washerDrier.SupplierOrTrademark),
				strings.TrimSpace(washerDrier.ModelIdentifier))),
		},
		{
			"EPREL Registration Number",
			"EPREL_REGISTRATION_NUMBER",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				require.True(t, ok, "EPREL registration number is not an identifier claim")
				return identifierClaim.Value
			},
			washerDrier.EPRELRegistrationNumber,
		},
		{
			"Model Identifier",
			"MODEL_IDENTIFIER",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				require.True(t, ok, "model identifier is not an identifier claim")
				return identifierClaim.Value
			},
			washerDrier.ModelIdentifier,
		},
		{
			"Eprel Contact ID",
			"EPREL_CONTACT_ID",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				require.True(t, ok, "EPREL contact ID is not an identifier claim")
				return identifierClaim.Value
			},
			strconv.FormatInt(washerDrier.EPRELContactID, 10),
		},
		{
			"Energy Label ID",
			"ENERGY_LABEL_ID",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				require.True(t, ok, "energy label ID is not an identifier claim")
				return identifierClaim.Value
			},
			strconv.FormatInt(int64(washerDrier.EnergyLabelID), 10),
		},
		{
			"Ecolabel Registration Number",
			"ECOLABEL_REGISTRATION_NUMBER",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				require.True(t, ok, "Ecolabel registration number is not an identifier claim")
				return identifierClaim.Value
			},
			washerDrier.EcoLabelRegistrationNumber,
		},
		{
			"Energy Class",
			"ENERGY_CLASS",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				require.True(t, ok, "energy class is not a string claim")
				return stringClaim.String
			},
			string(washerDrier.EnergyClass),
		},
		{
			"Energy Class Image",
			"ENERGY_CLASS_IMAGE",
			"file",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				fileClaim, ok := c.(*document.FileClaim)
				require.True(t, ok, "energy class image is not a file claim")
				return strings.TrimPrefix(fileClaim.URL,
					"https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/")
			}, washerDrier.EnergyClassImage,
		},
		{
			"Energy Class Image With Scale",
			"ENERGY_CLASS_IMAGE_WITH_SCALE",
			"file",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				fileClaim, ok := c.(*document.FileClaim)
				require.True(t, ok, "energy class image with scale is not a file claim")
				return strings.TrimPrefix(fileClaim.URL,
					"https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/")
			}, washerDrier.EnergyClassImageWithScale,
		},
		{
			"Implementing Act",
			"IMPLEMENTING_ACT",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				require.True(t, ok, "implementing act is not a string claim")
				return stringClaim.String
			}, washerDrier.ImplementingAct,
		},
		{
			"Supplier Or Trademark",
			"SUPPLIER_OR_TRADEMARK",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				require.True(t, ok, "supplier or trademark is not a string claim")
				return stringClaim.String
			}, washerDrier.SupplierOrTrademark,
		},
		{
			"Noise Dry",
			"NOISE_DRY",
			"amount",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				amountClaim, ok := c.(*document.AmountClaim)
				require.True(t, ok, "noise dry is not an amount claim")
				return fmt.Sprintf("%.1f dB", amountClaim.Amount)
			},
			fmt.Sprintf("%.1f dB", washerDrier.NoiseDry),
		},
		{
			"Noise Spin",
			"NOISE_SPIN",
			"amount",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				amountClaim, ok := c.(*document.AmountClaim)
				require.True(t, ok, "noise spin is not an amount claim")
				return fmt.Sprintf("%.1f dB", amountClaim.Amount)
			},
			fmt.Sprintf("%.1f dB", washerDrier.NoiseSpin),
		},
		{
			"Noise Wash",
			"NOISE_WASH",
			"amount",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				amountClaim, ok := c.(*document.AmountClaim)
				require.True(t, ok, "noise wash is not an amount claim")
				return fmt.Sprintf("%.1f dB", amountClaim.Amount)
			},
			fmt.Sprintf("%.1f dB", washerDrier.NoiseWash),
		},
	}
}

func TestMakeWasherDrierDoc(t *testing.T) {
	t.Parallel()

	washerDrier := createTestWasherDrier(t)

	doc, errE := makeWasherDrierDoc(washerDrier)
	require.NoError(t, errE, "% -+#.1v", errE)

	tests := getWasherDrierTestCases(t, washerDrier)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			claims := doc.Get(document.GetCorePropertyID(tt.propName))
			require.NotEmpty(t, claims, "no claims found for property %s", tt.propName)

			found := false
			for _, claim := range claims {
				switch tt.claimType {
				case "identifier":
					assert.IsType(t, &document.IdentifierClaim{}, claim, "property %s should be an identifier claim", tt.propName) //nolint:exhaustruct
					continue
				case "file":
					assert.IsType(t, &document.FileClaim{}, claim, "property %s should be a file claim", tt.propName) //nolint:exhaustruct
					continue
				case "string":
					assert.IsType(t, &document.StringClaim{}, claim, "property %s should be a string claim", tt.propName) //nolint:exhaustruct
					continue
				case "relation":
					assert.IsType(t, &document.RelationClaim{}, claim, "property %s should be a relation claim", tt.propName) //nolint:exhaustruct
					continue
				}
				value := tt.getValue(t, claim)
				if value == tt.expected {
					found = true
					break
				}
			}
			assert.True(t, found, "expected value %s not found for property %s", tt.expected, tt.propName)
		})
	}
}

func TestNullUnmarshalingAndMarshaling(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field Null `json:"field"`
	}

	var s testStruct
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"field":null}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)

	b, errE := x.MarshalWithoutEscapeHTML(s)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `{"field":null}`, string(b))

	errE = x.UnmarshalWithoutUnknownFields([]byte(`{"field":123}`), &s)
	assert.Error(t, errE)
}

func TestBoolUnmarshalingAndMarshaling(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field Bool `json:"field"`
	}

	var s testStruct
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"field":true}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)

	b, errE := x.MarshalWithoutEscapeHTML(s)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `{"field":true}`, string(b))

	errE = x.UnmarshalWithoutUnknownFields([]byte(`{"field":false}`), &s)
	assert.Error(t, errE)
}

func TestEnergyClassUnmarshaling(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field EnergyClass `json:"field"`
	}

	var s testStruct
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"field":"H"}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, EnergyClass("H"), s.Field)

	errE = x.UnmarshalWithoutUnknownFields([]byte(`{"field":"APPP"}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, EnergyClass("A+++"), s.Field)
}

func TestStatusUnmarshaling(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field Status `json:"field"`
	}

	var s testStruct
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"field":"PUBLISHED"}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)

	errE = x.UnmarshalWithoutUnknownFields([]byte(`{"field":"something else"}`), &s)
	assert.Error(t, errE)
}

func TestTrademarkVerificationStatusUnmarshaling(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Field TrademarkVerificationStatus `json:"field"`
	}

	var s testStruct
	errE := x.UnmarshalWithoutUnknownFields([]byte(`{"field":"VERIFIED"}`), &s)
	assert.NoError(t, errE, "% -+#.1v", errE)

	errE = x.UnmarshalWithoutUnknownFields([]byte(`{"field":"something else"}`), &s)
	assert.Error(t, errE)
}

func TestEpochTimeUnmarshalingAndMarshaling(t *testing.T) {
	t.Parallel()

	jsonData := []byte(`{"timestamp":1540512000}`)
	var result struct {
		Timestamp EpochTime `json:"timestamp"`
	}

	errE := x.UnmarshalWithoutUnknownFields(jsonData, &result)
	require.NoError(t, errE, "% -+#.1v", errE)

	expectedTime := time.Date(2018, 10, 26, 0, 0, 0, 0, time.UTC)
	actualTime := time.Time(result.Timestamp)

	assert.Equal(t, expectedTime.Year(), actualTime.Year())
	assert.Equal(t, expectedTime.Month(), actualTime.Month())
	assert.Equal(t, expectedTime.Day(), actualTime.Day())

	marshalledData, errE := x.MarshalWithoutEscapeHTML(result)
	require.NoError(t, errE, "% -+#.1v", errE)

	assert.Equal(t, `{"timestamp":1540512000}`, string(marshalledData))
}

func TestAddPlacementCountries(t *testing.T) {
	t.Parallel()
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123"),
			Score: document.HighConfidence,
		},
		Claims: &document.ClaimTypes{},
	}

	placementCountries := []PlacementCountry{
		{Country: "DE", OrderNumber: 1},
		{Country: "FR", OrderNumber: 2},
		{Country: "IT", OrderNumber: 3},
		{Country: "", OrderNumber: 4}, // Test empty country code.
	}

	for i, placementCountry := range placementCountries {
		country := strings.TrimSpace(placementCountry.Country)
		if country != "" {
			errE := doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "PLACEMENT_COUNTRY", i),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("MARKET_COUNTRY"),
				String: country,
			})
			require.NoError(t, errE, "% -+#.1v", errE)
		}
	}

	marketCountryClaims := doc.Get(document.GetCorePropertyID("MARKET_COUNTRY"))
	assert.Len(t, marketCountryClaims, len(placementCountries)-1, "should have added 3 valid placement countries")

	var foundCountries []string
	for _, claim := range marketCountryClaims {
		if stringClaim, ok := claim.(*document.StringClaim); ok {
			foundCountries = append(foundCountries, stringClaim.String)
		}
	}
	assert.ElementsMatch(t, []string{"DE", "FR", "IT"}, foundCountries, "should contain DE, FR, IT placement countries")
}

func TestAddUploadedLabels(t *testing.T) {
	t.Parallel()
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123"),
			Score: document.HighConfidence,
		},
		Claims: &document.ClaimTypes{},
	}

	uploadedLabels := []string{
		"label1.pdf",
		"label2.jpg",
		"label3.png",
		"label4.svg",
		"invalid.jfif",
		".50583513_17mb211s_invalid",
		"label5.jpeg",
		"",
		" ",
		".pd",
	}

	for i, uploadedLabel := range uploadedLabels {
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
				// Skip invalid extensions.
				continue
			}
			errE := doc.Add(&document.FileClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "UPLOADED_LABEL", i),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("UPLOADED_LABEL"),
				MediaType: mediaType,
				URL:       "https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel,
				Preview:   []string{"https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel},
			})
			require.NoError(t, errE, "% -+#.1v", errE)
		}
	}

	// Check the number of valid labels added.
	uploadedLabelClaims := doc.Get(document.GetCorePropertyID("UPLOADED_LABEL"))
	assert.Len(t, uploadedLabelClaims, 5, "should have added 5 valid uploaded labels")

	// Verify the media types are correct.
	var foundMediaTypes []string
	var foundURLs []string
	for _, claim := range uploadedLabelClaims {
		if fileClaim, ok := claim.(*document.FileClaim); ok {
			foundMediaTypes = append(foundMediaTypes, fileClaim.MediaType)
			foundURLs = append(foundURLs, fileClaim.URL)
		}
	}

	// Check media types.
	expectedMediaTypes := []string{
		"application/pdf",
		"image/jpeg",
		"image/png",
		"image/svg+xml",
		"image/jpeg",
	}
	assert.ElementsMatch(t, expectedMediaTypes, foundMediaTypes, "should have correct media types")

	// Check URLs.
	expectedURLs := []string{
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label1.pdf",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label2.jpg",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label3.png",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label4.svg",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label5.jpeg",
	}
	assert.ElementsMatch(t, expectedURLs, foundURLs, "should have correct URLs")
}

func TestAddOtherIdentifiers(t *testing.T) {
	t.Parallel()
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123"),
			Score: document.HighConfidence,
		},
		Claims: &document.ClaimTypes{},
	}

	otherIdentifiers := []OtherIdentifiers{
		{OrderNumber: 1, ModelIdentifier: "7381032426154", Type: "EAN_13"},
		{OrderNumber: 2, ModelIdentifier: "SAND_IS40E", Type: "OTHER"},
		{OrderNumber: 3, ModelIdentifier: "", Type: "EAN_13"},
		{OrderNumber: 4, ModelIdentifier: "  ", Type: "OTHER"},
		{OrderNumber: 5, ModelIdentifier: "5901234123457", Type: "EAN_14"},
	}

	for i, otherIdentifier := range otherIdentifiers {
		if strings.TrimSpace(otherIdentifier.ModelIdentifier) != "" {
			errE := doc.Add(&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "EPREL_OTHER_IDENTIFIER", i),
					Confidence: document.HighConfidence,
					Meta: &document.ClaimTypes{
						String: document.StringClaims{
							{
								CoreClaim: document.CoreClaim{
									ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "EPREL_OTHER_IDENTIFIER", i, "TYPE", 0),
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
			require.NoError(t, errE, "% -+#.1v", errE)
		}
	}

	otherIdentifierClaims := doc.Get(document.GetCorePropertyID("EPREL_OTHER_IDENTIFIER"))
	assert.Len(t, otherIdentifierClaims, 3, "should have added 3 valid identifiers")

	foundIdentifiers := make([]string, 0, len(otherIdentifierClaims))
	identifierToType := map[string]string{}

	for _, claim := range otherIdentifierClaims {
		identifierClaim, ok := claim.(*document.IdentifierClaim)
		require.True(t, ok, "claim should be an IdentifierClaim")

		foundIdentifiers = append(foundIdentifiers, identifierClaim.Value)
		typeClaims := identifierClaim.Get(document.GetCorePropertyID("EPREL_OTHER_IDENTIFIER_TYPE"))

		require.NotEmpty(t, typeClaims, "missing type metadata for identifier %s", identifierClaim.Value)

		stringClaim, ok := typeClaims[0].(*document.StringClaim)
		require.True(t, ok, "type claim should be a StringClaim")
		identifierToType[identifierClaim.Value] = stringClaim.String
	}

	expectedIdentifiers := []string{
		"7381032426154",
		"SAND_IS40E",
		"5901234123457",
	}
	assert.ElementsMatch(t, expectedIdentifiers, foundIdentifiers, "should have correct identifiers")

	expectedIdentifierToType := map[string]string{
		"7381032426154": "EAN_13",
		"SAND_IS40E":    "OTHER",
		"5901234123457": "EAN_14",
	}

	for identifier, expectedType := range expectedIdentifierToType {
		actualType, exists := identifierToType[identifier]
		assert.True(t, exists, "identifier %s should have a type", identifier)
		assert.Equal(t, expectedType, actualType, "identifier %s should have type %s, got %s",
			identifier, expectedType, actualType)
	}
}
