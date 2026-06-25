package peerdb

import (
	"net/url"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

//nolint:gochecknoglobals
var TestingClearDirContents = clearDirContents

// Embeds the parsed key and group so their exported fields are promoted as-is.
type TestingShortcutQueryGroup struct {
	shortcutPropKey
	shortcutQueryGroup
}

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
