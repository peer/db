package peerdb

import (
	"context"

	"github.com/riverqueue/river"
)

// authCleanupJobArgs are the arguments for the periodic auth cleanup
// job. No payload. Each invocation simply prunes whatever has aged
// out since the last run.
type authCleanupJobArgs struct{}

// Kind implements river.JobArgs.
func (authCleanupJobArgs) Kind() string { return "AuthCleanup" }

// authCleanupWorker prunes expired rows from the per-site auth flow
// and revocation stores. The work is delegated to
// Authenticator.CleanupExpired so OIDC and Mock authenticators share
// a single cleanup contract.
type authCleanupWorker struct {
	river.WorkerDefaults[authCleanupJobArgs]

	Site *Site
}

// Work implements river.Worker.
func (w *authCleanupWorker) Work(ctx context.Context, _ *river.Job[authCleanupJobArgs]) error {
	return w.Site.authenticator.CleanupExpired(ctx)
}
