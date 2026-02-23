package transform

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

// Load loads all JSON files from path as values of type T and returns a slice of
// pointers to those values.
func Load[T any](ctx context.Context, path string) ([]any, errors.E) {
	var values []any

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		// Skip if file or directory (even path) does not exist.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return errors.WithStack(err)
		}

		// We skip directories.
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(p, ".json") {
			return nil
		}

		data, err := os.ReadFile(p) //nolint:gosec
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["path"] = p
			return errE
		}

		var value T
		errE := x.UnmarshalWithoutUnknownFields(data, &value)
		if errE != nil {
			errors.Details(errE)["path"] = p
			return errE
		}

		values = append(values, &value)

		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return values, nil
}
