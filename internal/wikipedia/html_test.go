package wikipedia_test

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/search/internal/wikipedia"
)

//go:embed testdata
var content embed.FS

func TestConvertArticle(t *testing.T) {
	entries, err := content.ReadDir("testdata")
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
			input, err := content.ReadFile(filepath.Join("testdata", entry.Name()))
			require.NoError(t, err)
			output, err := wikipedia.ConvertArticle(string(input))
			require.NoError(t, err)
			expectedFilePath := filepath.Join("testdata", base+"_out.html")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				f.WriteString(output)
			} else {
				assert.Equal(t, string(expected), output)
			}
		})
	}
}
