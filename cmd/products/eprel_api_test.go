package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
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

	// Assert that washerdriers are present (add more later when we process other product groups).
	expectedGroups := []string{
		"washerdriers",
	}

	for _, expected := range expectedGroups {
		assert.Contains(t, urlCodes, expected, "product groups should contain %s", expected)
	}
}

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("EPREL_API_KEY") == "" {
		t.Skip("EPREL_API_KEY is not available")
	}
}

func getAPIKey(t *testing.T) string {
	t.Helper()
	skipIfNoAPIKey(t)
	key, err := os.ReadFile("../../.eprel.secret")
	require.NoError(t, err)

	return strings.TrimSpace(string(key))
}

func TestGetWasherDriers(t *testing.T) {
	t.Parallel()

	skipIfNoAPIKey(t)
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil // suppress unnecessary debug logs unless something fails.
	apiKey := getAPIKey(t)

	washerDriers, errE := getWasherDriers(ctx, httpClient, apiKey)
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Logf("Total washer driers retrieved: %d", len(washerDriers))

	// Test first item has expected fields.
	if len(washerDriers) > 0 {
		first := washerDriers[0]
		t.Log("First washer drier:")
		t.Logf("Model: %s", first.ModelIdentifier)
		t.Logf("Energy class: %s", first.EnergyClass)
		t.Logf("Number of cycles: %d", len(first.Cycles))
	}
}

func TestInspectSingleWasherDrier(t *testing.T) {
	t.Parallel()

	skipIfNoAPIKey(t)
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil
	apiKey := getAPIKey(t)

	// Get just one washer-drier.
	url := "https://eprel.ec.europa.eu/api/products/washerdriers?_limit=1&_page=1"
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	hits, ok := result["hits"].([]interface{})
	require.True(t, ok, "result['hits'] is not []interface{}")
	require.NotEmpty(t, hits, "no washer-driers found")

	// Pretty print the first washer-drier.
	washerDrier := hits[0]
	prettyJSON, err := json.MarshalIndent(washerDrier, "", "    ")
	require.NoError(t, err)

	t.Logf("Single washer-drier example:\n%s", string(prettyJSON))
}

func createTestWasherDrier() WasherDrierProduct {
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

func getWasherDrierTestCases(washerDrier WasherDrierProduct) []washerDrierTestCase {
	return []washerDrierTestCase{
		{
			"Type",
			"TYPE",
			"relation",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				rel, ok := c.(*document.RelationClaim)
				require.True(t, ok, "Type property is not a relation claim")
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
				require.True(t, ok, "Name property is not a Text claim")
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
				require.True(t, ok, "EPREL Registration Number is not an identifier claim")
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
				require.True(t, ok, "Model Identifier is not an identifier claim")
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
				require.True(t, ok, "EPREL Contact ID is not an identifier claim")
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
				require.True(t, ok, "Energy Label ID is not an identifier claim")
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
				require.True(t, ok, "Ecolabel Registration Number is not an identifier claim")
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
				require.True(t, ok, "Energy Class is not a string claim")
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
				require.True(t, ok, "Energy Class Image is not a file claim")
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
				require.True(t, ok, "Energy Class Image With Scale is not a file claim")
				return strings.TrimPrefix(fileClaim.URL,
					"https://ec.europa.eu/assets/move-ener/eprel/EPREL%20Public/Nested-labels%20thumbnails/")
			}, washerDrier.EnergyClassImageWithScale,
		},
		{
			"Energy Class Range",
			"ENERGY_CLASS_RANGE",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				require.True(t, ok, "Energy Class Range is not a string claim")
				return stringClaim.String
			}, washerDrier.EnergyClassRange,
		},
		{
			"Implementing Act",
			"IMPLEMENTING_ACT",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				require.True(t, ok, "Implementing Act is not a string claim")
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
				require.True(t, ok, "Supplier Or Trademark is not a string claim")
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
				require.True(t, ok, "Noise Dry is not an amount claim")
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
				require.True(t, ok, "Noise Spin is not an amount claim")
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
				require.True(t, ok, "Noise Wash is not an amount claim")
				return fmt.Sprintf("%.1f dB", amountClaim.Amount)
			},
			fmt.Sprintf("%.1f dB", washerDrier.NoiseWash),
		},
	}
}

func TestMakeWasherDrierDoc(t *testing.T) {
	t.Parallel()

	skipIfNoAPIKey(t)

	washerDrier := createTestWasherDrier()

	doc, errE := makeWasherDrierDoc(washerDrier)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Print document to inspect in console.
	prettyDoc, errE := x.MarshalWithoutEscapeHTML(doc)
	require.NoError(t, errE, "% -+#.1v", errE)

	t.Logf("Document structure:\n%s", string(prettyDoc))

	tests := getWasherDrierTestCases(washerDrier)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claims := doc.Get(document.GetCorePropertyID(tt.propName))
			require.NotEmpty(t, claims, "no claims found for property %s", tt.propName)

			found := false
			for _, claim := range claims {
				// First verify the claim type.
				switch tt.claimType {
				case "identifier":
					if _, ok := claim.(*document.IdentifierClaim); !ok {
						assert.IsType(t, &document.IdentifierClaim{
							CoreClaim: document.CoreClaim{
								ID:         identifier.Identifier{},
								Confidence: 0,
							},
							Prop: document.Reference{
								ID: nil,
							},
							Value: "",
						}, claim, "property %s should be an identifier claim", tt.propName)
						continue
					}
				case "file":
					if _, ok := claim.(*document.FileClaim); !ok {
						assert.IsType(t, &document.FileClaim{
							CoreClaim: document.CoreClaim{
								ID:         identifier.Identifier{},
								Confidence: 0,
							},
							Prop: document.Reference{
								ID: nil,
							},
							MediaType: "",
							Preview:   []string{},
							URL:       "",
						}, claim, "property %s should be a file claim", tt.propName)
						continue
					}
				case "string":
					if _, ok := claim.(*document.StringClaim); !ok {
						assert.IsType(t, &document.StringClaim{
							CoreClaim: document.CoreClaim{
								ID:         identifier.Identifier{},
								Confidence: 0,
							},
							Prop: document.Reference{
								ID: nil,
							},
							String: "",
						}, claim, "property %s should be a string claim", tt.propName)
						continue
					}
				case "relation":
					if _, ok := claim.(*document.RelationClaim); !ok {
						assert.IsType(t, &document.RelationClaim{
							CoreClaim: document.CoreClaim{
								ID:         identifier.Identifier{},
								Confidence: 0,
							},
							Prop: document.Reference{
								ID: nil,
							},
							To: document.Reference{
								ID: nil,
							},
						}, claim, "property %s should be a relation claim", tt.propName)
						continue
					}
				}
				value := tt.getValue(t, claim)
				if value == tt.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected value %s not found for property %s", tt.expected, tt.propName)
			}
		})
	}
}

func TestInvalidNullUnmarshalling(t *testing.T) {
	t.Parallel()

	validWasherDrier := createTestWasherDrier()

	validJSON, err := json.Marshal(validWasherDrier)
	require.NoError(t, err, "Failed to marshal valid washer drier")

	invalidJSON := strings.Replace(string(validJSON), `"generatedLabels":null`,
		`"generatedLabels":"test non null value"`, 1)

	var invalidWasherDrier WasherDrierProduct
	err = json.Unmarshal([]byte(invalidJSON), &invalidWasherDrier)

	assert.Error(t, err, "Unmarshaling should fail when a Null field contains a non-null value")
	assert.Contains(t, err.Error(), "only null value is excepted",
		"Error should indicate that only null values are accepted")
}

func TestEnergyClassUnmarshalling(t *testing.T) {
	t.Parallel()

	washerDrier := createTestWasherDrier()
	assert.Equal(t, "APPP", string(washerDrier.EnergyClass), "Initial energy class should be URL-safe format")

	jsonData, err := json.Marshal(washerDrier)
	require.NoError(t, err, "Failed to marshal valid washer drier")

	assert.Contains(t, string(jsonData), `"energyClass":"APPP"`, "JSON should contain URL-safe format")

	var unmarshaled WasherDrierProduct
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err, "Failed to unmarshal washer drier")

	assert.Equal(t, "A+++", string(unmarshaled.EnergyClass),
		"Unmarshaled energy class should be converted to display format with + characters")

	doc, errE := makeWasherDrierDoc(unmarshaled)
	require.NoError(t, errE, "Failed to create document from washer drier")

	claims := doc.Get(document.GetCorePropertyID("ENERGY_CLASS"))
	require.NotEmpty(t, claims, "No energy class claims found")

	stringClaim, ok := claims[0].(*document.StringClaim)
	require.True(t, ok, "Energy class claim is not a string claim")
	assert.Equal(t, "A+++", stringClaim.String,
		"Energy class in document should use display format with + characters")
}

func TestEpochTimeUnmarshallingAndMarshalling(t *testing.T) {
	t.Parallel()

	jsonData := []byte(`{"timestamp": 1540512000}`)
	var result struct {
		Timestamp EpochTime `json:"timestamp"`
	}

	err := json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON data")

	expectedTime := time.Date(2018, 10, 26, 0, 0, 0, 0, time.UTC)
	actualTime := time.Time(result.Timestamp)

	assert.Equal(t, expectedTime.Year(), actualTime.Year())
	assert.Equal(t, expectedTime.Month(), actualTime.Month())
	assert.Equal(t, expectedTime.Day(), actualTime.Day())

	marshalledData, err := json.Marshal(result)
	require.NoError(t, err, "Failed to marshal result")

	assert.Contains(t, string(marshalledData), `"timestamp":1540512000`)
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
			err := doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "PLACEMENT_COUNTRY", i),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("MARKET_COUNTRY"),
				String: country,
			})
			if err != nil {
				t.Fatalf("Error adding claim: %v", err)
			}
		}
	}

	marketCountryClaims := doc.Get(document.GetCorePropertyID("MARKET_COUNTRY"))
	assert.Len(t, marketCountryClaims, len(placementCountries)-1, "Should have added 3 valid placement countries")

	var foundCountries []string
	for _, claim := range marketCountryClaims {
		if stringClaim, ok := claim.(*document.StringClaim); ok {
			foundCountries = append(foundCountries, stringClaim.String)
		}
	}
	assert.ElementsMatch(t, []string{"DE", "FR", "IT"}, foundCountries, "Should contain DE, FR, IT placement countries")
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
			err := doc.Add(&document.FileClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceProducts, "WASHER_DRIER", "TEST123", "UPLOADED_LABELS", i),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("UPLOADED_LABELS"),
				MediaType: mediaType,
				URL:       "https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel,
				Preview:   []string{"https://eprel.ec.europa.eu/supplier-labels/washerdriers/" + uploadedLabel},
			})
			if err != nil {
				t.Fatalf("Error adding claim: %v", err)
			}
		}
	}

	// Check the number of valid labels added.
	uploadedLabelClaims := doc.Get(document.GetCorePropertyID("UPLOADED_LABELS"))
	assert.Len(t, uploadedLabelClaims, 5, "Should have added 5 valid uploaded labels")

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
	assert.ElementsMatch(t, expectedMediaTypes, foundMediaTypes, "Should have correct media types")

	// Check URLs.
	expectedURLs := []string{
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label1.pdf",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label2.jpg",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label3.png",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label4.svg",
		"https://eprel.ec.europa.eu/supplier-labels/washerdriers/label5.jpeg",
	}
	assert.ElementsMatch(t, expectedURLs, foundURLs, "Should have correct URLs")
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
			err := doc.Add(&document.IdentifierClaim{
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
			if err != nil {
				t.Fatalf("Error adding claim: %v", err)
			}
		}
	}

	otherIdentifierClaims := doc.Get(document.GetCorePropertyID("EPREL_OTHER_IDENTIFIER"))
	assert.Len(t, otherIdentifierClaims, 3, "Should have added 3 valid identifiers")

	foundIdentifiers := make([]string, 0, len(otherIdentifierClaims))
	identifierToType := map[string]string{}

	for _, claim := range otherIdentifierClaims {
		identifierClaim, ok := claim.(*document.IdentifierClaim)
		if !ok {
			t.Error("Claim should be an IdentifierClaim")
			continue
		}

		foundIdentifiers = append(foundIdentifiers, identifierClaim.Value)
		typeClaims := identifierClaim.Get(document.GetCorePropertyID("EPREL_OTHER_IDENTIFIER_TYPE"))

		if len(typeClaims) == 0 {
			t.Error("Missing type metadata for identifier", identifierClaim.Value)
			continue
		}

		stringClaim, ok := typeClaims[0].(*document.StringClaim)
		if !ok {
			t.Error("Type claim should be a StringClaim")
			continue
		}
		identifierToType[identifierClaim.Value] = stringClaim.String
	}

	expectedIdentifiers := []string{
		"7381032426154",
		"SAND_IS40E",
		"5901234123457",
	}
	assert.ElementsMatch(t, expectedIdentifiers, foundIdentifiers, "Should have correct identifiers")

	expectedIdentifierToType := map[string]string{
		"7381032426154": "EAN_13",
		"SAND_IS40E":    "OTHER",
		"5901234123457": "EAN_14",
	}

	for identifier, expectedType := range expectedIdentifierToType {
		actualType, exists := identifierToType[identifier]
		assert.True(t, exists, "Identifier %s should have a type", identifier)
		assert.Equal(t, expectedType, actualType, "Identifier %s should have type %s, got %s",
			identifier, expectedType, actualType)
	}
}
