package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/transform"
)

func TestValidateEmbed(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=ns.example.com,B2")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("source path", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=ns.example.com,B2:ns.example.com,B3")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("multiple entries", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=ns.example.com,B2&ns.example.com,C1=ns.example.com,D2")
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("base58 identifiers", func(t *testing.T) {
		t.Parallel()

		destination := identifier.New().String()
		source := identifier.New().String()
		errE := transform.TestingValidateEmbed(destination + "=" + source)
		require.NoError(t, errE, "% -+#.1v", errE)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("")
		require.Error(t, errE)
		assert.EqualError(t, errE, "embed must not be empty")
	})

	t.Run("missing equals", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1")
		require.Error(t, errE)
		assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
	})

	t.Run("empty source", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=")
		require.Error(t, errE)
		assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
	})

	t.Run("empty destination", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("=ns.example.com,B2")
		require.Error(t, errE)
		assert.EqualError(t, errE, "entry must have a non-empty key and value separated by '='")
	})

	t.Run("multi-segment destination", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1:ns.example.com,A2=ns.example.com,B2")
		require.Error(t, errE)
		assert.EqualError(t, errE, "embed entry destination must be a single segment")
	})

	t.Run("invalid source identifier", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=bogus")
		require.Error(t, errE)
	})

	t.Run("empty part in source", func(t *testing.T) {
		t.Parallel()

		errE := transform.TestingValidateEmbed("ns.example.com,A1=ns.example.com,,B2")
		require.Error(t, errE)
	})
}
