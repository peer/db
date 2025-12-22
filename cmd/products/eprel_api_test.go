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

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()
	httpClient := es.NewHTTPClient(cleanhttp.DefaultPooledClient(), logger)
	ctx := context.Background()

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
				outputDoc, errE := makeWasherDrierDoc(ctx, logger, httpClient.StandardClient(), result.Hits[i])
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
