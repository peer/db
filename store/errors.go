package store

import "gitlab.com/tozd/go/errors"

var (
	ErrValueNotFound    = errors.Base("value not found")
	ErrValueDeleted     = errors.Base("value deleted")
	ErrAlreadyCommitted = errors.Base("changeset already committed")
	ErrConflict         = errors.Base("conflict")
)
