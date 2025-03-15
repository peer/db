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
		EprelRegistrationNumber:    "132300",
		ModelIdentifier:            "F94J8VH2WD",
		EprelContactID:             1234,
		EnergyLabelID:              998462,
		EcoLabelRegistrationNumber: "1234",
		EnergyClass:                "A",
		EnergyClassImage:           "A-Left-DarkGreen.png",
		EnergyClassImageWithScale:  "A-Left-DarkGreen-WithAGScale.svg",
		EnergyClassRange:           "A_G",
		ImplementingAct:            "EC_96_60",
		SupplierOrTrademark:        "LG Electronics Inc.",
		AllowEprelLabelGeneration:  false,
		Blocked:                    false,
		ContactDetails: ContactDetails{
			Address:              "",
			City:                 "",
			ContactByReferenceID: "",
			ContactReference:     "",
			Country:              "",
			DefaultContact:       false,
			Email:                "",
			ID:                   0,
			Municipality:         "",
			OrderNumber:          "",
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
		EnergyAnnualWash:                0,
		EnergyAnnualWashAndDry:          0,
		ExportDateTimestamp:             0,
		FirstPublicationDate:            []int{},
		FirstPublicationDateTimestamp:   0,
		FormType:                        "",
		GeneratedLabels:                 nil,
		ImportedOn:                      0,
		LastVersion:                     false,
		NoiseDry:                        0,
		NoiseSpin:                       0,
		NoiseWash:                       0,
		OnMarketEndDate:                 []int{},
		OnMarketEndDateTimestamp:        0,
		OnMarketFirstStartDate:          []int{},
		OnMarketFirstStartDateTimestamp: 0,
		OnMarketStartDate:               []int{},
		OnMarketStartDateTimestamp:      0,
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
		OtherIdentifiers:            []string{},
		PlacementCountries:          []PlacementCountry{},
		ProductGroup:                "",
		ProductModelCoreID:          0,
		PublishedOnDate:             []int{},
		PublishedOnDateTimestamp:    0,
		RegistrantNature:            "",
		Status:                      "",
		TrademarkID:                 0,
		TrademarkOwner:              nil,
		TrademarkVerificationStatus: "",
		UploadedLabels:              []string{},
		VersionID:                   0,
		VersionNumber:               0,
		VisibleToUnitedKingdomMarketSurveillanceAuthority: false,
		WaterAnnualWash:       0,
		WaterAnnualWashAndDry: 0,
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
			washerDrier.EprelRegistrationNumber,
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
				require.True(t, ok, "Eprel Contact ID is not an identifier claim")
				return identifierClaim.Value
			},
			strconv.FormatInt(int64(washerDrier.EprelContactID), 10),
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
			washerDrier.EnergyClass,
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
