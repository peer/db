package search

import (
	"gitlab.com/tozd/go/errors"
)

var (
	ErrNotFound        = errors.Base("not found")
	ErrInvalidArgument = errors.Base("invalid argument")
)
