package search_test

import (
	"os"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// init loads the value labels for the special-value search tests from the on-disk frontend locale
// files. In the application the main package loads them from the embedded src/locales on init; the
// search package tests do not import the main package, so they load the same files by relative path
// (test binaries run with the package directory as the working directory).
func init() { //nolint:gochecknoinits
	errE := internalSearch.LoadSpecialSearchLabels(os.DirFS("../src/locales"))
	if errE != nil {
		panic(errE)
	}
}
