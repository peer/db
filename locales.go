package peerdb

import (
	"embed"
	"io/fs"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// localeFS embeds the frontend translation catalogs. They are the single source for the special value
// labels the filter-pane value search matches against, so those labels stay identical to what the
// frontend renders without a hand-maintained copy in the search package.
//
//go:embed src/locales/*.json
var localeFS embed.FS

// init loads the special value search labels from the embedded locale files into the search package.
// The files are embedded and validated by the frontend build, so a failure here is a build-time error.
func init() { //nolint:gochecknoinits
	sub, err := fs.Sub(localeFS, "src/locales")
	if err != nil {
		panic(err)
	}
	errE := internalSearch.LoadSpecialSearchLabels(sub)
	if errE != nil {
		panic(errE)
	}
}
