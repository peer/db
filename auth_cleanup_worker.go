package peerdb

import (
	"context"

	internalSite "gitlab.com/peerdb/peerdb/internal/site"

	"github.com/riverqueue/river"

	"gitlab.com/peerdb/peerdb/base"
)

// authCleanupJobArgs are the arguments for the periodic auth cleanup
// job. No payload. Each invocation simply prunes whatever has aged
// out since the last run.
type authCleanupJobArgs struct{}

// Kind implements river.JobArgs.
func (authCleanupJobArgs) Kind() string { return "AuthCleanup" }

// InsertOpts implements river.JobArgsWithInsertOpts interface.
func (a authCleanupJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ //nolint:exhaustruct
		// Every job kind runs in its own queue named after the kind.
		Queue: base.QueueName(a.Kind()),
	}
}

// authCleanupWorker prunes expired rows from the per-site auth flow
// and revocation stores. The work is delegated to
// Authenticator.CleanupExpired so OIDC and Mock authenticators share
// a single cleanup contract.
type authCleanupWorker struct {
	river.WorkerDefaults[authCleanupJobArgs]

	Site *internalSite.Site
}

// Work implements river.Worker.
func (w *authCleanupWorker) Work(ctx context.Context, _ *river.Job[authCleanupJobArgs]) error {
	return w.Site.Authenticator.CleanupExpired(ctx)
}
