package indexer

import (
	"time"
)

const (
	// ProgressPrintRate is the interval at which progress updates are printed, same as go-mediawiki's progressPrintRate.
	ProgressPrintRate = 30 * time.Second

	// PreviewSize is the minimum width and height of an image to be considered for preview.
	PreviewSize = 256

	clientRetryWaitMax = 10 * 60 * time.Second
	clientRetryMax     = 9
)
