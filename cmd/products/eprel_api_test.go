// eprel_api_test.go
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
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/tozd/go/x"
)

func TestGetProductGroups(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()

	urlCodes, err := getProductGroups(ctx, httpClient)
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range urlCodes {
		fmt.Printf("url_code: %s\n", code)
	}
}

func getAPIKey(t *testing.T) string {
	key, err := os.ReadFile("../../.eprel.secret")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(key))
}

func TestGetAPIKey(t *testing.T) {
	key := getAPIKey(t)
	fmt.Printf("API key: %s\n", key)
}

func TestGetWasherDriers(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	washerDriers, err := getWasherDriers(ctx, httpClient, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Total washer driers retrieved: %d\n", len(washerDriers))

	// Test first item has expected fields
	if len(washerDriers) > 0 {
		first := washerDriers[0]
		fmt.Printf("First washer drier:\n")
		fmt.Printf("Model: %s\n", first.ModelIdentifier)
		fmt.Printf("Energy class: %s\n", first.EnergyClass)
		fmt.Printf("Number of cycles: %d\n", len(first.Cycles))
	}
}

func TestMapAllWasherDrierFields(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	seenFields := make(map[string][]interface{})

	var page int = 1
	for {
		url := fmt.Sprintf("https://eprel.ec.europa.eu/api/products/washerdriers?_limit=100&_page=%d", page)
		req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("x-api-key", apiKey)

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

		hits := result["hits"].([]interface{})
		if len(hits) == 0 {
			break
		}

		for _, hit := range hits {
			product := hit.(map[string]interface{})
			for field, value := range product {
				if _, exists := seenFields[field]; !exists {
					seenFields[field] = []interface{}{value}
				}
			}
		}

		page++
	}

	// Print fields and sample values
	var fields []string
	for field := range seenFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	fmt.Println("Fields and their structure:")
	for _, field := range fields {
		value := seenFields[field][0]
		fmt.Printf("- %s: %T\n", field, value)

		// Print structure of nested objects
		if m, ok := value.(map[string]interface{}); ok {
			for k := range m {
				fmt.Printf("  - %s\n", k)
			}
		} else if a, ok := value.([]interface{}); ok && len(a) > 0 {
			if m, ok := a[0].(map[string]interface{}); ok {
				for k := range m {
					fmt.Printf("  - %s\n", k)
				}
			}
		}
	}
}

func TestInspectSingleWasherDrier(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()
	apiKey := getAPIKey(t)

	// Get just one washer-drier
	url := "https://eprel.ec.europa.eu/api/products/washerdriers?_limit=1&_page=1"
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	hits := result["hits"].([]interface{})
	if len(hits) == 0 {
		t.Fatal("no washer-driers found")
	}

	// Pretty print the first washer-drier
	washerDrier := hits[0]
	prettyJSON, err := json.MarshalIndent(washerDrier, "", "    ")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Single washer-drier example:\n%s\n", string(prettyJSON))
}

func TestMakeWasherDrierDoc(t *testing.T) {
	t.Parallel()

	washerDrier := WasherDrierProduct{
		EprelRegistrationNumber:    "132300",
		ModelIdentifier:            "F94J8VH2WD",
		ContactId:                  1234,
		EnergyLabelId:              998462,
		EcoLabelRegistrationNumber: "1234",
		EnergyClass:                "A",
		EnergyClassImage:           "A-Left-DarkGreen.png",
		EnergyClassImageWithScale:  "A-Left-DarkGreen-WithAGScale.svg",
		EnergyClassRange:           "A_G",
		ImplementingAct:            "EC_96_60",
		SupplierOrTrademark:        "LG Electronics Inc.",
	}

	doc, err := makeWasherDrierDoc(washerDrier)
	if err != nil {
		t.Fatal(err)
	}

	// Print document to inspect in console
	prettyDoc, errE := x.MarshalWithoutEscapeHTML(doc)
	if errE != nil {
		t.Fatal(errE)
	}
	fmt.Printf("\nDocument structure:\n%s\n\n", string(prettyDoc))

	tests := []struct {
		name      string
		propName  string
		claimType string
		getValue  func(claim document.Claim) string
		expected  string
	}{
		{
			"Type",
			"TYPE",
			"relation",
			func(c document.Claim) string {
				rel := c.(*document.RelationClaim)
				// We need to get the ID that this type points to
				return rel.To.ID.String()
			},
			// This should match the ID generated for WASHER_DRIER property
			document.GetCorePropertyID("WASHER_DRIER").String(),
		},
		{
			"Name",
			"NAME",
			"text",
			func(c document.Claim) string {
				textClaim := c.(*document.TextClaim)
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
			func(c document.Claim) string { return c.(*document.IdentifierClaim).Value },
			washerDrier.EprelRegistrationNumber,
		},
		{
			"Model Identifier",
			"MODEL_IDENTIFIER",
			"identifier",
			func(c document.Claim) string { return c.(*document.IdentifierClaim).Value },
			washerDrier.ModelIdentifier,
		},
		{
			"Contact Id",
			"CONTACT_ID",
			"identifier",
			func(c document.Claim) string { return c.(*document.IdentifierClaim).Value },
			strconv.FormatFloat(washerDrier.ContactId, 'f', 0, 64),
		},
		{
			"Energy Label Id",
			"ENERGY_LABEL_ID",
			"identifier",
			func(c document.Claim) string { return c.(*document.IdentifierClaim).Value },
			strconv.FormatFloat(washerDrier.EnergyLabelId, 'f', 0, 64),
		},
		{
			"Ecolabel Registration Number",
			"ECOLABEL_REGISTRATION_NUMBER",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.EcoLabelRegistrationNumber,
		},
		{
			"Energy Class",
			"ENERGY_CLASS",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.EnergyClass,
		},
		{
			"Energy Class Image",
			"ENERGY_CLASS_IMAGE",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.EnergyClassImage,
		},
		{
			"Energy Class Image With Scale",
			"ENERGY_CLASS_IMAGE_WITH_SCALE",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.EnergyClassImageWithScale,
		},
		{
			"Energy Class Range",
			"ENERGY_CLASS_RANGE",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.EnergyClassRange,
		},
		{
			"Implementing Act",
			"IMPLEMENTING_ACT",
			"string",
			func(c document.Claim) string { return c.(*document.StringClaim).String },
			washerDrier.ImplementingAct,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := doc.Get(document.GetCorePropertyID(tt.propName))
			if len(claims) == 0 {
				t.Errorf("no claims found for property %s", tt.propName)
				return
			}

			found := false
			for _, claim := range claims {
				// First verify the claim type
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

				value := tt.getValue(claim)
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

// func setupTestContainers(t *testing.T) (string, string, func()) {
// 	ctx := context.Background()

// 	// Create Docker network
// 	network, err := dockertest.CreateNetwork("eprel-test-network")
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Start PostgreSQL
// 	pgContainer, err := dockertest.RunWithOptions(&dockertest.RunOptions{
// 		Repository: "postgres",
// 		Tag: "16",
// 		NetworkID: network.ID,
// 		Name: "eprel-test-postgres",
// 		Env: []string{
// 			"POSTGRES_USER=test",
// 			"POSTGRES_PASSWORD=test",
// 			"POSTGRES_DB=test",
// 		},
// 		ExposedPorts: []string{"5432"},
// 	})
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Start Elasticsearch
// 	esContainer, err := dockertest.NewContainer("elasticsearch:7.16.3")
// 		.WithNetwork(network)
// 		.WithName("eprel-test-elasticsearch")
// 		.WithEnv("discovery.type=single-node")
// 		.WithEnv("xpack.security.enabled=false")
// 		.WithExposedPorts("9200")
// 		.Start(ctx)
// 	if err != nil {
// 		network.Close()
// 		pgContainer.Close()
// 		t.Fatal(err)
// 	}

// 	// Get connection strings
// 	pgURL := fmt.Sprintf("postgres://test:test@localhost:%s/test", pgContainer.MappedPort("5432"))
// 	esURL := fmt.Sprintf("http://localhost:%s", esContainer.MappedPort("9200"))

// 	// Return cleanup function
// 	cleanup := func() {
// 		esContainer.Close()
// 		pgContainer.Close()
// 		network.Close()
// 	}

// 	return pgURL, esURL, cleanup
// }
