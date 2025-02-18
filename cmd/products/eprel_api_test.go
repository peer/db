// eprel_api_test.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

func TestGetProductGroups(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()

	urlCodes, err := getProductGroups(ctx, httpClient)
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range urlCodes {
		t.Logf("url_code: %s", code)
	}
}

func getAPIKey(t *testing.T) string {
	t.Helper()
	key, err := os.ReadFile("../../.eprel.secret")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(key))
}

func TestGetAPIKey(t *testing.T) {
	t.Parallel()
	key := getAPIKey(t)
	t.Logf("API key: %s", key)
}

func TestGetWasherDriers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	washerDriers, err := getWasherDriers(ctx, httpClient, apiKey)
	if err != nil {
		t.Fatal(err)
	}

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

func TestMapAllWasherDrierFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	seenFields := make(map[string][]interface{})

	page := 1
	for {
		url := fmt.Sprintf("https://eprel.ec.europa.eu/api/products/washerdriers?_limit=100&_page=%d", page)
		req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Api-Key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()

		hits, ok := result["hits"].([]interface{})
		if !ok {
			t.Fatal("result['hits'] is not []interface{}")
		}
		if len(hits) == 0 {
			break
		}

		for _, hit := range hits {
			product, ok := hit.(map[string]interface{})
			if !ok {
				t.Fatal("hit is not map[string]interface{}")
			}

			for field, value := range product {
				if _, exists := seenFields[field]; !exists {
					seenFields[field] = []interface{}{value}
				}
			}
		}

		page++
	}

	// Print fields and sample values.
	fields := make([]string, 0, len(seenFields))

	for field := range seenFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	t.Log("Fields and their structure")
	for _, field := range fields {
		value := seenFields[field][0]
		t.Logf("- %s: %T", field, value)

		// Print structure of nested objects.
		if m, ok := value.(map[string]interface{}); ok {
			for k := range m {
				t.Logf("  - %s", k)
			}
		} else if a, ok := value.([]interface{}); ok && len(a) > 0 {
			if m, ok := a[0].(map[string]interface{}); ok {
				for k := range m {
					t.Logf("  - %s", k)
				}
			}
		}
	}
}

func TestInspectSingleWasherDrier(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	// Get just one washer-drier.
	url := "https://eprel.ec.europa.eu/api/products/washerdriers?_limit=1&_page=1"
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Api-Key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	hits, ok := result["hits"].([]interface{})
	if !ok {
		t.Fatal("result['hits'] is not []interface{}")
	}
	if len(hits) == 0 {
		t.Fatal("no washer-driers found")
	}

	// Pretty print the first washer-drier.
	washerDrier := hits[0]
	prettyJSON, err := json.MarshalIndent(washerDrier, "", "    ")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Single washer-drier example:\n%s", string(prettyJSON))
}

func createTestWasherDrier() WasherDrierProduct {
	return WasherDrierProduct{
		EprelRegistrationNumber:    "132300",
		ModelIdentifier:            "F94J8VH2WD",
		ContactID:                  1234,
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
			AddressBloc:          nil,
			City:                 "",
			ContactByReferenceID: nil,
			ContactReference:     "",
			Country:              "",
			DefaultContact:       false,
			Email:                "",
			ID:                   0,
			Municipality:         nil,
			OrderNumber:          nil,
			Phone:                "",
			PostalCode:           "",
			Province:             nil,
			ServiceName:          "",
			Status:               "",
			Street:               "",
			StreetNumber:         "",
			WebSiteURL:           nil,
		},
		Cycles:                   []Cycle{},
		EcoLabel:                 false,
		EnergyAnnualWash:         0,
		EnergyAnnualWashAndDry:   0,
		ExportDateTS:             0,
		FirstPublicationDate:     []int{},
		FirstPublicationDateTS:   0,
		FormType:                 "",
		GeneratedLabels:          nil,
		ImportedOn:               0,
		LastVersion:              false,
		NoiseDry:                 0,
		NoiseSpin:                0,
		NoiseWash:                0,
		OnMarketEndDate:          []int{},
		OnMarketEndDateTS:        0,
		OnMarketFirstStartDate:   []int{},
		OnMarketFirstStartDateTS: 0,
		OnMarketStartDate:        []int{},
		OnMarketStartDateTS:      0,
		OrgVerificationStatus:    "",
		Organisation: Organisation{
			CloseDate:         nil,
			CloseStatus:       nil,
			FirstName:         nil,
			IsClosed:          false,
			LastName:          nil,
			OrganisationName:  "",
			OrganisationTitle: "",
			Website:           nil,
		},
		OtherIdentifiers:            []interface{}{},
		PlacementCountries:          []interface{}{},
		ProductGroup:                "",
		ProductModelCoreID:          0,
		PublishedOnDate:             []int{},
		PublishedOnDateTS:           0,
		RegistrantNature:            "",
		Status:                      "",
		TrademarkID:                 0,
		TrademarkOwner:              nil,
		TrademarkVerificationStatus: "",
		UploadedLabels:              []string{},
		VersionID:                   0,
		VersionNumber:               0,
		VisibleToUkMsa:              false,
		WaterAnnualWash:             0,
		WaterAnnualWashAndDry:       0,
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
				if !ok {
					t.Fatal("Type property is not a relation claim")
				}
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
				if !ok {
					t.Fatal("Name property is not a Text claim")
				}
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
				if !ok {
					t.Fatal("EPREL Registration Number is not an identifier claim")
				}
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
				if !ok {
					t.Fatal("Model Identifier is not an identifier claim")
				}
				return identifierClaim.Value
			},
			washerDrier.ModelIdentifier,
		},
		{
			"Contact ID",
			"CONTACT_ID",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				if !ok {
					t.Fatal("Contact ID is not an identifier claim")
				}
				return identifierClaim.Value
			},
			strconv.FormatFloat(washerDrier.ContactID, 'f', 0, 64),
		},
		{
			"Energy Label ID",
			"ENERGY_LABEL_ID",
			"identifier",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				identifierClaim, ok := c.(*document.IdentifierClaim)
				if !ok {
					t.Fatal("Energy Label ID is not an identifier claim")
				}
				return identifierClaim.Value
			},
			strconv.FormatFloat(washerDrier.EnergyLabelID, 'f', 0, 64),
		},
		{
			"Ecolabel Registration Number",
			"ECOLABEL_REGISTRATION_NUMBER",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				if !ok {
					t.Fatal("Ecolabel Registration Number is not an string claim")
				}
				return stringClaim.String
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
				if !ok {
					t.Fatal("Energy Class is not a string claim")
				}
				return stringClaim.String
			},
			washerDrier.EnergyClass,
		},
		{
			"Energy Class Image",
			"ENERGY_CLASS_IMAGE",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				if !ok {
					t.Fatal("Energy Class Image is not a string claim")
				}
				return stringClaim.String
			}, washerDrier.EnergyClassImage,
		},
		{
			"Energy Class Image With Scale",
			"ENERGY_CLASS_IMAGE_WITH_SCALE",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				if !ok {
					t.Fatal("Energy Class Image With Scale is not a string claim")
				}
				return stringClaim.String
			}, washerDrier.EnergyClassImageWithScale,
		},
		{
			"Energy Class Range",
			"ENERGY_CLASS_RANGE",
			"string",
			func(t *testing.T, c document.Claim) string {
				t.Helper()
				stringClaim, ok := c.(*document.StringClaim)
				if !ok {
					t.Fatal("Energy Class Range is not a string claim")
				}
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
				if !ok {
					t.Fatal("Implementing Act is not a string claim")
				}
				return stringClaim.String
			}, washerDrier.ImplementingAct,
		},
	}
}

func TestMakeWasherDrierDoc(t *testing.T) {
	t.Parallel()

	washerDrier := createTestWasherDrier()

	doc := makeWasherDrierDoc(washerDrier)

	// Print document to inspect in console.
	prettyDoc, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		t.Fatal(errE)
	}
	t.Logf("Document structure:\n%s", string(prettyDoc))

	tests := getWasherDrierTestCases(washerDrier)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claims := doc.Get(document.GetCorePropertyID(tt.propName))
			if len(claims) == 0 {
				t.Errorf("no claims found for property %s", tt.propName)
				return
			}

			found := false
			for _, claim := range claims {
				// First verify the claim type.
				switch tt.claimType {
				case "identifier":
					if _, ok := claim.(*document.IdentifierClaim); !ok {
						t.Errorf("expected identifier claim for property %s, got %T", tt.propName, claim)
						continue
					}
				case "string":
					if _, ok := claim.(*document.StringClaim); !ok {
						t.Errorf("expected string claim for property %s, got %T", tt.propName, claim)
						continue
					}
				case "relation":
					if _, ok := claim.(*document.RelationClaim); !ok {
						t.Errorf("expected relation claim for property %s, got %T", tt.propName, claim)
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
