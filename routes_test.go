package peerdb_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb"
)

// normalizeRoutePath replaces every parameter segment (":id", ":prop", ":path*", ...) with a single
// wildcard marker so that two route templates which match the same concrete URLs compare equal.
func normalizeRoutePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = "*"
		}
	}
	return strings.Join(segments, "/")
}

// TestRoutesNoPathCollision asserts that no two routes share the same path shape.
func TestRoutesNoPathCollision(t *testing.T) {
	t.Parallel()

	byShape := map[string]string{}
	for name, path := range peerdb.TestingRoutePaths(true) {
		shape := normalizeRoutePath(path)
		other, ok := byShape[shape]
		assert.Falsef(t, ok, "routes %q and %q share the same path shape %q", name, other, shape)
		byShape[shape] = name
	}
}
