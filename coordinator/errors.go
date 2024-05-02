package coordinator

import "gitlab.com/tozd/go/errors"

var (
	ErrSessionNotFound   = errors.Base("session not found")
	ErrOperationNotFound = errors.Base("operation not found")
	ErrAlreadyEnded      = errors.Base("session already ended")
	ErrConflict          = errors.Base("conflict")
)
