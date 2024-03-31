package store

import "gitlab.com/tozd/go/errors"

var (
	ErrViewNotFound       = errors.Base("view not found")
	ErrValueNotFound      = errors.Base("value not found")
	ErrValueDeleted       = errors.BaseWrap(ErrValueNotFound, "value deleted")
	ErrAlreadyCommitted   = errors.Base("changeset already committed")
	ErrParentNotCommitted = errors.Base("parent changeset not committed")
	ErrConflict           = errors.Base("conflict")
)
