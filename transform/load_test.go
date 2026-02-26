package transform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/transform"
)

//nolint:gochecknoglobals
var isRootUser = os.Getuid() == 0

type TestJSONData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("BasicJSON", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"name":"Alice","value":1}`), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"name":"Bob","value":2}`), 0o600)
		require.NoError(t, err)

		results, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, results, 2)

		// Results are pointers to TestJSONData.
		d0, ok := results[0].(*TestJSONData)
		require.True(t, ok)
		d1, ok := results[1].(*TestJSONData)
		require.True(t, ok)

		// Files are walked in alphabetical order.
		assert.Equal(t, "Alice", d0.Name)
		assert.Equal(t, 1, d0.Value)
		assert.Equal(t, "Bob", d1.Name)
		assert.Equal(t, 2, d1.Value)
	})

	t.Run("NonExistentDirectory", func(t *testing.T) {
		t.Parallel()

		results, errE := transform.Load[TestJSONData](t.Context(), "/nonexistent/path/that/does/not/exist")
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, results)
	})

	t.Run("EmptyDirectory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		results, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, results)
	})

	t.Run("SkipsNonJSONFiles", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"name":"Alice","value":1}`), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "data.yaml"), []byte("name: Alice"), 0o600)
		require.NoError(t, err)

		results, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, results, 1)

		d, ok := results[0].(*TestJSONData)
		require.True(t, ok)
		assert.Equal(t, "Alice", d.Name)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{invalid json`), 0o600)
		require.NoError(t, err)

		_, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.Error(t, errE)
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"name":"Alice","value":1}`), 0o600)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, errE := transform.Load[TestJSONData](ctx, dir)
		require.Error(t, errE)
		assert.ErrorIs(t, errE, context.Canceled)
	})

	t.Run("SubdirectoryRecursive", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		subdir := filepath.Join(dir, "sub")
		err := os.Mkdir(subdir, 0o700)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "root.json"), []byte(`{"name":"Root","value":0}`), 0o600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir, "nested.json"), []byte(`{"name":"Nested","value":99}`), 0o600)
		require.NoError(t, err)

		results, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.NoError(t, errE, "% -+#.1v", errE)

		// Both root and nested files are loaded (WalkDir is recursive).
		assert.Len(t, results, 2)
	})

	t.Run("UnreadableFile", func(t *testing.T) {
		t.Parallel()

		if isRootUser {
			t.Skip("Skipping permission test when running as root.")
		}

		dir := t.TempDir()
		jsonFile := filepath.Join(dir, "data.json")
		err := os.WriteFile(jsonFile, []byte(`{"name":"Alice","value":1}`), 0o600)
		require.NoError(t, err)

		// Make the file unreadable.
		err = os.Chmod(jsonFile, 0o000)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Chmod(jsonFile, 0o600) })

		_, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.Error(t, errE)
	})

	t.Run("UnreadableDirectory", func(t *testing.T) {
		t.Parallel()

		if isRootUser {
			t.Skip("Skipping permission test when running as root.")
		}

		dir := t.TempDir()
		subdir := filepath.Join(dir, "sub")
		err := os.Mkdir(subdir, 0o700)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir, "data.json"), []byte(`{"name":"Alice","value":1}`), 0o600)
		require.NoError(t, err)

		// Make the subdirectory unreadable (prevents WalkDir from listing its contents).
		err = os.Chmod(subdir, 0o000)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Chmod(subdir, 0o700) }) //nolint:gosec

		_, errE := transform.Load[TestJSONData](t.Context(), dir)
		require.Error(t, errE)
	})
}
