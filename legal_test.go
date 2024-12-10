package peerdb_test

import (
	"testing"
)

func TestRouteLicense(t *testing.T) {
	t.Parallel()

	testStaticFile(t, "License", "dist/LICENSE.txt", "text/plain; charset=utf-8")
}

func TestRouteNotice(t *testing.T) {
	t.Parallel()

	testStaticFile(t, "Notice", "dist/NOTICE.txt", "text/plain; charset=utf-8")
}
