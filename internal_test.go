package peerdb

import (
	"net/url"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

//nolint:gochecknoglobals
var TestingClearDirContents = clearDirContents

// TestingRoutePaths returns the route name to path-template map that setRoutes configures,
// including debugging routes when development is true.
func TestingRoutePaths(development bool) map[string]string {
	var s Service
	s.Development = development
	s.setRoutes()
	paths := make(map[string]string, len(s.Routes))
	for name, route := range s.Routes {
		paths[name] = route.Path
	}
	return paths
}

// Embeds the parsed key and group so their exported fields are promoted as-is.
type TestingShortcutQueryGroup struct {
	shortcutPropKey
	shortcutQueryGroup
}

func TestingParseShortcutQueryGroups(query url.Values) ([]TestingShortcutQueryGroup, *identifier.Identifier, []identifier.Identifier, string, string, errors.E) {
	groups, reverse, ids, language, fullTextQuery, errE := parseShortcutQueryGroups(query)
	if errE != nil {
		return nil, nil, nil, "", "", errE
	}
	out := make([]TestingShortcutQueryGroup, 0, len(groups))
	for key, group := range groups {
		out = append(out, TestingShortcutQueryGroup{shortcutPropKey: key, shortcutQueryGroup: *group})
	}
	return out, reverse, ids, language, fullTextQuery, nil
}
