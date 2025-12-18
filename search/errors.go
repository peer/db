package search

import (
	"gitlab.com/tozd/go/errors"
)

var (
	ErrNotFound         = errors.Base("not found")
	ErrValidationFailed = errors.Base("validation failed")
)
