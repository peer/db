package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
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

func TestMakeWasherDrierDoc(t *testing.T) {
	entries, err := content.ReadDir("testdata/eprel")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.json") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.json")
		t.Run(base, func(t *testing.T) {
			input, err := content.ReadFile(filepath.Join("testdata/eprel", entry.Name()))
			require.NoError(t, err)

			type washerDrierResponse struct {
				Offset int                  `json:"offset"`
				Size   int                  `json:"size"`
				Hits   []WasherDrierProduct `json:"hits"`
			}

			var result washerDrierResponse
			errE := x.UnmarshalWithoutUnknownFields(input, &result)
			require.NoError(t, errE, "% -+#.1v", errE)

			for i := range result.Hits {
				outputDoc, errE := makeWasherDrierDoc(result.Hits[i])
				require.NoError(t, errE, "% -+#.1v", errE)
				outputJSON, errE := x.MarshalWithoutEscapeHTML(outputDoc)
				require.NoError(t, errE, "% -+#.1v", errE)
				var buf bytes.Buffer
				err := json.Indent(&buf, outputJSON, "", "  ")
				require.NoError(t, err)
				output := buf.Bytes()
				expectedFilePath := filepath.Join("testdata/eprel", fmt.Sprintf("%s_%03d_out.json", base, i))
				expected, err := content.ReadFile(expectedFilePath)
				if errors.Is(err, fs.ErrNotExist) {
					f, err := os.Create(expectedFilePath)
					require.NoError(t, err)
					_, _ = f.Write(output)
				} else {
					assert.JSONEq(t, string(expected), string(output))
				}
			}
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
