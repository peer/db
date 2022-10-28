package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

//go:embed testdata
var content embed.FS

func TestExtractArtist(t *testing.T) {
	entries, err := content.ReadDir("testdata/artist")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.html") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.html")
		t.Run(base, func(t *testing.T) {
			input, err := content.ReadFile(filepath.Join("testdata", "artist", entry.Name()))
			require.NoError(t, err)
			outputData, err := extractData[momaArtist](bytes.NewReader(input))
			require.NoError(t, err)
			outputJSON, err := x.MarshalWithoutEscapeHTML(outputData)
			require.NoError(t, err)
			var buf bytes.Buffer
			err = json.Indent(&buf, outputJSON, "", "  ")
			require.NoError(t, err)
			output := buf.Bytes()
			expectedFilePath := filepath.Join("testdata", "artist", base+"_out.json")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				_, _ = f.Write(output)
			} else {
				assert.JSONEq(t, string(expected), string(output))
			}
		})
	}
}

func TestExtractArtwork(t *testing.T) {
	entries, err := content.ReadDir("testdata/artwork")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.html") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.html")
		t.Run(base, func(t *testing.T) {
			input, err := content.ReadFile(filepath.Join("testdata", "artwork", entry.Name()))
			require.NoError(t, err)
			outputData, err := extractData[momaArtwork](bytes.NewReader(input))
			require.NoError(t, err)
			outputJSON, err := x.MarshalWithoutEscapeHTML(outputData)
			require.NoError(t, err)
			var buf bytes.Buffer
			err = json.Indent(&buf, outputJSON, "", "  ")
			require.NoError(t, err)
			output := buf.Bytes()
			expectedFilePath := filepath.Join("testdata", "artwork", base+"_out.json")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				_, _ = f.Write(output)
			} else {
				assert.JSONEq(t, string(expected), string(output))
			}
		})
	}
}
