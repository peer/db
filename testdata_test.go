package peerdb_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/internal/xeno"
	"gitlab.com/peerdb/peerdb/transform"
)

// testDataDir is the directory the populate command is pointed at to populate with the test data set.
const testDataDir = "testdata"

// attachmentLinkRe matches the links test data documents point at their attachments with. The
// identifier is derived from the file's path, so a link can be checked against the files which are
// really there.
var attachmentLinkRe = regexp.MustCompile(`/f/([0-9A-Za-z]{22})`)

func testDataContext(t *testing.T) context.Context {
	t.Helper()

	logger := zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.WarnLevel)
	return logger.WithContext(t.Context())
}

// TestTestDataLoads verifies that every class has test data and that the data unmarshals into the
// class's struct. Unmarshaling rejects unknown fields, so this catches drift between the field
// definitions and the test data: a renamed field, or a field changing between a single value and a
// list of values.
func TestTestDataLoads(t *testing.T) {
	t.Parallel()

	for _, class := range peerdb.TestingTestDataClasses {
		t.Run(class.Directory, func(t *testing.T) {
			t.Parallel()

			documents, errE := class.Load(t.Context(), filepath.Join(testDataDir, class.Directory))
			require.NoError(t, errE, "% -+#.1v", errE)
			assert.NotEmpty(t, documents, "no test data documents loaded")
		})
	}
}

// TestTestDataTransforms verifies that the test data transforms into documents together with the core
// and test data schema documents. Transforming enforces the cardinality and the value type of every
// field, so this is the same check the populate command does.
func TestTestDataTransforms(t *testing.T) {
	t.Parallel()

	ctx := testDataContext(t)

	documents, errE := peerdb.TestingLoadTestData(ctx, *zerolog.Ctx(ctx), testDataDir)
	require.NoError(t, errE, "% -+#.1v", errE)

	all, transformed, errE := peerdb.TestingGenerateTestDataDocs(ctx, *zerolog.Ctx(ctx), documents)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Len(t, transformed, len(all))
	assert.Greater(t, len(transformed), len(documents))
}

// TestTestDataReferences verifies that the test data has no duplicate document ID and that every
// reference in it points at a document which exists, either in the test data itself or among the core
// and test data schema documents. A dangling reference does not fail populating, but it leaves the
// data with a link nothing resolves.
func TestTestDataReferences(t *testing.T) {
	t.Parallel()

	ctx := testDataContext(t)

	testData, errE := peerdb.TestingLoadTestData(ctx, *zerolog.Ctx(ctx), testDataDir)
	require.NoError(t, errE, "% -+#.1v", errE)

	generated, _, errE := peerdb.TestingGenerateTestDataDocs(ctx, *zerolog.Ctx(ctx), nil)
	require.NoError(t, errE, "% -+#.1v", errE)

	known := map[string]bool{}
	for _, doc := range append(slices.Clone(generated), testData...) {
		id, errE := transform.ExtractDocumentID(doc)
		require.NoError(t, errE, "% -+#.1v", errE)
		key := strings.Join(id, "/")
		assert.False(t, known[key], "duplicate document ID %q", key)
		known[key] = true
	}

	for path, contents := range testDataJSONFiles(t) {
		var doc map[string]any
		require.NoError(t, json.Unmarshal(contents, &doc), "%s", path)

		for _, reference := range references(doc) {
			assert.True(t, known[reference], "%s: reference to unknown document %q", path, reference)
		}
	}
}

// TestTestDataAttachments verifies that every attachment link in the test data points at a file which
// is there, that every file is linked from somewhere, and that every attachment is named after the
// file it points at. Without the name the interface has nothing to show for the attachment but the
// link itself.
func TestTestDataAttachments(t *testing.T) {
	t.Parallel()

	root := filepath.Join(testDataDir, peerdb.TestingTestDataFilesDirectory)
	linked := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if d.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, p)
		if err != nil {
			return errors.WithStack(err)
		}
		linked[identifier.From(xeno.Namespace, xeno.FilesStorage, name).String()] = name
		return nil
	})
	require.NoError(t, err)
	assert.NotEmpty(t, linked, "no attachments")

	used := map[string]bool{}
	for path, contents := range testDataJSONFiles(t) {
		for _, match := range attachmentLinkRe.FindAllStringSubmatch(string(contents), -1) {
			assert.Contains(t, linked, match[1], "%s: link to unknown attachment %q", path, match[0])
			used[match[1]] = true
		}

		var doc map[string]any
		require.NoError(t, json.Unmarshal(contents, &doc), "%s", path)

		for _, attachment := range attachments(doc) {
			id := strings.TrimPrefix(attachment.Link, "/f/")
			assert.Equal(t, filepath.Base(linked[id]), attachment.Name, "%s: attachment %q is named for another file", path, attachment.Link)
		}
	}

	for id, name := range linked {
		assert.True(t, used[id], "attachment %q is not linked from any document", name)
	}
}

// testDataJSONFiles returns the contents of every test data document, keyed by its path.
func testDataJSONFiles(t *testing.T) map[string][]byte {
	t.Helper()

	files := map[string][]byte{}
	for _, class := range peerdb.TestingTestDataClasses {
		directory := filepath.Join(testDataDir, class.Directory)
		entries, err := os.ReadDir(directory)
		require.NoError(t, err)

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			contents, err := os.ReadFile(path) //nolint:gosec
			require.NoError(t, err)
			files[path] = contents
		}
	}
	return files
}

// attachment is one attachment found in a test data document: the link it points at the file with,
// and the name it gives that file.
type attachment struct {
	Link string
	Name string
}

// attachments collects every attachment in the document. An attachment is a value which is a link
// into the file storage, whatever field it sits in.
func attachments(doc map[string]any) []attachment {
	found := []attachment{}

	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if link, ok := v["value"].(string); ok && strings.HasPrefix(link, "/f/") {
				name, _ := v["name"].(string)
				found = append(found, attachment{Link: link, Name: name})
			}
			for _, sub := range v {
				walk(sub)
			}
		case []any:
			for _, sub := range v {
				walk(sub)
			}
		}
	}

	walk(doc)

	return found
}

// references collects every reference in the document, as a slash-joined document ID. The document's
// own ID is not a reference, so the top-level "id" is skipped.
func references(doc map[string]any) []string {
	found := []string{}

	var walk func(value any, isDocumentID bool)
	walk = func(value any, isDocumentID bool) {
		switch v := value.(type) {
		case map[string]any:
			for key, sub := range v {
				// A reference is an object whose only content is the "id" of the target document.
				if key == "id" && !isDocumentID {
					if id, ok := stringSlice(sub); ok {
						found = append(found, strings.Join(id, "/"))
						continue
					}
				}
				walk(sub, false)
			}
		case []any:
			for _, sub := range v {
				walk(sub, isDocumentID)
			}
		}
	}

	for key, value := range doc {
		walk(value, key == "id")
	}

	return found
}

// stringSlice converts a JSON array of strings into a string slice.
func stringSlice(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, v := range values {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		result = append(result, s)
	}
	return result, true
}
