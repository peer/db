package wikipedia

import (
	"gitlab.com/tozd/go/errors"
)

var (
	ErrSkipped       = errors.Base("skipped")
	ErrSilentSkipped = errors.BaseWrap(ErrSkipped, "silent skipped")
)
