package peerdb

import (
	"net/url"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

//nolint:gochecknoglobals
var TestingClearDirContents = clearDirContents

// TestingShortcutQueryGroup is an exported view of one filter group parsed from search shortcut query
// parameters, used by external tests to assert the parsed property key and its To, Direct, and Missing
// selections. It embeds the parsed key and group so their exported fields are promoted as-is.
type TestingShortcutQueryGroup struct {
	shortcutPropKey
	shortcutQueryGroup
}

// TestingParseShortcutQueryGroups wraps parseShortcutQueryGroups, returning the per-property groups as a
// slice of exported values (order-independent) together with the optional reverse target, language, and
// full-text query.
func TestingParseShortcutQueryGroups(query url.Values) ([]TestingShortcutQueryGroup, *identifier.Identifier, string, string, errors.E) {
	groups, reverse, language, fullTextQuery, errE := parseShortcutQueryGroups(query)
	if errE != nil {
		return nil, nil, "", "", errE
	}
	out := make([]TestingShortcutQueryGroup, 0, len(groups))
	for key, group := range groups {
		out = append(out, TestingShortcutQueryGroup{shortcutPropKey: key, shortcutQueryGroup: *group})
	}
	return out, reverse, language, fullTextQuery, nil
}
