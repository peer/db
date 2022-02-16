package wikipedia

import (
	"gitlab.com/tozd/go/errors"
)

var (
	SkippedError       = errors.Base("skipped")
	SilentSkippedError = errors.BaseWrap(SkippedError, "silent skipped")
)
