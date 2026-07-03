package peerdb_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/search"
)

func TestParseShortcutQueryGroups(t *testing.T) {
	t.Parallel()

	prop := identifier.New()
	parent := identifier.New()
	value1 := identifier.New()
	value2 := identifier.New()
	value3 := identifier.New()
	doc := identifier.New()

	t.Run("plain target values", func(t *testing.T) {
		t.Parallel()

		groups, reverse, ids, language, fullTextQuery, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			prop.String(): {value1.String(), value2.String()},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Nil(t, reverse)
		assert.Empty(t, ids)
		assert.Empty(t, language)
		assert.Empty(t, fullTextQuery)
		require.Len(t, groups, 1)
		assert.Equal(t, prop, groups[0].Prop)
		assert.False(t, groups[0].Nested)
		assert.Equal(t, []search.ToValue{{ID: value1}, {ID: value2}}, groups[0].To)
		assert.Nil(t, groups[0].Direct)
		assert.False(t, groups[0].Missing)
	})

	t.Run("missing bucket", func(t *testing.T) {
		t.Parallel()

		groups, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{prop.String(): {"missing"}})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, groups, 1)
		assert.Nil(t, groups[0].To)
		assert.Nil(t, groups[0].Direct)
		assert.True(t, groups[0].Missing)
	})

	t.Run("direct target", func(t *testing.T) {
		t.Parallel()

		groups, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{prop.String(): {"direct:" + value1.String()}})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, groups, 1)
		assert.Nil(t, groups[0].To)
		assert.Equal(t, []search.ToValue{{ID: value1}}, groups[0].Direct)
		assert.False(t, groups[0].Missing)
	})

	t.Run("mixed to, direct, and missing", func(t *testing.T) {
		t.Parallel()

		groups, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			prop.String(): {value1.String(), "direct:" + value2.String(), "missing", value3.String()},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, groups, 1)
		assert.Equal(t, []search.ToValue{{ID: value1}, {ID: value3}}, groups[0].To)
		assert.Equal(t, []search.ToValue{{ID: value2}}, groups[0].Direct)
		assert.True(t, groups[0].Missing)
	})

	t.Run("nested key with direct", func(t *testing.T) {
		t.Parallel()

		groups, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			parent.String() + ":" + prop.String(): {"direct:" + value1.String()},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, groups, 1)
		assert.True(t, groups[0].Nested)
		assert.Equal(t, parent, groups[0].Parent)
		assert.Equal(t, prop, groups[0].Prop)
		assert.Equal(t, []search.ToValue{{ID: value1}}, groups[0].Direct)
	})

	t.Run("reverse, language, and query", func(t *testing.T) {
		t.Parallel()

		groups, reverse, ids, language, fullTextQuery, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			"reverse":  {doc.String()},
			"language": {"sl"},
			"q":        {"hello world"},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, groups)
		require.NotNil(t, reverse)
		assert.Equal(t, doc, *reverse)
		assert.Empty(t, ids)
		assert.Equal(t, "sl", language)
		assert.Equal(t, "hello world", fullTextQuery)
	})

	t.Run("id scope", func(t *testing.T) {
		t.Parallel()

		groups, reverse, ids, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			"id": {value1.String(), value2.String()},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Empty(t, groups)
		assert.Nil(t, reverse)
		assert.Equal(t, []identifier.Identifier{value1, value2}, ids)
	})

	t.Run("id scope with property values", func(t *testing.T) {
		t.Parallel()

		groups, _, ids, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{
			"id":          {value1.String()},
			prop.String(): {value2.String()},
		})
		require.NoError(t, errE, "% -+#.1v", errE)
		require.Len(t, groups, 1)
		assert.Equal(t, []search.ToValue{{ID: value2}}, groups[0].To)
		assert.Equal(t, []identifier.Identifier{value1}, ids)
	})

	t.Run("invalid id identifier", func(t *testing.T) {
		t.Parallel()

		_, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{"id": {"bogus"}})
		require.Error(t, errE)
		assert.ErrorContains(t, errE, `"id" query parameter value is not a valid identifier`)
	})

	t.Run("invalid direct identifier", func(t *testing.T) {
		t.Parallel()

		_, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{prop.String(): {"direct:bogus"}})
		require.Error(t, errE)
		assert.ErrorContains(t, errE, "query parameter direct value is not a valid identifier")
	})

	t.Run("invalid target identifier", func(t *testing.T) {
		t.Parallel()

		_, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{prop.String(): {"bogus"}})
		require.Error(t, errE)
		assert.ErrorContains(t, errE, "query parameter value is not a valid identifier")
	})

	t.Run("reverse set twice", func(t *testing.T) {
		t.Parallel()

		_, _, _, _, _, errE := peerdb.TestingParseShortcutQueryGroups(url.Values{"reverse": {doc.String(), value1.String()}})
		require.Error(t, errE)
		assert.ErrorContains(t, errE, `"reverse" query parameter must be set exactly once`)
	})
}
