package main

import (
	"context"
	"os"
	"strings"
	"testing"

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
