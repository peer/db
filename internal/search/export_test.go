package search

import "time"

// TestingSetReindexSoftDeadline overrides the soft deadline that bounds how long a reindex job drains
// the queue before flushing what it has and scheduling a follow-up job. Tests use it to force the
// follow-up continuation path without having to enqueue enough work to run for minutes.
func (b *Bridge) TestingSetReindexSoftDeadline(d time.Duration) {
	b.reindexSoftDeadline = d
}
