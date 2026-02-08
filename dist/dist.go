// Package dist provides built frontend files.
package dist

import (
	"embed"
	"io/fs"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

//go:embed *
var files embed.FS

// Files exposes files inside dist directory, except for the dist.go file.
//
//nolint:gochecknoglobals
var Files fs.FS

//nolint:gochecknoinits
func init() {
	var errE errors.E
	Files, errE = x.MakeFilteredFS(files, "dist.go")
	if errE != nil {
		panic(errE)
	}
}
