package search

import "time"

// TestingSetReindexSoftDeadline overrides the soft deadline that bounds how long a reindex job drains
// the queue before flushing what it has and scheduling a follow-up job. Tests use it to force the
// follow-up continuation path without having to enqueue enough work to run for minutes.
func (b *Bridge) TestingSetReindexSoftDeadline(d time.Duration) {
	b.reindexSoftDeadline = d
}

// TestingSetMaxContentLength overrides the ElasticSearch http.max_content_length the bridge uses to size
// bulk requests. Tests set it small to force the payload-size flush path with tiny documents.
func (b *Bridge) TestingSetMaxContentLength(n int) {
	b.maxContentLength = n
}
