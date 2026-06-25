package search

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func (b *Bridge) TestingSetReindexSoftDeadline(d time.Duration) {
	b.reindexSoftDeadline = d
}

func (b *Bridge) TestingSetMaxContentLength(n int) {
	b.maxContentLength = n
}

func (b *Bridge) TestingScheduleReindexJob(ctx context.Context, prefix string) errors.E {
	_, err := b.riverClient.Insert(ctx, jobArgs{Prefix: prefix}, nil)
	return errors.WithStack(err)
}

func (b *Bridge) TestingCountAvailableReindexJobs(ctx context.Context, prefix string) (int, errors.E) {
	var count int
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.WithPgxError(
			tx.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE "kind" = $1 AND "state" = 'available' AND "args"->>'prefix' = $2`, jobArgs{}.Kind(), prefix).Scan(&count),
		)
	})
	return count, errE
}

func (b *Bridge) TestingDeletePendingReindexJobs(ctx context.Context) (int64, errors.E) {
	return b.deletePendingReindexJobs(ctx)
}
