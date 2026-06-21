package peerdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb"
)

func TestClearDirContents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Populate the directory with a top-level file and a nested subdirectory with a file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assemble-abc.tmp"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "hash"), []byte("y"), 0o600))

	errE := peerdb.TestingClearDirContents(dir)
	require.NoError(t, errE, "% -+#.1v", errE)

	// The directory itself remains in place, but is now empty.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// A missing directory is treated as already empty.
	errE = peerdb.TestingClearDirContents(filepath.Join(dir, "does-not-exist"))
	assert.NoError(t, errE, "% -+#.1v", errE)
}
