package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/transform"
)

func TestValidateShortcut(t *testing.T) {
	t.Parallel()

	t.Run("multi-part key and value", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("core.namespace,INSTANCE_OF=ns.example.com,A")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("multiple parts with &", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND=ns.example.com,OPT_A&ns.example.com,OTHER=ns.example.com,OPT_B")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("nested key", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,LOCATED_IN:ns.example.com,COUNTRY=ns.example.com,FRANCE")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifier in key and value", func(t *testing.T) {
		t.Parallel()

		id := identifier.New().String()
		errE := transform.TestingValidateShortcut(id + "=" + id)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifier in value with multi-part key", func(t *testing.T) {
		t.Parallel()

		id := identifier.New().String()
		errE := transform.TestingValidateShortcut("ns.example.com,KIND=" + id)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifier in key with multi-part value", func(t *testing.T) {
		t.Parallel()

		id := identifier.New().String()
		errE := transform.TestingValidateShortcut(id + "=ns.example.com,A")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifier in nested key", func(t *testing.T) {
		t.Parallel()

		parent := identifier.New().String()
		prop := identifier.New().String()
		errE := transform.TestingValidateShortcut(parent + ":" + prop + "=ns.example.com,A")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifier in reverse value", func(t *testing.T) {
		t.Parallel()

		id := identifier.New().String()
		errE := transform.TestingValidateShortcut("reverse=" + id)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("reverse key", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("reverse=ns.example.com,DOC")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("self value", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND=self")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("")
		require.Error(t, errE)
		assert.EqualError(t, errE, "search shortcut must not be empty")
	})

	t.Run("missing equals", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND")
		require.Error(t, errE)
		assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
	})

	t.Run("empty value", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND=")
		require.Error(t, errE)
		assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
	})

	t.Run("key with too many colons", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("a:b:c=ns.example.com,D")
		require.Error(t, errE)
		assert.EqualError(t, errE, "search shortcut key must contain at most one ':'")
	})

	t.Run("reverse inside nested key", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("reverse:ns.example.com,X=ns.example.com,Y")
		require.Error(t, errE)
		assert.EqualError(t, errE, `"reverse" is not allowed inside a nested key`)
	})

	t.Run("invalid identifier in value", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND=bogus")
		require.Error(t, errE)
	})

	t.Run("empty part inside multi-part identifier", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateShortcut("ns.example.com,KIND=ns.example.com,,A")
		require.Error(t, errE)
		assert.EqualError(t, errE, "empty identifier part")
	})
}
