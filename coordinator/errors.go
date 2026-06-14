package coordinator

import "gitlab.com/tozd/go/errors"

var (
	ErrSessionNotFound   = errors.Base("session not found")
	ErrOperationNotFound = errors.Base("operation not found")
	ErrAlreadyEnded      = errors.Base("session already ended")
	ErrNotEnded          = errors.Base("session not ended")
	ErrAlreadyCompleted  = errors.Base("session already completed")
	ErrConflict          = errors.Base("conflict")
	// ErrInvalidSessionData is a base error for session data which deterministically
	// fails validation when completing the session, so retrying cannot succeed.
	ErrInvalidSessionData = errors.Base("invalid session data")
)
