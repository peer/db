package search

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

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

// TestingScheduleReindexJob enqueues a reindex job for the given store prefix through the bridge's river
// client, the way updateSeq does, so a test can build up a backlog of pending jobs.
func (b *Bridge) TestingScheduleReindexJob(ctx context.Context, prefix string) errors.E {
	_, err := b.riverClient.Insert(ctx, jobArgs{Prefix: prefix}, nil)
	return errors.WithStack(err)
}

// TestingCountAvailableReindexJobs returns how many reindex jobs are in the available state for the given
// store prefix.
func (b *Bridge) TestingCountAvailableReindexJobs(ctx context.Context, prefix string) (int, errors.E) {
	var count int
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.WithPgxError(
			tx.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE "kind" = $1 AND "state" = 'available' AND "args"->>'prefix' = $2`, jobArgs{}.Kind(), prefix).Scan(&count),
		)
	})
	return count, errE
}

// TestingDeletePendingReindexJobs exposes deletePendingReindexJobs (which targets the bridge's own prefix).
func (b *Bridge) TestingDeletePendingReindexJobs(ctx context.Context) (int64, errors.E) {
	return b.deletePendingReindexJobs(ctx)
}
