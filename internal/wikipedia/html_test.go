package wikipedia_test

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/internal/wikipedia"
)

//go:embed testdata
var content embed.FS

func TestExtractArticle(t *testing.T) {
	t.Parallel()

	entries, err := content.ReadDir("testdata/article")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.html") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.html")
		entry := entry
		t.Run(base, func(t *testing.T) {
			t.Parallel()

			input, err := content.ReadFile(filepath.Join("testdata", "article", entry.Name()))
			require.NoError(t, err)
			output, _, err := wikipedia.ExtractArticle(string(input))
			require.NoError(t, err)
			expectedFilePath := filepath.Join("testdata", "article", base+"_out.html")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				_, _ = f.WriteString(output)
			} else {
				assert.Equal(t, string(expected), output)
			}
		})
	}
}

func TestExtractArticleSummary(t *testing.T) {
	t.Parallel()

	entries, err := content.ReadDir("testdata/article")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.html") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.html")
		entry := entry
		t.Run(base, func(t *testing.T) {
			t.Parallel()

			input, err := content.ReadFile(filepath.Join("testdata", "article", entry.Name()))
			require.NoError(t, err)
			_, doc, err := wikipedia.ExtractArticle(string(input))
			require.NoError(t, err)
			output, err := wikipedia.ExtractArticleSummary(doc)
			require.NoError(t, err)
			expectedFilePath := filepath.Join("testdata", "article", base+"_summary.html")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				_, _ = f.WriteString(output)
			} else {
				assert.Equal(t, string(expected), output)
			}
		})
	}
}

type outputStruct struct {
	Output []string `json:"output"`
}

func TestExtractFileDescriptions(t *testing.T) {
	t.Parallel()

	entries, err := content.ReadDir("testdata/file")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_in.html") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), "_in.html")
		entry := entry
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			input, err := content.ReadFile(filepath.Join("testdata", "file", entry.Name()))
			require.NoError(t, err)
			output, err := wikipedia.ExtractFileDescriptions(string(input))
			got := outputStruct{output}
			require.NoError(t, err)
			expectedFilePath := filepath.Join("testdata", "file", base+"_out.json")
			expected, err := content.ReadFile(expectedFilePath)
			if errors.Is(err, fs.ErrNotExist) {
				f, err := os.Create(expectedFilePath)
				require.NoError(t, err)
				data, err := x.MarshalWithoutEscapeHTML(got)
				require.NoError(t, err)
				_, _ = f.Write(data)
			} else {
				var e outputStruct
				err := x.UnmarshalWithoutUnknownFields(expected, &e)
				require.NoError(t, err)
				assert.Equal(t, e.Output, output)
			}
		})
	}
}

func TestExtractCategoryAndTemplateDescription(t *testing.T) {
	t.Parallel()

	for _, conf := range []struct {
		Dir  string
		Func func(string) (string, errors.E)
	}{
		{
			"category",
			wikipedia.ExtractCategoryDescription,
		},
		{
			"template",
			wikipedia.ExtractTemplateDescription,
		},
	} {
		conf := conf
		t.Run(conf.Dir, func(t *testing.T) {
			t.Parallel()

			entries, err := content.ReadDir("testdata/" + conf.Dir)
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
					input, err := content.ReadFile(filepath.Join("testdata", conf.Dir, entry.Name()))
					require.NoError(t, err)
					output, err := conf.Func(string(input))
					require.NoError(t, err)
					expectedFilePath := filepath.Join("testdata", conf.Dir, base+"_out.html")
					expected, err := content.ReadFile(expectedFilePath)
					if errors.Is(err, fs.ErrNotExist) {
						f, err := os.Create(expectedFilePath)
						require.NoError(t, err)
						_, _ = f.WriteString(output)
					} else {
						assert.Equal(t, string(expected), output)
					}
				})
			}
		})
	}
}
