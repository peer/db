package storage

import "gitlab.com/tozd/go/errors"

var (
	ErrEndNotPossible = errors.Base("end upload not possible")
	ErrInvalidChunk   = errors.Base("invalid chunk")
)
