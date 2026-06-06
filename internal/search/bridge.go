// Package search provides ElasticSearch integration functionality for PeerDB.
package search

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operationtype"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/indexer"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

const bridgeRetryDelay = 5 * time.Second

// reindexSoftDeadline bounds how long a single reindex job spends draining the queue before it flushes
// what it has and schedules a follow-up job.
const reindexSoftDeadline = 10 * time.Minute

// reindexJobTimeoutSlack is the extra time, on top of reindexSoftDeadline, that the reindex job has to
// finish its final flush and schedule its follow-up before River cancels it for exceeding its timeout.
const reindexJobTimeoutSlack = 5 * time.Minute

// reindexMaxBatch is the maximum number of documents accumulated into a single ElasticSearch bulk request
// while draining the reindex queue.
const reindexMaxBatch = 1000

var errCommittedChannelClosed = errors.Base("committed channel is closed")

type bulkError struct {
	ID         string            `json:"id,omitempty"`
	Status     int               `json:"status,omitempty"`
	ErrorCause *types.ErrorCause `json:"errorCause,omitempty"`
	Doc        *Document         `json:"doc,omitempty"`
}

type bridgeJob interface {
	runReindexQueue(ctx context.Context, job *river.Job[jobArgs]) errors.E
}

//nolint:gochecknoglobals
var (
	// Map from schema to map from prefix to bridgeJob.
	bridges   = map[string]map[string]bridgeJob{}
	bridgesMu = sync.RWMutex{}
)

type jobArgs struct {
	Schema string `json:"schema"`
	Prefix string `json:"prefix"`
}

// Kind implements river.JobArgs interface.
func (jobArgs) Kind() string {
	return "BridgeReindex"
}

// InsertOpts implements river.JobArgsWithInsertOpts interface.
func (jobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ //nolint:exhaustruct
		// We use a single worker queue for the bridge so that its jobs are run sequentially.
		// This prevents duplicate work where multiple parallel jobs would pick the same document
		// ID to work on.
		//
		// We do not use UniqueOpts because River requires JobStateRunning in ByState,
		// which causes inserts to be silently deduplicated while a job is running.
		// This creates a race where new BridgeReindexQueue entries added during
		// job execution are never processed. Instead we currently allow multiple jobs
		// for correctness even if it means that some jobs will not do anything.
		// See: https://github.com/riverqueue/river/issues/1178
		//
		// Downside of this approach is that if there are multiple bridges (with different schema/prefix
		// combinations) their own jobs are not run in parallel but still only one at a time.
		//
		// TODO: Should we instead of our work queue table BridgeReindexQueue submit one job for each set of updates in updateSeq?
		//       So instead of having our own table we would maintain what has to be done in job arguments.
		//       We could still use single worker queue to work on those jobs one at a time.
		//       In Bridge table we would then maintain two rows, how far committed changesets have been
		//       processed (a seq number) and how far the reindex queue has been processed (also a seq number).
		Queue: "bridge",
	}
}

type worker struct {
	river.WorkerDefaults[jobArgs]
}

// Timeout implements river.Worker interface. The reindex job drains the queue until reindexSoftDeadline,
// so it needs a longer timeout than the client default: that deadline plus slack for the final flush and
// follow-up scheduling. Setting it here scopes the longer timeout to the reindex job only, not to all jobs.
func (w *worker) Timeout(*river.Job[jobArgs]) time.Duration {
	return reindexSoftDeadline + reindexJobTimeoutSlack
}

// Work implements river.Worker interface.
func (w *worker) Work(ctx context.Context, job *river.Job[jobArgs]) error {
	ctx = internalStore.WithFallbackDBContext(ctx, job.Args.Schema, "bridge")

	c, errE := w.getBridge(job.Args.Schema, job.Args.Prefix)
	if errE != nil {
		return errE
	}

	errE = c.runReindexQueue(ctx, job)
	if errE != nil {
		// We do not wrap any error into JobCancel because for all errors we want the job to be retried.
		// Job can safely be rerun multiple times because it keeps track of successful work in its table.
		// So it could partially succeed and then fail and the next time it will continue where it left off.
		return errE
	}

	return nil
}

func (w *worker) getBridge(schema, prefix string) (bridgeJob, errors.E) { //nolint:ireturn
	bridgesMu.RLock()
	defer bridgesMu.RUnlock()

	s, ok := bridges[schema]
	if !ok {
		errE := errors.New("bridge not found")
		details := errors.Details(errE)
		details["schema"] = schema
		details["prefix"] = prefix
		return nil, errE
	}

	c, ok := s[prefix]
	if !ok {
		errE := errors.New("bridge not found")
		details := errors.Details(errE)
		details["schema"] = schema
		details["prefix"] = prefix
		return nil, errE
	}

	return c, nil
}

// Bridge synchronizes changes from the store to ElasticSearch.
//
// It saves progress in a PostgreSQL table so it resumes from where it left off on restart.
type Bridge struct {
	// Store is the store to read documents from.
	Store *store.Store[
		json.RawMessage, *store.DocumentMetadata,
		*store.NoMetadata, *store.NoMetadata, *store.CommitMetadata,
		document.Changes,
	]

	// ESClient is the ElasticSearch client.
	ESClient *elasticsearch.TypedClient

	// Index is the ElasticSearch index name.
	Index string

	dbpool                 *pgxpool.Pool
	schema                 string
	riverClient            *river.Client[pgx.Tx]
	converter              *Converter
	lastSeqMu              sync.RWMutex
	lastSeqCond            *sync.Cond
	lastSeq                int64
	reindexQueueMinSeqMu   sync.RWMutex
	reindexQueueMinSeqCond *sync.Cond
	// reindexQueueMinSeq is the MIN(seq) of remaining rows in BridgeReindexQueue,
	// or math.MaxInt64 if the table is empty. A waiter for seq X is done when this value > X.
	reindexQueueMinSeq int64
	// reindexQueueCount is the number of distinct document IDs remaining
	// in BridgeReindexQueue. It is used for progress tracking.
	reindexQueueCount int64
	// reindexSoftDeadline bounds how long a single reindex job drains the queue before flushing and
	// scheduling a follow-up. It defaults to the reindexSoftDeadline constant and can be lowered in tests.
	reindexSoftDeadline time.Duration
}

// Init creates the bridge progress table and registers a NOTIFY handler on the shared listener
// so that WaitUntilCaughtUp is notified immediately when the bridge seq advances.
func (b *Bridge) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internalStore.Listener, schema string,
	riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if b.dbpool != nil {
		return errors.New("already initialized")
	}
	b.dbpool = dbpool
	b.schema = schema
	b.riverClient = riverClient

	errE := internalStore.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			-- "Bridge" table tracks the last commit seq successfully indexed to ElasticSearch.
			CREATE TABLE "`+b.Store.Prefix+`Bridge" (
				-- Seq of the last commit fully indexed. 0 means nothing has been indexed yet.
				"seq" bigint NOT NULL DEFAULT 0
			);
			INSERT INTO "`+b.Store.Prefix+`Bridge" DEFAULT VALUES;
			CREATE FUNCTION "`+b.Store.Prefix+`BridgeAfterUpdateFunc"()
				RETURNS TRIGGER LANGUAGE plpgsql AS $$
				BEGIN
					PERFORM pg_notify('`+b.Store.Prefix+`BridgeSeq', NEW."seq"::text);
					RETURN NULL;
				END;
			$$;
			CREATE TRIGGER "`+b.Store.Prefix+`BridgeAfterUpdate" AFTER UPDATE ON "`+b.Store.Prefix+`Bridge"
				FOR EACH ROW EXECUTE FUNCTION "`+b.Store.Prefix+`BridgeAfterUpdateFunc"();

			-- "BridgeReindexQueue" holds document IDs that need to be re-indexed,
			-- for any reason. It acts as a work queue. The "seq" column records
			-- which commit enqueued the document, allowing detection of new table
			-- entries added during job processing.
			CREATE TABLE "`+b.Store.Prefix+`BridgeReindexQueue" (
				"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"seq" bigint NOT NULL,
				PRIMARY KEY ("id", "seq")
			);
			-- This allows efficient MIN(seq) queries.
			CREATE INDEX "`+b.Store.Prefix+`BridgeReindexQueueSeq" ON "`+b.Store.Prefix+`BridgeReindexQueue" ("seq");
			CREATE FUNCTION "`+b.Store.Prefix+`BridgeReindexQueueAfterChangeFunc"()
				RETURNS TRIGGER LANGUAGE plpgsql AS $$
				BEGIN
					-- Notify without payload. The handler queries MIN(seq) in a separate read-only transaction to avoid serialization conflicts.
					-- Computing MIN(seq) inside this read-write trigger would create an unnecessary dependency on the table, conflicting with
					-- concurrent INSERTs and DELETEs under serializable isolation, but it is not really necessary to know the MIN(seq) from
					-- inside the transaction because the handler can only obtain >= MIN(seq) through a later query.
					PERFORM pg_notify('`+b.Store.Prefix+`BridgeReindexQueueMinSeq', '');
					RETURN NULL;
				END;
			$$;
			CREATE TRIGGER "`+b.Store.Prefix+`BridgeReindexQueueAfterChange" AFTER INSERT OR DELETE ON "`+b.Store.Prefix+`BridgeReindexQueue"
				FOR EACH STATEMENT EXECUTE FUNCTION "`+b.Store.Prefix+`BridgeReindexQueueAfterChangeFunc"();
			CREATE TRIGGER "`+b.Store.Prefix+`BridgeReindexQueueNotAllowed" BEFORE UPDATE OR TRUNCATE ON "`+b.Store.Prefix+`BridgeReindexQueue"
				FOR EACH STATEMENT EXECUTE FUNCTION "`+b.Store.Prefix+`DoNotAllow"();
		`)
		return internalStore.WithPgxError(err)
	})
	if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
		switch pgError.Code {
		case pgerrcode.DuplicateTable:
			// Nothing.
		case pgerrcode.DuplicateFunction:
			// Nothing.
		default:
			return errE
		}
	} else if errE != nil {
		return errE
	}

	errE = b.registerCoordinator(workers)
	if errE != nil {
		return errE
	}

	b.lastSeqCond = sync.NewCond(b.lastSeqMu.RLocker())
	b.reindexQueueMinSeqCond = sync.NewCond(b.reindexQueueMinSeqMu.RLocker())
	b.reindexQueueMinSeq = math.MaxInt64
	b.reindexSoftDeadline = reindexSoftDeadline
	listener.Handle(b.Store.Prefix+"BridgeSeq", b)
	listener.Handle(b.Store.Prefix+"BridgeReindexQueueMinSeq", b)

	return nil
}

func (b *Bridge) registerCoordinator(workers *river.Workers) errors.E {
	bridgesMu.Lock()
	defer bridgesMu.Unlock()

	s, ok := bridges[b.schema]
	if ok {
		_, ok := s[b.Store.Prefix]
		if ok {
			errE := errors.New("bridge already registered")
			details := errors.Details(errE)
			details["schema"] = b.schema
			details["prefix"] = b.Store.Prefix
			return errE
		}
	} else {
		s = map[string]bridgeJob{}
		bridges[b.schema] = s

		// We register the worker if this is the first coordinator for this schema.
		err := river.AddWorkerSafely(workers, &worker{})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	s[b.Store.Prefix] = b

	return nil
}

// HandleNotification implements pgxlisten.Handler interface.
func (b *Bridge) HandleNotification(
	ctx context.Context, notification *pgconn.Notification, conn *pgx.Conn,
) error {
	switch notification.Channel {
	case b.Store.Prefix + "BridgeSeq":
		return b.handleBridgeSeq(ctx, notification, conn)
	case b.Store.Prefix + "BridgeReindexQueueMinSeq":
		return b.handleBridgeReindexQueueMinSeq(ctx)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = notification.Channel
		return errE
	}
}

// HandleBacklog implements pgxlisten.BacklogHandler interface.
//
// It fetches the last seq from the Bridge table to get the current state if anything was missed.
func (b *Bridge) HandleBacklog(
	ctx context.Context, channel string, _ *pgx.Conn,
) error {
	switch channel {
	case b.Store.Prefix + "BridgeSeq":
		// TODO: Improve what happens on an error.
		//       Any error from fixBridgeSeq is just logged. Which means that goroutines waiting in WaitUntilCaughtUp
		//       might continue waiting until some other new commit is made, which might be never.
		_, errE := b.fixBridgeSeq(ctx)
		return errE
	case b.Store.Prefix + "BridgeReindexQueueMinSeq":
		// TODO: Improve what happens on an error.
		//       Any error from updateBridgeReindexQueueMinSeq is just logged. Which means that goroutines waiting
		//       in WaitUntilCaughtUp might continue waiting until some other new commit is made, which might be never.
		return b.updateBridgeReindexQueueMinSeq(ctx)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
}

// HandlingReady implements internalStore.Handler interface.
func (b *Bridge) HandlingReady(ctx context.Context, channel string) errors.E {
	switch channel {
	case b.Store.Prefix + "BridgeSeq":
		return b.waitForFixBridgeSeq(ctx)
	case b.Store.Prefix + "BridgeReindexQueueMinSeq":
		return b.waitForUpdateBridgeReindexQueueMinSeq(ctx)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
}

// handleBridgeSeq handles BridgeSeq notifications from the Bridge table trigger and
// broadcasts to any goroutines waiting in WaitUntilCaughtUp.
func (b *Bridge) handleBridgeSeq(_ context.Context, notification *pgconn.Notification, _ *pgx.Conn) errors.E {
	seq, err := strconv.ParseInt(notification.Payload, 10, 64)
	if err != nil {
		errE := errors.WithMessage(err, "failed to parse bridge seq notification payload")
		errors.Details(errE)["payload"] = notification.Payload
		return errE
	}
	b.lastSeqMu.Lock()
	defer b.lastSeqMu.Unlock()
	if seq > b.lastSeq {
		b.lastSeq = seq
		b.lastSeqCond.Broadcast()
	}
	return nil
}

// fixBridgeSeq fetches the last seq from the Bridge table, updates the in-memory state,
// broadcasts to any goroutines waiting in WaitUntilCaughtUp, and returns the bridge seq.
func (b *Bridge) fixBridgeSeq(ctx context.Context) (int64, errors.E) {
	seq, errE := b.getSeq(ctx)
	if errE != nil {
		return 0, errE
	}
	b.lastSeqMu.Lock()
	defer b.lastSeqMu.Unlock()
	if seq > b.lastSeq {
		b.lastSeq = seq
		b.lastSeqCond.Broadcast()
	}
	return seq, nil
}

// handleBridgeReindexQueueMinSeq handles notifications from the BridgeReindexQueue table
// trigger and broadcasts to any goroutines waiting in WaitUntilCaughtUp.
//
// We query MIN(seq) in a separate read-only transaction via updateBridgeReindexQueueMinSeq
// rather than receiving it as the notification payload. Computing MIN(seq) inside the trigger's
// read-write transaction would create an unnecessary dependency on the BridgeReindexQueue table,
// causing serialization conflicts with concurrent INSERTs and DELETEs under serializable isolation.
// A read-only transaction does not take conflicting predicate locks, avoiding this issue. This is
// safe because seq values only increase (new INSERTs always have higher seq) and DELETEs only
// remove rows that have already been processed, so the MIN(seq) observed by the handler is always
// a correct (or conservatively low) value.
func (b *Bridge) handleBridgeReindexQueueMinSeq(ctx context.Context) errors.E {
	return b.updateBridgeReindexQueueMinSeq(ctx)
}

// updateBridgeReindexQueueMinSeq fetches the current MIN(seq) and COUNT(DISTINCT "id")
// from BridgeReindexQueue, updates the in-memory state, and broadcasts to any goroutines
// waiting in WaitUntilCaughtUp.
func (b *Bridge) updateBridgeReindexQueueMinSeq(ctx context.Context) errors.E {
	var minSeq *int64
	var cnt int64
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internalStore.WithPgxError(
			tx.QueryRow(ctx, `SELECT MIN("seq"), COUNT(DISTINCT "id") FROM "`+b.Store.Prefix+`BridgeReindexQueue"`).Scan(&minSeq, &cnt),
		)
	})
	if errE != nil {
		return errE
	}
	b.reindexQueueMinSeqMu.Lock()
	defer b.reindexQueueMinSeqMu.Unlock()
	if minSeq == nil {
		b.reindexQueueMinSeq = math.MaxInt64
	} else {
		b.reindexQueueMinSeq = *minSeq
	}
	b.reindexQueueCount = cnt
	b.reindexQueueMinSeqCond.Broadcast()
	return nil
}

// waitForFixBridgeSeq is similar to WaitUntilCaughtUp but it does not wait for b.lastSeq to catch up with
// committed commits, but just that it catches up with the current last-indexed seq from the bridge table.
func (b *Bridge) waitForFixBridgeSeq(ctx context.Context) errors.E {
	// We must call fixBridgeSeq here because HandleBacklog runs in a separate goroutine and may not have
	// executed yet.
	seq, errE := b.fixBridgeSeq(ctx)
	if errE != nil {
		return errE
	}

	return b.waitForLastSeq(ctx, seq, nil, nil)
}

func (b *Bridge) waitForLastSeq(ctx context.Context, seq int64, count, size *x.Counter) errors.E {
	b.lastSeqCond.L.Lock()
	defer b.lastSeqCond.L.Unlock()

	// Nothing to do.
	if b.lastSeq >= seq {
		return nil
	}

	// This is based on example for context.AfterFunc from the context package.
	// See comments there for explanation how it works and why.
	stop := context.AfterFunc(ctx, func() {
		b.lastSeqCond.L.Lock()
		defer b.lastSeqCond.L.Unlock()
		b.lastSeqCond.Broadcast()
	})
	defer stop()

	prevSeq := b.lastSeq

	if size != nil {
		size.Add(seq - prevSeq)
	}

	for b.lastSeq < seq {
		b.lastSeqCond.Wait()
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		if count != nil && b.lastSeq > prevSeq {
			// Just in case b.lastSeq jumps more than seq.
			current := min(b.lastSeq, seq)
			count.Add(current - prevSeq)
			prevSeq = current
		}
	}

	// To get count to match the increase we made to size initially.
	if count != nil && seq > prevSeq {
		count.Add(seq - prevSeq)
	}

	return nil
}

// waitForUpdateBridgeReindexQueueMinSeq is similar to WaitUntilCaughtUp but it does not wait for
// b.reindexQueueMinSeq to catch up with committed commits, but just that it catches up with
// the current last-indexed seq from the bridge table. A startup job submitted in Init ensures
// any leftover rows will be processed.
func (b *Bridge) waitForUpdateBridgeReindexQueueMinSeq(ctx context.Context) errors.E {
	// We must call updateBridgeReindexQueueMinSeq here because HandleBacklog runs in a separate
	// goroutine and may not have executed yet.
	errE := b.updateBridgeReindexQueueMinSeq(ctx)
	if errE != nil {
		return errE
	}

	seq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
	}

	return b.waitForReindexQueueMinSeq(ctx, seq, nil, nil)
}

func (b *Bridge) waitForReindexQueueMinSeq(ctx context.Context, seq int64, count, size *x.Counter) errors.E {
	b.reindexQueueMinSeqCond.L.Lock()
	defer b.reindexQueueMinSeqCond.L.Unlock()

	// Nothing to do.
	if b.reindexQueueMinSeq > seq {
		return nil
	}

	// This is based on example for context.AfterFunc from the context package.
	// See comments there for explanation how it works and why.
	stop := context.AfterFunc(ctx, func() {
		b.reindexQueueMinSeqCond.L.Lock()
		defer b.reindexQueueMinSeqCond.L.Unlock()
		b.reindexQueueMinSeqCond.Broadcast()
	})
	defer stop()

	// We use the number of distinct document IDs for progress tracking instead of seq values.
	// This provides regular progress updates because the count decreases with each processed
	// document, while MIN(seq) can stay the same when many documents share the same seq.
	initialCount := b.reindexQueueCount

	if size != nil {
		size.Add(initialCount)
	}

	// reindexQueueMinSeq tracks the MIN(seq) of remaining rows in BridgeReindexQueue.
	// When it exceeds seq (or the table is empty, represented as MaxInt64), we are done.
	for b.reindexQueueMinSeq <= seq {
		b.reindexQueueMinSeqCond.Wait()
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		if count != nil {
			processed := initialCount - b.reindexQueueCount
			if processed > 0 {
				count.Add(processed)
				initialCount = b.reindexQueueCount
			}
		}
	}

	// To get count to match the increase we made to size initially.
	if count != nil && initialCount > 0 {
		count.Add(initialCount)
	}

	return nil
}

// ResetSeq resets the bridge progress to 0 and clears the reindex queue.
// This causes the bridge to re-process all commits from the beginning when started.
func (b *Bridge) ResetSeq(ctx context.Context) errors.E {
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `UPDATE "`+b.Store.Prefix+`Bridge" SET "seq" = 0`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		_, err = tx.Exec(ctx, `DELETE FROM "`+b.Store.Prefix+`BridgeReindexQueue"`)
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return errE
	}

	b.lastSeqMu.Lock()
	b.lastSeq = 0
	b.lastSeqMu.Unlock()

	b.reindexQueueMinSeqMu.Lock()
	b.reindexQueueMinSeq = math.MaxInt64
	b.reindexQueueCount = 0
	b.reindexQueueMinSeqMu.Unlock()

	// We reset the store's Committed channel so that the bridge goroutine detects the closed
	// channel and restarts its run loop, picking up the reset seq from the database.
	// This impacts only the current process but this is fine because any concurrent process
	// will just wait for this process to reindex everything and then continue from there on.
	b.Store.Reset()

	return nil
}

// Prepare stores the converter and submits a startup job that processes any leftover rows
// in BridgeReindexQueue from a previous run.
//
// It must be called before the river client and the store listener are started. The listener's
// HandlingReady for the reindex queue channel blocks until the reindex queue backlog
// (entries at or below the indexed seq) is drained, and draining is possible only once the bridge
// has the converter and worker can run its jobs.
func (b *Bridge) Prepare(ctx context.Context, converter *Converter) errors.E {
	b.converter = converter

	// Submit a startup job to process any leftover rows in BridgeReindexQueue from a previous run.
	_, err := b.riverClient.Insert(ctx, jobArgs{
		Schema: b.schema,
		Prefix: b.Store.Prefix,
	}, nil)
	return errors.WithStack(err)
}

// Start begins the bridging goroutine.
//
// It first indexes any commits from CommitLog that are newer than what is recorded in the bridge
// table (catch-up), then processes new commits from the Committed channel as they arrive.
//
// Prepare must have been called before Start to store the converter used to convert
// documents for indexing and to track inverse relations.
//
// The store listener should be listening to notifications from PostgreSQL and sending them to
// the Committed channel before calling Start to assure that there is no gap between catch-up and
// real-time processing of new commits.
func (b *Bridge) Start(ctx context.Context) errors.E {
	go func() {
		// TODO: Measure how many retries have to be made and abort if it is too much.
		//       The goal is that if this is happening too often, we should terminate the whole process and let the
		//       process supervisor decide what to do about instability (it is probably not a local thing).
		for {
			errE := b.run(ctx)
			if errors.Is(errE, context.Canceled) || errors.Is(errE, context.DeadlineExceeded) {
				// No need to retry. We are stopping.
				return
			} else if errors.Is(errE, errCommittedChannelClosed) {
				// Channel was closed which means that notifications about commits made might have been
				// missed and we should take corrective actions. We just rerun and our existing catch-up
				// logic will do the rest.
				continue
			}
			// There should always be an error.
			zerolog.Ctx(ctx).Error().Err(errE).Msg("bridge error")

			select {
			case <-ctx.Done():
				// We are stopping.
				return
			case <-time.After(bridgeRetryDelay):
				// We wait a little before retrying.
			}
		}
	}()

	return nil
}

// WaitUntilCaughtUp blocks until the bridge has indexed all currently committed commits.
//
// It is useful for waiting after a bulk import before querying ElasticSearch.
//
// Optional count and size counters can be provided to track ES indexing progress.
// If provided, size is increased for the number of commits to process, and count is
// incremented as commits are indexed.
func (b *Bridge) WaitUntilCaughtUp(ctx context.Context, count, size *x.Counter) errors.E {
	// Find the current maximum seq in CommitLog.
	var maxSeq int64
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT COALESCE(MAX("seq"), 0) FROM "`+b.Store.Prefix+`CommitLog"`).Scan(&maxSeq)
		return internalStore.WithPgxError(err)
	})
	if errE != nil {
		return errE
	}

	if maxSeq == 0 {
		return nil
	}

	// We first wait on lastSeq (commit indexing phase).
	errE = b.waitForLastSeq(ctx, maxSeq, count, size)
	if errE != nil {
		return errE
	}

	// And then we wait on reindexQueueMinSeq (reindex queue phase).
	return b.waitForReindexQueueMinSeq(ctx, maxSeq, count, size)
}

func (b *Bridge) run(ctx context.Context) errors.E {
	// We acquire the Committed channel before reading the bridge seq from the database so that
	// any concurrent Store.Reset (e.g., from ResetSeq during a reindex) closes the channel we are
	// holding here. That way the real-time select loop below detects the closure and run returns
	// errCommittedChannelClosed, causing the outer loop to restart run and re-read getSeq.
	// Otherwise, if we read getSeq first and Store.Reset ran between catch-up and Committed.Get,
	// we would acquire the freshly recreated channel and miss the reset signal entirely.
	ch, errE := b.Store.Committed.Get(ctx)
	if errE != nil {
		return errE
	}

	// Determine where we left off.
	lastSeq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
	}

	logger := zerolog.Ctx(ctx)
	catchUpStart := time.Now()
	catchUpStartSeq := lastSeq
	catchUpCommits := 0

	// Catch-up: index any commits in CommitLog newer than lastSeq.
	for {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		commits, errE := b.Store.CommitLog(ctx, &lastSeq, nil)
		if errE != nil {
			return errE
		}
		for _, commit := range commits {
			addedInverseRelations, removedInverseRelations, referenceTargets, errE := b.indexCommit(ctx, commit)
			if errE != nil {
				return errE
			}
			errE = b.updateSeq(ctx, commit.Seq, addedInverseRelations, removedInverseRelations, referenceTargets)
			if errE != nil {
				return errE
			}
			lastSeq = commit.Seq
			catchUpCommits++
		}
		if len(commits) < store.MaxPageLength {
			break
		}
	}

	// Catch-up phase covers commits already in CommitLog at startup. Logging it shows how long the
	// initial backlog took and how many commits it spanned.
	logger.Debug().
		Int64("fromSeq", catchUpStartSeq).
		Int64("toSeq", lastSeq).
		Int("commits", catchUpCommits).
		Dur("duration", time.Since(catchUpStart)).
		Msg("bridge catch-up complete")

	// Real-time: process new commits from the channel.
	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case c, ok := <-ch:
			if !ok {
				// Channel was closed which means that notifications about commits made might have been
				// missed and we should take corrective actions. We return the sentinel error.
				return errors.WithStack(errCommittedChannelClosed)
			}
			// Skip commits already processed during catch-up.
			if c.Seq <= lastSeq {
				continue
			}
			addedInverseRelations, removedInverseRelations, referenceTargets, errE := b.indexCommit(ctx, c)
			if errE != nil {
				return errE
			}
			// The bridge table is only advanced after indexing returned no error.
			errE = b.updateSeq(ctx, c.Seq, addedInverseRelations, removedInverseRelations, referenceTargets)
			if errE != nil {
				return errE
			}
			lastSeq = c.Seq
		}
	}
}

// withCommitDetails annotates errE with the commit, changeset, and (when non-empty) document that an
// error in indexCommit occurred in.
func withCommitDetails(errE errors.E, seq int64, view, changeset, doc string) errors.E {
	details := errors.Details(errE)
	details["seq"] = seq
	details["view"] = view
	details["changeset"] = changeset
	if doc != "" {
		details["doc"] = doc
	}
	return errE
}

// TODO: We should batch multiple commits together if they are small and split them if they are large.
//       indexCommit operates on a single commit but those could be very small or very large.
//       Maybe a batch should be made when we reach 1000 documents or if more than 1 second has
//       passed since the batch was started (so that we index with at most 1 second delay).

// indexCommit collects all document changes from the commit, fetches the latest version
// of each document, and indexes them to ElasticSearch as a single bulk request.
//
// Documents are converted for indexing and inverse relations are collected.
// The first returned map contains, for each target document ID, the inverse relations that
// should be stored in that document's metadata. The second returned map contains
// inverse relations that should be removed from the document's metadata.
func (b *Bridge) indexCommit(
	ctx context.Context,
	committed store.CommittedChangesets[
		json.RawMessage, *store.DocumentMetadata, *store.NoMetadata, *store.NoMetadata, *store.CommitMetadata, document.Changes,
	],
) (map[identifier.Identifier][]store.InverseRelation, map[identifier.Identifier][]store.InverseRelation, map[identifier.Identifier]bool, errors.E) {
	logger := zerolog.Ctx(ctx)
	start := time.Now()
	var stats ConversionStats
	ctx = WithConversionStats(ctx, &stats)

	// Reconstruct changesets with the store so we can query them.
	withStoreStart := time.Now()
	c, errE := committed.WithStore(ctx, b.Store)
	if errE != nil {
		errors.Details(errE)["seq"] = committed.Seq
		errors.Details(errE)["view"] = committed.View.Name()
		return nil, nil, nil, errE
	}

	indexOps := 0
	deleteOps := 0
	// Per-phase durations, all disjoint, accumulated across the commit's documents. changesDuration
	// starts with reconstructing the changesets here and adds reading their pages in the loop.
	// accumulateDuration and convertDuration exclude their getDocument store fetches (those are
	// fetchDuration), so the phases do not overlap.
	changesDuration := time.Since(withStoreStart)
	var getDuration, accumulateDuration, convertDuration time.Duration
	bulkService := b.ESClient.Bulk()

	// Collect inverse relations from all processed documents.
	addedInverseRelations := map[identifier.Identifier][]store.InverseRelation{}
	removedInverseRelations := map[identifier.Identifier][]store.InverseRelation{}

	// Collect documents whose counts.references must be refreshed because a processed
	// document started or stopped referencing them.
	referenceTargets := map[identifier.Identifier]bool{}

	debugDocs := map[string]*Document{}

	for _, cs := range c.Changesets {
		var after *identifier.Identifier
		for {
			changesStart := time.Now()
			page, errE := cs.Changes(ctx, after)
			changesDuration += time.Since(changesStart)
			if errE != nil {
				return nil, nil, nil, withCommitDetails(errE, committed.Seq, committed.View.Name(), cs.String(), "")
			}
			for _, change := range page {
				// The document changed in this commit, so drop any cached info and fetched content for it.
				b.converter.InvalidateCaches(change.ID)

				// Fetch document at the change version.
				deleted := false
				getStart := time.Now()
				data, metadata, _, parentChangesets, errE := b.Store.Get(ctx, change.ID, change.Version)
				getDuration += time.Since(getStart)
				if errors.Is(errE, store.ErrValueDeleted) {
					// Deleted at this version: no outgoing relations or reference targets.
					deleted = true
				} else if errE != nil {
					return nil, nil, nil, withCommitDetails(errE, committed.Seq, committed.View.Name(), cs.String(), change.ID.String())
				}

				// Collect, for other documents, the inverse-relation and counts.references
				// changes implied by this document's change.
				accumulateFetchBefore := stats.FetchDuration
				accumulateStart := time.Now()
				errE = b.accumulateChangeRelations(
					ctx, change.ID, deleted, data, parentChangesets,
					addedInverseRelations, removedInverseRelations, referenceTargets,
				)
				accumulateDuration += time.Since(accumulateStart) - (stats.FetchDuration - accumulateFetchBefore)
				if errE != nil {
					return nil, nil, nil, withCommitDetails(errE, committed.Seq, committed.View.Name(), cs.String(), change.ID.String())
				}

				if deleted {
					id := change.ID.String()
					err := bulkService.DeleteOp(types.DeleteOperation{Index_: &b.Index, Id_: &id}) //nolint:exhaustruct
					if err != nil {
						return nil, nil, nil, errors.WithStack(err)
					}
					deleteOps++
				} else {
					// TODO: Use also information about the view so that documents are searchable by view as well.
					convertFetchBefore := stats.FetchDuration
					convertStart := time.Now()
					searchDoc, errE := b.ConvertDocument(ctx, data, metadata)
					convertDuration += time.Since(convertStart) - (stats.FetchDuration - convertFetchBefore)
					if errE != nil {
						return nil, nil, nil, withCommitDetails(errE, committed.Seq, committed.View.Name(), cs.String(), change.ID.String())
					}
					id := change.ID.String()
					err := bulkService.IndexOp(types.IndexOperation{Index_: &b.Index, Id_: &id}, searchDoc) //nolint:exhaustruct
					if err != nil {
						return nil, nil, nil, errors.WithStack(err)
					}
					debugDocs[id] = searchDoc
					indexOps++
				}
			}
			if len(page) < store.MaxPageLength {
				break
			}
			after = &page[store.MaxPageLength-1].ID
		}
	}

	if indexOps+deleteOps == 0 {
		logger.Debug().
			Int64("seq", committed.Seq).
			Int("indexed", 0).
			Int("deleted", 0).
			Dur("duration", time.Since(start)).
			Msg("bridge indexed commit")
		return nil, nil, nil, nil
	}

	bulkStart := time.Now()
	response, err := bulkService.Do(ctx)
	if err != nil {
		return nil, nil, nil, WithESError(err)
	}
	bulkDuration := time.Since(bulkStart)

	bulkErrors := []bulkError{}
	for _, item := range response.Items {
		for action, result := range item {
			if result.Status >= 200 && result.Status <= 299 {
				continue
			}
			// Deleting a document that does not exist in ES is not an error.
			// This can happen when indexCommit is retried after the bulk request
			// succeeded but updateSeq failed.
			if action == operationtype.Delete && result.Status == 404 {
				continue
			}
			id := ""
			if result.Id_ != nil {
				id = *result.Id_
			}
			bulkErrors = append(bulkErrors, bulkError{
				ID:         id,
				Status:     result.Status,
				ErrorCause: result.Error,
				Doc:        debugDocs[id],
			})
		}
	}
	if len(bulkErrors) > 0 {
		errE := errors.New("bulk indexing had failures")
		errors.Details(errE)["seq"] = committed.Seq
		errors.Details(errE)["view"] = committed.View.Name()
		// We do not name this field "errors" to not confuse go-errors package which tries to parse it as joined errors.
		errors.Details(errE)["esErrors"] = bulkErrors
		return nil, nil, nil, errE
	}

	// The counts here are the work this commit implies for other documents. indexed/deleted are the
	// bulk operations for the changed documents themselves. inverseAdded/inverseRemoved are the
	// numbers of target documents whose inverse-relation metadata changes, and referenceTargets is
	// the number of documents whose counts.references must be refreshed. The durations are disjoint and
	// sum to duration (minus small in-memory overhead for cache invalidation, bulk buffering, and the
	// bulk error scan): changesDuration is reconstructing and reading the committed changesets,
	// getDuration is the per-document store reads, fetchDuration is the getDocument store fetches,
	// accumulateDuration and convertDuration are accumulateChangeRelations and ConvertDocument
	// excluding those fetches, and bulkDuration is the ES bulk request.
	logger.Debug().
		Int64("seq", committed.Seq).
		Int("indexed", indexOps).
		Int("deleted", deleteOps).
		Int("inverseAdded", len(addedInverseRelations)).
		Int("inverseRemoved", len(removedInverseRelations)).
		Int("referenceTargets", len(referenceTargets)).
		Int("docCacheHits", stats.DocCacheHits).
		Int("docCacheMisses", stats.DocCacheMisses).
		Int("infoCacheHits", stats.InfoCacheHits).
		Int("infoCacheMisses", stats.InfoCacheMisses).
		Dur("changesDuration", changesDuration).
		Dur("getDuration", getDuration).
		Dur("fetchDuration", stats.FetchDuration).
		Dur("accumulateDuration", accumulateDuration).
		Dur("convertDuration", convertDuration).
		Dur("bulkDuration", bulkDuration).
		Dur("duration", time.Since(start)).
		Msg("bridge indexed commit")

	return addedInverseRelations, removedInverseRelations, referenceTargets, nil
}

// diffOutgoingInverseRelations compares current and parent outgoing inverse relations,
// returning added and removed maps. A relation is considered changed (and thus both
// removed and added) if any of its fields (target, property, confidence) differ,
// even if the claim ID stays the same.
func diffOutgoingInverseRelations(
	current, parent map[identifier.Identifier][]store.InverseRelation,
) (map[identifier.Identifier][]store.InverseRelation, map[identifier.Identifier][]store.InverseRelation) {
	currentSet := map[store.InverseRelation]bool{}
	for _, irs := range current {
		for _, ir := range irs {
			currentSet[ir] = true
		}
	}

	parentSet := map[store.InverseRelation]bool{}
	for _, irs := range parent {
		for _, ir := range irs {
			parentSet[ir] = true
		}
	}

	added := map[identifier.Identifier][]store.InverseRelation{}
	for targetID, irs := range current {
		for _, ir := range irs {
			if !parentSet[ir] {
				added[targetID] = append(added[targetID], ir)
			}
		}
	}

	removed := map[identifier.Identifier][]store.InverseRelation{}
	for targetID, irs := range parent {
		for _, ir := range irs {
			if !currentSet[ir] {
				removed[targetID] = append(removed[targetID], ir)
			}
		}
	}

	return added, removed
}

// ConvertDocument unmarshals data into a document.D and calls the converter's
// FromDocument with inverse relations from metadata.
func (b *Bridge) ConvertDocument(ctx context.Context, data json.RawMessage, metadata *store.DocumentMetadata) (*Document, errors.E) {
	var doc document.D
	errE := x.UnmarshalWithoutUnknownFields(data, &doc)
	if errE != nil {
		return nil, errE
	}

	return b.converter.FromDocument(ctx, &doc, metadata.InverseRelations)
}

// CountReferences returns how many documents reference the document with the
// given ID via a top-level ref claim or a sub-ref claim. It runs an ElasticSearch
// count against the current index, so it reflects whatever is indexed at call
// time.
func (b *Bridge) CountReferences(ctx context.Context, id identifier.Identifier) (int, errors.E) {
	query := esdsl.NewBoolQuery().Should(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(id.String())),
		).Path("claims.ref"),
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.subRef.to", esdsl.NewFieldValue().String(id.String())),
		).Path("claims.subRef"),
	).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))

	res, err := b.ESClient.Count().Index(b.Index).Query(query).Do(ctx)
	if err != nil {
		errE := WithESError(err)
		errors.Details(errE)["id"] = id.String()
		return 0, errE
	}
	// The count endpoint has no allow_partial_search_results flag, so a shard failure
	// would silently undercount. Treat any failed shard as an error so the caller retries
	// rather than recording a too-low counts.references.
	if res.Shards_.Failed > 0 {
		errE := errors.New("references count had shard failures")
		errors.Details(errE)["id"] = id.String()
		errors.Details(errE)["failed"] = res.Shards_.Failed
		errors.Details(errE)["total"] = res.Shards_.Total
		errors.Details(errE)["failures"] = res.Shards_.Failures
		return 0, errE
	}
	return int(res.Count), nil
}

// outgoingRelationsAndTargets unmarshals a document and returns both its outgoing
// inverse relations (for inverse-relation metadata) and the set of all documents it
// references (for refreshing those targets' counts.references), from a single parse.
func (b *Bridge) outgoingRelationsAndTargets(
	ctx context.Context, data json.RawMessage,
) (map[identifier.Identifier][]store.InverseRelation, map[identifier.Identifier]bool, errors.E) {
	var doc document.D
	errE := x.UnmarshalWithoutUnknownFields(data, &doc)
	if errE != nil {
		return nil, nil, errE
	}

	c := b.converter
	outgoing, errE := c.OutgoingInverseRelations(ctx, &doc)
	if errE != nil {
		return nil, nil, errE
	}
	return outgoing, c.OutgoingReferenceTargets(&doc), nil
}

// collectChangedReferenceTargets adds to out every document that the changed document
// started or stopped referencing (the symmetric difference of current and parent
// reference targets), skipping targets ignored for counts.references.
func (b *Bridge) collectChangedReferenceTargets(
	ctx context.Context, current, parent, out map[identifier.Identifier]bool,
) errors.E {
	converter := b.converter
	add := func(targetID identifier.Identifier) errors.E {
		if out[targetID] {
			return nil
		}
		ignored, errE := converter.ReferencesCountIgnored(ctx, targetID)
		if errE != nil {
			return errE
		}
		if !ignored {
			out[targetID] = true
		}
		return nil
	}
	for targetID := range current {
		if parent[targetID] {
			continue
		}
		errE := add(targetID)
		if errE != nil {
			return errE
		}
	}
	for targetID := range parent {
		if current[targetID] {
			continue
		}
		errE := add(targetID)
		if errE != nil {
			return errE
		}
	}
	return nil
}

// accumulateChangeRelations computes, for a single document change, the inverse-relation
// and reference-target differences it implies for other documents, and merges them into
// the provided accumulators (addedInverseRelations, removedInverseRelations, referenceTargets).
// data is the document at the change version (unused when deleted); parentChangesets are its parent versions.
func (b *Bridge) accumulateChangeRelations(
	ctx context.Context, changeID identifier.Identifier, deleted bool, data json.RawMessage, parentChangesets []store.Version,
	addedInverseRelations, removedInverseRelations map[identifier.Identifier][]store.InverseRelation,
	referenceTargets map[identifier.Identifier]bool,
) errors.E {
	currentOutgoing := map[identifier.Identifier][]store.InverseRelation{}
	currentRefTargets := map[identifier.Identifier]bool{}
	if !deleted {
		var errE errors.E
		currentOutgoing, currentRefTargets, errE = b.outgoingRelationsAndTargets(ctx, data)
		if errE != nil {
			return errE
		}
	}

	// Aggregate outgoing relations and reference targets across all parent versions.
	parentOutgoing := map[identifier.Identifier][]store.InverseRelation{}
	parentRefTargets := map[identifier.Identifier]bool{}
	for _, pv := range parentChangesets {
		parentData, _, _, _, errE := b.Store.Get(ctx, changeID, pv)
		if errors.Is(errE, store.ErrValueDeleted) {
			// Parent document was deleted, so there were no outgoing relations in it.
			continue
		} else if errE != nil {
			return errE
		}
		po, pt, errE := b.outgoingRelationsAndTargets(ctx, parentData)
		if errE != nil {
			return errE
		}
		for targetID, irs := range po {
			parentOutgoing[targetID] = append(parentOutgoing[targetID], irs...)
		}
		for targetID := range pt {
			parentRefTargets[targetID] = true
		}
	}

	added, removed := diffOutgoingInverseRelations(currentOutgoing, parentOutgoing)
	for targetID, irs := range added {
		addedInverseRelations[targetID] = append(addedInverseRelations[targetID], irs...)
	}
	for targetID, irs := range removed {
		removedInverseRelations[targetID] = append(removedInverseRelations[targetID], irs...)
	}

	// A target's counts.references changes when this document starts or stops referencing it.
	return b.collectChangedReferenceTargets(ctx, currentRefTargets, parentRefTargets, referenceTargets)
}

// getSeq reads the current last-indexed seq from the bridge table.
func (b *Bridge) getSeq(ctx context.Context) (int64, errors.E) {
	var seq int64
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT "seq" FROM "`+b.Store.Prefix+`Bridge"`).Scan(&seq)
		return internalStore.WithPgxError(err)
	})
	return seq, errE
}

// Fetch latest metadata and merge inverse relations for all affected documents.
type preparedUpdate struct {
	id       identifier.Identifier
	version  store.Version
	metadata *store.DocumentMetadata
}

// updateSeq advances the bridge table to seq, updates document metadata with inverse
// relations, and enqueues both the documents whose inverse relations changed and the
// documents whose counts.references must be refreshed (referenceTargets) for re-indexing,
// all in a single transaction.
func (b *Bridge) updateSeq(
	ctx context.Context, seq int64,
	addedInverseRelations, removedInverseRelations map[identifier.Identifier][]store.InverseRelation,
	referenceTargets map[identifier.Identifier]bool,
) errors.E {
	logger := zerolog.Ctx(ctx)
	start := time.Now()

	// TODO: How to get MetricDatabaseRetries inside RetryTransaction to be incremented at every loop here?
	for range internalStore.MaxRetries {
		// Collect all affected document IDs from both added and removed maps.
		affectedDocs := map[identifier.Identifier]bool{}
		for docID, irs := range addedInverseRelations {
			if len(irs) > 0 {
				affectedDocs[docID] = true
			}
		}
		for docID, irs := range removedInverseRelations {
			if len(irs) > 0 {
				affectedDocs[docID] = true
			}
		}

		var updates []preparedUpdate
		for docID := range affectedDocs {
			_, metadata, version, _, errE := b.Store.GetLatest(ctx, docID)
			if errors.Is(errE, store.ErrValueNotFound) {
				// Document does not exist (yet), skip.
				// TODO: We should handle the "not exist yet" case better.
				//       We could every time a new document is inserted make a background job which would run an ES query to
				//       find all relations pointing to it and update metadata new document's metadata and then re-index it.
				//       Or, we can index the metadata column in PostgreSQL and then query that to obtain current inverse
				//       relations inside a PostgreSQL transaction.
				continue
			} else if errors.Is(errE, store.ErrValueDeleted) {
				// Document does not exist anymore, skip.
				// TODO: We should keep track in source document's metadata, that some of its outgoing relations are invalid.
				//       This can then be used to prompt the user to fix those relations. We could even use the metadata to
				//       show links for those relations in red color in UI or something like that.
				continue
			} else if errE != nil {
				return errE
			}
			metadata.RemoveInverseRelations(removedInverseRelations[docID])
			metadata.AddInverseRelations(addedInverseRelations[docID])
			updates = append(updates, preparedUpdate{id: docID, version: version, metadata: metadata})
		}

		// Enqueue both the documents whose inverse-relation metadata changed and the
		// documents whose counts.references must be refreshed; the same worker re-indexes
		// both. Reference targets get no metadata update.
		enqueue := make(map[identifier.Identifier]bool, len(updates)+len(referenceTargets))
		for _, u := range updates {
			enqueue[u.id] = true
		}
		for docID := range referenceTargets {
			enqueue[docID] = true
		}

		// In a single transaction: update metadata, enqueue document IDs for re-indexing,
		// and then advance the bridge seq. The order matters: the INSERT into
		// BridgeReindexQueue triggers a notification BEFORE the UPDATE of Bridge seq
		// triggers the BridgeSeq notification. Since notifications are delivered in order
		// within a transaction and processed sequentially by the listener, the handler for
		// BridgeReindexQueueMinSeq queries the current MIN(seq) and updates
		// reindexQueueMinSeq before the BridgeSeq handler unblocks waitForLastSeq.
		errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			for _, u := range updates {
				_, errE := b.Store.UpdateExistingMetadata(ctx, u.id, u.version, u.metadata)
				if errE != nil {
					return errE
				}
			}

			if len(enqueue) > 0 {
				// Add document IDs with commit seq to the work queue for re-indexing.
				for docID := range enqueue {
					_, err := tx.Exec(ctx, `
						INSERT INTO "`+b.Store.Prefix+`BridgeReindexQueue" ("id", "seq") VALUES ($1, $2)
							ON CONFLICT ("id", "seq") DO NOTHING
					`, docID.String(), seq)
					if err != nil {
						return internalStore.WithPgxError(err)
					}
				}

				// Submit a job to process the queued documents.
				_, err := b.riverClient.InsertTx(ctx, tx, jobArgs{
					Schema: b.schema,
					Prefix: b.Store.Prefix,
				}, nil)
				if err != nil {
					return errors.WithStack(err)
				}
			}

			// Advance the bridge seq last, so its notification arrives after BridgeReindexQueueMinSeq.
			_, err := tx.Exec(ctx, `UPDATE "`+b.Store.Prefix+`Bridge" SET "seq" = $1 WHERE "seq" < $1`, seq)
			return internalStore.WithPgxError(err)
		})
		if errors.Is(errE, store.ErrRevisionMismatch) {
			// Concurrent update changed a revision, refetch and retry.
			continue
		}
		if errE == nil {
			// Each enqueued document becomes a row in BridgeReindexQueue, and a non-empty enqueue
			// submits one reindex job. Logging this shows how many jobs each commit triggers.
			logger.Debug().
				Int64("seq", seq).
				Int("metadataUpdates", len(updates)).
				Int("enqueued", len(enqueue)).
				Bool("jobSubmitted", len(enqueue) > 0).
				Dur("duration", time.Since(start)).
				Msg("bridge updated seq")
		}
		return errE
	}

	return errors.WithStack(internalStore.ErrMaxRetriesReached)
}

// reindexStats accumulates per-job timing and counts for a reindex job so that the job summary can
// show where time went: fetching documents from the work queue, converting them, bulk-indexing them,
// and deleting their processed entries.
type reindexStats struct {
	// Reindexed is the number of documents bulk-indexed to ElasticSearch by the job.
	Reindexed int
	// Skipped is the number of queued documents that were deleted or never existed, so they were not
	// indexed, but whose queue entries were still removed.
	Skipped int
	// Batches is the number of bulk-index plus delete flushes the job performed.
	Batches int
	// Queries is the number of work-queue SELECT queries run, including the final one that returns no rows.
	Queries int
	// ScheduledFollowUp is true if the job hit the soft deadline and scheduled a follow-up job to continue.
	ScheduledFollowUp bool
	// QueryDuration is the total time spent in the work-queue SELECT queries.
	QueryDuration time.Duration
	// GetLatestDuration is the total time spent reading the latest version of each document from the store.
	GetLatestDuration time.Duration
	// ConvertDuration is the time spent in ConvertDocument excluding the related-document store fetches.
	ConvertDuration time.Duration
	// IndexDuration is the total time spent in the ElasticSearch bulk index requests.
	IndexDuration time.Duration
	// DeleteDuration is the total time spent deleting processed entries from the work queue.
	DeleteDuration time.Duration
}

// reindexJobOutput is recorded on the River job via RecordOutput so that what each reindex job did,
// and where its time went, is visible. Durations are in seconds.
type reindexJobOutput struct {
	Seq             int64   `json:"seq"`
	Duration        float64 `json:"duration"`
	RefreshDuration float64 `json:"refreshDuration"`

	// Values from reindexStats.
	Reindexed         int     `json:"reindexed"`
	Skipped           int     `json:"skipped"`
	Batches           int     `json:"batches"`
	Queries           int     `json:"queries"`
	ScheduledFollowUp bool    `json:"scheduledFollowUp"`
	QueryDuration     float64 `json:"queryDuration"`
	GetLatestDuration float64 `json:"getLatestDuration"`
	ConvertDuration   float64 `json:"convertDuration"`
	IndexDuration     float64 `json:"indexDuration"`
	DeleteDuration    float64 `json:"deleteDuration"`

	// Values from ConversionStats.
	DocCacheHits    int     `json:"docCacheHits"`
	DocCacheMisses  int     `json:"docCacheMisses"`
	InfoCacheHits   int     `json:"infoCacheHits"`
	InfoCacheMisses int     `json:"infoCacheMisses"`
	FetchDuration   float64 `json:"fetchDuration"`
}

func (b *Bridge) runReindexQueue(ctx context.Context, job *river.Job[jobArgs]) errors.E {
	logger := zerolog.Ctx(ctx)
	jobStart := time.Now()

	// Snapshot the bridge seq, then refresh the index. updateSeq advances the bridge seq in the
	// same transaction that enqueues an entry, after indexCommit has bulk-indexed the changed
	// documents, so every commit at or below this seq is already in ES and the refresh makes
	// those documents searchable. We then process only entries at or below the snapshot, so that
	// recomputing a target's counts.references (an ElasticSearch count query, which sees only
	// refreshed documents) counts every referrer whose entry we are about to clear. Entries
	// enqueued by later commits are left for those commits' own jobs, so we refresh once per run.
	snapshotSeq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
	}
	refreshStart := time.Now()
	_, err := b.ESClient.Indices.Refresh().Index(b.Index).Do(ctx)
	if err != nil {
		return WithESError(err)
	}
	refreshDuration := time.Since(refreshStart)

	// Accumulate conversion stats across the whole job so the summary can show cache effectiveness.
	var convStats ConversionStats
	ctx = WithConversionStats(ctx, &convStats)

	// The job drains the queue until the soft deadline, then flushes what it has and schedules a follow-up
	// job to continue, keeping each job comfortably under the River job timeout.
	deadline := jobStart.Add(b.reindexSoftDeadline)
	stats, errE := b.processReindexQueue(ctx, snapshotSeq, deadline)
	duration := time.Since(jobStart)

	// Record what the job did as its River job output.
	err = river.RecordOutput(ctx, reindexJobOutput{
		Seq:             snapshotSeq,
		Duration:        duration.Seconds(),
		RefreshDuration: refreshDuration.Seconds(),

		Reindexed:         stats.Reindexed,
		Skipped:           stats.Skipped,
		Batches:           stats.Batches,
		Queries:           stats.Queries,
		ScheduledFollowUp: stats.ScheduledFollowUp,
		QueryDuration:     stats.QueryDuration.Seconds(),
		GetLatestDuration: stats.GetLatestDuration.Seconds(),
		ConvertDuration:   stats.ConvertDuration.Seconds(),
		IndexDuration:     stats.IndexDuration.Seconds(),
		DeleteDuration:    stats.DeleteDuration.Seconds(),

		DocCacheHits:    convStats.DocCacheHits,
		DocCacheMisses:  convStats.DocCacheMisses,
		InfoCacheHits:   convStats.InfoCacheHits,
		InfoCacheMisses: convStats.InfoCacheMisses,
		FetchDuration:   convStats.FetchDuration.Seconds(),
	})
	if err != nil {
		logger.Error().Err(errors.WithStack(err)).Int64("job", job.ID).Msg("recording reindex job output failed")
	}

	// The same breakdown is also logged at debug level.
	logger.Debug().
		Int64("job", job.ID).
		Int64("seq", snapshotSeq).
		Dur("duration", duration).
		Dur("refreshDuration", refreshDuration).

		// Values from reindexStats.
		Int("reindexed", stats.Reindexed).
		Int("skipped", stats.Skipped).
		Int("batches", stats.Batches).
		Int("queries", stats.Queries).
		Bool("scheduledFollowUp", stats.ScheduledFollowUp).
		Dur("queryDuration", stats.QueryDuration).
		Dur("getLatestDuration", stats.GetLatestDuration).
		Dur("convertDuration", stats.ConvertDuration).
		Dur("indexDuration", stats.IndexDuration).
		Dur("deleteDuration", stats.DeleteDuration).

		// Values from ConversionStats.
		Int("docCacheHits", convStats.DocCacheHits).
		Int("docCacheMisses", convStats.DocCacheMisses).
		Int("infoCacheHits", convStats.InfoCacheHits).
		Int("infoCacheMisses", convStats.InfoCacheMisses).
		Dur("fetchDuration", convStats.FetchDuration).
		Msg("bridge reindex job")

	return errE
}

// processReindexQueue drains the BridgeReindexQueue work queue for entries at or below snapshotSeq.
// It fetches documents in batches, converts and bulk-indexes them to ElasticSearch, and removes the
// processed entries. Documents are flushed (bulk indexed and their queue entries deleted) at the end of
// each fetched batch, whenever the flush interval elapses (so a waiter in WaitUntilCaughtUp keeps seeing
// progress), and when the soft deadline is reached. On the deadline a follow-up job is scheduled in the
// same transaction as the final delete so the chain cannot break, and the job returns. It returns timing
// and count stats for the job.
func (b *Bridge) processReindexQueue(ctx context.Context, snapshotSeq int64, deadline time.Time) (reindexStats, errors.E) {
	var stats reindexStats
	var pending []reindexEntry
	lastFlush := time.Now()

	// flush bulk-indexes and clears the documents accumulated since the last flush, then resets pending
	// and the flush timer.
	flush := func(scheduleFollowUp bool) errors.E {
		if len(pending) == 0 {
			return nil
		}
		errE := b.flushReindexBatch(ctx, snapshotSeq, pending, scheduleFollowUp, &stats)
		if errE != nil {
			return errE
		}
		pending = pending[:0]
		lastFlush = time.Now()
		return nil
	}

	for {
		fetched, errE := b.fetchReindexBatch(ctx, snapshotSeq, &stats)
		if errE != nil {
			return stats, errE
		}
		if len(fetched) == 0 {
			// Queue drained at or below the snapshot.
			break
		}

		for _, f := range fetched {
			docID, errE := identifier.MaybeString(f.idStr)
			if errE != nil {
				return stats, errE
			}

			f.doc, errE = b.convertForReindex(ctx, docID, &stats)
			if errE != nil {
				return stats, errE
			}
			pending = append(pending, f)

			// We check the deadline after each document because a single large document can take a while to
			// convert, so a fixed batch could otherwise overshoot the deadline.
			if time.Now().After(deadline) {
				// We flush with scheduleFollowUp set to true.
				errE = flush(true)
				if errE != nil {
					return stats, errE
				}
				stats.ScheduledFollowUp = true
				return stats, nil
			}

			// Flush periodically so that a waiter in WaitUntilCaughtUp keeps seeing progress. The interval is
			// the progress print rate, so each printed update reflects a recent flush. Rare printed updates
			// might not progress when intervals align, but we are fine with that.
			if time.Since(lastFlush) >= indexer.ProgressPrintRate {
				errE = flush(false)
				if errE != nil {
					return stats, errE
				}
			}
		}

		// Flush the rest of this batch before fetching the next one, so that the rows we just processed are
		// removed and the next fetch does not return them again.
		errE = flush(false)
		if errE != nil {
			return stats, errE
		}
	}

	return stats, nil
}

// reindexEntry is one document fetched from the reindex queue. doc is set during conversion and is nil for
// documents that were deleted or never existed: they are not indexed, but their queue entries are still removed.
type reindexEntry struct {
	idStr  string
	maxSeq int64
	doc    *Document
}

// fetchReindexBatch reads up to reindexMaxBatch distinct documents from the reindex queue with their max
// seq at or below snapshotSeq. GROUP BY collapses multiple entries for the same document (from different
// commits). Documents are picked at random to reduce conflicts when multiple processes reindex in parallel.
func (b *Bridge) fetchReindexBatch(ctx context.Context, snapshotSeq int64, stats *reindexStats) ([]reindexEntry, errors.E) {
	arguments := []any{
		snapshotSeq, reindexMaxBatch,
	}
	var fetched []reindexEntry
	queryStart := time.Now()
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		fetched = nil

		rows, err := tx.Query(ctx, `
			SELECT "id", MAX("seq") FROM "`+b.Store.Prefix+`BridgeReindexQueue"
				WHERE "seq" <= $1 GROUP BY "id" ORDER BY RANDOM() LIMIT $2
		`, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		var idStr string
		var maxSeq int64
		_, err = pgx.ForEachRow(rows, []any{&idStr, &maxSeq}, func() error {
			fetched = append(fetched, reindexEntry{idStr: idStr, maxSeq: maxSeq, doc: nil})
			return nil
		})
		return internalStore.WithPgxError(err)
	})
	stats.QueryDuration += time.Since(queryStart)
	stats.Queries++
	if errE != nil {
		return nil, errE
	}
	return fetched, nil
}

// flushReindexBatch bulk-indexes the converted documents in pending, removes their queue entries up to the
// seq observed for each, and optionally schedules a follow-up job in the same transaction as the delete.
func (b *Bridge) flushReindexBatch(
	ctx context.Context, snapshotSeq int64, pending []reindexEntry, scheduleFollowUp bool, stats *reindexStats,
) errors.E {
	indexStart := time.Now()
	indexed, errE := b.bulkIndexReindexed(ctx, snapshotSeq, pending)
	stats.IndexDuration += time.Since(indexStart)
	if errE != nil {
		return errE
	}

	// Build the (id, maxSeq) arrays for the delete. Entries with a higher seq (added during our processing)
	// are kept for later re-indexing.
	ids := make([]string, len(pending))
	maxSeqs := make([]int64, len(pending))
	for i, e := range pending {
		ids[i] = e.idStr
		maxSeqs[i] = e.maxSeq
	}

	deleteStart := time.Now()
	errE = internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			DELETE FROM "`+b.Store.Prefix+`BridgeReindexQueue" q
				USING (SELECT unnest($1::text[]) AS "id", unnest($2::bigint[]) AS "maxseq") v
				WHERE q."id" = v."id" AND q."seq" <= v."maxseq"
		`, ids, maxSeqs)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if scheduleFollowUp {
			// Schedule the follow-up in the same transaction as the delete so that, if entries remain, a job
			// to process them is guaranteed to exist once this delete commits.
			_, err := b.riverClient.InsertTx(ctx, tx, jobArgs{
				Schema: b.schema,
				Prefix: b.Store.Prefix,
			}, nil)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
	stats.DeleteDuration += time.Since(deleteStart)
	if errE != nil {
		return errE
	}

	stats.Reindexed += indexed
	stats.Skipped += len(pending) - indexed
	stats.Batches++
	return nil
}

// bulkIndexReindexed bulk-indexes the non-skipped documents in pending and returns how many were indexed.
func (b *Bridge) bulkIndexReindexed(ctx context.Context, snapshotSeq int64, pending []reindexEntry) (int, errors.E) {
	bulkService := b.ESClient.Bulk()
	debugDocs := map[string]*Document{}
	indexed := 0
	for _, e := range pending {
		if e.doc == nil {
			continue
		}
		id := e.idStr
		err := bulkService.IndexOp(types.IndexOperation{Index_: &b.Index, Id_: &id}, e.doc) //nolint:exhaustruct
		if err != nil {
			return 0, errors.WithStack(err)
		}
		debugDocs[id] = e.doc
		indexed++
	}
	if indexed == 0 {
		return 0, nil
	}

	response, err := bulkService.Do(ctx)
	if err != nil {
		return 0, WithESError(err)
	}
	bulkErrors := []bulkError{}
	for _, item := range response.Items {
		for _, result := range item {
			if result.Status >= 200 && result.Status <= 299 {
				continue
			}
			id := ""
			if result.Id_ != nil {
				id = *result.Id_
			}
			bulkErrors = append(bulkErrors, bulkError{
				ID:         id,
				Status:     result.Status,
				ErrorCause: result.Error,
				Doc:        debugDocs[id],
			})
		}
	}
	if len(bulkErrors) > 0 {
		errE := errors.New("bulk indexing had failures")
		errors.Details(errE)["seq"] = snapshotSeq
		// We do not name this field "errors" to not confuse go-errors package which tries to parse it as joined errors.
		errors.Details(errE)["esErrors"] = bulkErrors
		return 0, errE
	}
	return indexed, nil
}

// convertForReindex fetches the latest version of a document and converts it to a search document for re-indexing.
// It returns a nil document (and nil error) when the document was deleted or never existed, in which case the caller
// still removes its queue entry but does not index it.
func (b *Bridge) convertForReindex(ctx context.Context, docID identifier.Identifier, stats *reindexStats) (*Document, errors.E) {
	getLatestStart := time.Now()
	data, metadata, _, _, errE := b.Store.GetLatest(ctx, docID)
	stats.GetLatestDuration += time.Since(getLatestStart)
	if errors.Is(errE, store.ErrValueDeleted) {
		// Document does not exist anymore, skip.
		// TODO: We should keep track in source document's metadata, that some of its outgoing relations are invalid.
		//       This can then be used to prompt the user to fix those relations. We could even use the metadata to
		//       show links for those relations in red color in UI or something like that.
		return nil, nil //nolint:nilnil
	} else if errors.Is(errE, store.ErrValueNotFound) {
		// Document never existed. This happens for a reference target enqueued for a
		// counts.references refresh that does not exist (a dangling reference). Skipping it
		// loses nothing: a document is indexed by its own creation commit, so if this one
		// is created later, that commit indexes it and computes its counts.references.
		// The ErrValueNotFound error should not be possible for inverse-relation documents
		// at this point because it means that the document have never existed, but GetLatest
		// did not return ErrValueNotFound in updateSeq for us to be here.
		return nil, nil //nolint:nilnil
	} else if errE != nil {
		return nil, errE
	}

	// ConvertDocument also fetches related documents, recorded separately as FetchDuration. We subtract that
	// so ConvertDuration is disjoint from the fetches: only the rendering and the counts.references query.
	convStats := conversionStatsFromContext(ctx)
	var fetchBefore time.Duration
	if convStats != nil {
		fetchBefore = convStats.FetchDuration
	}
	convertStart := time.Now()
	// TODO: Use also information about the view so that documents are searchable by view as well.
	searchDoc, errE := b.ConvertDocument(ctx, data, metadata)
	convertElapsed := time.Since(convertStart)
	if convStats != nil {
		convertElapsed -= convStats.FetchDuration - fetchBefore
	}
	stats.ConvertDuration += convertElapsed
	if errE != nil {
		return nil, errE
	}
	return searchDoc, nil
}
