package indexer

import (
	"time"
)

const (
	// ProgressPrintRate is the interval at which progress updates are printed, same as go-mediawiki's progressPrintRate.
	ProgressPrintRate = 30 * time.Second
)
