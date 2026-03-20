// Package search provides ElasticSearch integration functionality for PeerDB.
package search

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

const bridgeRetryDelay = 5 * time.Second

var errCommittedChannelClosed = errors.Base("committed channel is closed")

type bulkError struct {
	ID    string                `json:"id"`
	Error *elastic.ErrorDetails `json:"error,omitempty"`
}

type bridgeJob interface {
	runIndexInverseRelations(ctx context.Context, job *river.Job[jobArgs]) errors.E
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
	return "BridgeIndexInverseRelations"
}

// InsertOpts implements river.JobArgsWithInsertOpts interface.
func (jobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{ //nolint:exhaustruct
		// We want only one job running at a time, but we also want that another job can be scheduled
		// why another job is running. So we limit only by few job states and args (with args we allow
		// each schema/prefix to have their own jobs).
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: 0,
			ByQueue:  false,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
			ExcludeKind: false,
		},
	}
}

type worker struct {
	river.WorkerDefaults[jobArgs]
}

// Work implements river.Worker interface.
func (w *worker) Work(ctx context.Context, job *river.Job[jobArgs]) error {
	c, errE := w.getBridge(job.Args.Schema, job.Args.Prefix)
	if errE != nil {
		return errE
	}

	errE = c.runIndexInverseRelations(ctx, job)
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
	Store *store.Store[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes]

	// ESClient is the ElasticSearch client.
	ESClient *elastic.Client

	// Index is the ElasticSearch index name.
	Index string

	dbpool                     *pgxpool.Pool
	schema                     string
	riverClient                *river.Client[pgx.Tx]
	converter                  *Converter
	lastSeqMu                  sync.RWMutex
	lastSeqCond                *sync.Cond
	lastSeq                    int64
	inverseRelationsMinSeqMu   sync.RWMutex
	inverseRelationsMinSeqCond *sync.Cond
	// inverseRelationsMinSeq is the MIN(seq) of remaining rows in BridgeInverseRelations,
	// or math.MaxInt64 if the table is empty. A waiter for seq X is done when this value > X.
	inverseRelationsMinSeq int64
}

// Init creates the bridge progress table and registers a NOTIFY handler on the shared listener
// so that WaitUntilCaughtUp is notified immediately when the bridge seq advances.
func (b *Bridge) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internal.Listener, schema string,
	riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if b.dbpool != nil {
		return errors.New("already initialized")
	}
	b.dbpool = dbpool
	b.schema = schema
	b.riverClient = riverClient

	errE := internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
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

			-- "BridgeInverseRelations" holds document IDs whose inverse relations
			-- need to be re-indexed. It acts as a work queue. The "seq" column
			-- records which commit updated inverse relations metadata, allowing
			-- detection of new table entries added during job processing.
			CREATE TABLE "`+b.Store.Prefix+`BridgeInverseRelations" (
				"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"seq" bigint NOT NULL,
				PRIMARY KEY ("id", "seq")
			);
			CREATE FUNCTION "`+b.Store.Prefix+`BridgeInverseRelationsAfterChangeFunc"()
				RETURNS TRIGGER LANGUAGE plpgsql AS $$
				DECLARE
					_min_seq bigint;
				BEGIN
					-- After rows are inserted or deleted, notify with the MIN(seq) of all rows.
					-- If the table is empty, send -1 to indicate no pending work.
					SELECT MIN("seq") INTO _min_seq FROM "`+b.Store.Prefix+`BridgeInverseRelations";
					IF _min_seq IS NULL THEN
						_min_seq := -1;
					END IF;
					PERFORM pg_notify('`+b.Store.Prefix+`BridgeInverseRelationsMinSeq', _min_seq::text);
					RETURN NULL;
				END;
			$$;
			CREATE TRIGGER "`+b.Store.Prefix+`BridgeInverseRelationsAfterChange" AFTER INSERT OR DELETE ON "`+b.Store.Prefix+`BridgeInverseRelations"
				FOR EACH STATEMENT EXECUTE FUNCTION "`+b.Store.Prefix+`BridgeInverseRelationsAfterChangeFunc"();
			CREATE TRIGGER "`+b.Store.Prefix+`BridgeInverseRelationsNotAllowed" BEFORE UPDATE OR TRUNCATE ON "`+b.Store.Prefix+`BridgeInverseRelations"
				FOR EACH STATEMENT EXECUTE FUNCTION "`+b.Store.Prefix+`DoNotAllow"();
		`)
		return internal.WithPgxError(err)
	})
	if errE != nil {
		if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
			switch pgError.Code {
			case internal.ErrorCodeDuplicateTable:
				// Nothing.
			case internal.ErrorCodeDuplicateFunction:
				// Nothing.
			default:
				return errE
			}
		} else {
			return errE
		}
	}

	errE = b.registerCoordinator(workers)
	if errE != nil {
		return errE
	}

	b.lastSeqCond = sync.NewCond(b.lastSeqMu.RLocker())
	b.inverseRelationsMinSeqCond = sync.NewCond(b.inverseRelationsMinSeqMu.RLocker())
	b.inverseRelationsMinSeq = math.MaxInt64
	listener.Handle(b.Store.Prefix+"BridgeSeq", b)
	listener.Handle(b.Store.Prefix+"BridgeInverseRelationsMinSeq", b)

	// Submit a startup job to process any leftover rows in BridgeInverseRelations
	// from a previous run. The job is persisted and will be picked up once River starts.
	_, err := b.riverClient.Insert(ctx, jobArgs{
		Schema: b.schema,
		Prefix: b.Store.Prefix,
	}, nil)
	if err != nil {
		return errors.WithStack(err)
	}

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
	case b.Store.Prefix + "BridgeInverseRelationsMinSeq":
		return b.handleBridgeInverseRelationsMinSeq(notification)
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
	case b.Store.Prefix + "BridgeInverseRelationsMinSeq":
		// TODO: Improve what happens on an error.
		//       Any error from fixBridgeInverseRelationsMinSeq is just logged. Which means that goroutines waiting
		//       in WaitUntilCaughtUp might continue waiting until some other new commit is made, which might be never.
		return b.fixBridgeInverseRelationsMinSeq(ctx)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
}

// HandlingReady implements internal.Handler interface.
func (b *Bridge) HandlingReady(ctx context.Context, channel string) errors.E {
	switch channel {
	case b.Store.Prefix + "BridgeSeq":
		return b.waitForFixBridgeSeq(ctx)
	case b.Store.Prefix + "BridgeInverseRelationsMinSeq":
		return b.waitForFixBridgeInverseRelationsMinSeq(ctx)
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

// handleBridgeInverseRelationsMinSeq handles notifications from the BridgeInverseRelations table
// trigger and broadcasts to any goroutines waiting in WaitUntilCaughtUp.
func (b *Bridge) handleBridgeInverseRelationsMinSeq(notification *pgconn.Notification) errors.E {
	minSeq, err := strconv.ParseInt(notification.Payload, 10, 64)
	if err != nil {
		errE := errors.WithMessage(err, "failed to parse inverse relations min seq notification payload")
		errors.Details(errE)["payload"] = notification.Payload
		return errE
	}
	b.inverseRelationsMinSeqMu.Lock()
	defer b.inverseRelationsMinSeqMu.Unlock()
	if minSeq < 0 {
		// A payload of "-1" means the table is empty.
		b.inverseRelationsMinSeq = math.MaxInt64
	} else {
		b.inverseRelationsMinSeq = minSeq
	}
	b.inverseRelationsMinSeqCond.Broadcast()
	return nil
}

// fixBridgeInverseRelationsMinSeq fetches the current MIN(seq) from BridgeInverseRelations,
// updates the in-memory state, and broadcasts to any goroutines waiting in WaitUntilCaughtUp.
func (b *Bridge) fixBridgeInverseRelationsMinSeq(ctx context.Context) errors.E {
	var minSeq *int64
	errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		return internal.WithPgxError(
			tx.QueryRow(ctx, `SELECT MIN("seq") FROM "`+b.Store.Prefix+`BridgeInverseRelations"`).Scan(&minSeq),
		)
	})
	if errE != nil {
		return errE
	}
	b.inverseRelationsMinSeqMu.Lock()
	defer b.inverseRelationsMinSeqMu.Unlock()
	if minSeq == nil {
		b.inverseRelationsMinSeq = math.MaxInt64
	} else {
		b.inverseRelationsMinSeq = *minSeq
	}
	b.inverseRelationsMinSeqCond.Broadcast()
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

	return b.waitForLastSeq(ctx, seq)
}

func (b *Bridge) waitForLastSeq(ctx context.Context, seq int64) errors.E {
	b.lastSeqCond.L.Lock()
	defer b.lastSeqCond.L.Unlock()

	// This is based on example for context.AfterFunc from the context package.
	// See comments there for explanation how it works and why.
	stop := context.AfterFunc(ctx, func() {
		b.lastSeqCond.L.Lock()
		defer b.lastSeqCond.L.Unlock()
		b.lastSeqCond.Broadcast()
	})
	defer stop()

	for b.lastSeq < seq {
		b.lastSeqCond.Wait()
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
	}

	return nil
}

// waitForFixBridgeInverseRelationsMinSeq is similar to WaitUntilCaughtUp but it does not wait for
// b.inverseRelationsMinSeq to catch up with committed commits, but just that it catches up with
// the current last-indexed seq from the bridge table. A startup job submitted in Init ensures
// any leftover rows will be processed.
func (b *Bridge) waitForFixBridgeInverseRelationsMinSeq(ctx context.Context) errors.E {
	// We must call fixBridgeInverseRelationsMinSeq here because HandleBacklog runs in a separate
	// goroutine and may not have executed yet.
	errE := b.fixBridgeInverseRelationsMinSeq(ctx)
	if errE != nil {
		return errE
	}

	seq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
	}

	return b.waitForInverseRelationsMinSeq(ctx, seq)
}

func (b *Bridge) waitForInverseRelationsMinSeq(ctx context.Context, seq int64) errors.E {
	b.inverseRelationsMinSeqCond.L.Lock()
	defer b.inverseRelationsMinSeqCond.L.Unlock()

	// This is based on example for context.AfterFunc from the context package.
	// See comments there for explanation how it works and why.
	stop := context.AfterFunc(ctx, func() {
		b.inverseRelationsMinSeqCond.L.Lock()
		defer b.inverseRelationsMinSeqCond.L.Unlock()
		b.inverseRelationsMinSeqCond.Broadcast()
	})
	defer stop()

	// inverseRelationsMinSeq tracks the MIN(seq) of remaining rows in BridgeInverseRelations.
	// When it exceeds seq (or the table is empty, represented as MaxInt64), we are done.
	for b.inverseRelationsMinSeq <= seq {
		b.inverseRelationsMinSeqCond.Wait()
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
	}

	return nil
}

// Start begins the bridging goroutine.
//
// It first indexes any commits from CommitLog that are newer than what is recorded in the bridge
// table (catch-up), then processes new commits from the Committed channel as they arrive.
//
// The store listener should be listening to notifications from PostgreSQL and sending them to
// the Committed channel before calling Start to assure that there is no gap between catch-up and
// real-time processing of new commits.
//
// Converter is used to convert documents for indexing and to track inverse relations.
func (b *Bridge) Start(ctx context.Context, converter *Converter) {
	b.converter = converter

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
}

// WaitUntilCaughtUp blocks until the bridge has indexed all currently committed commits.
//
// It is useful for waiting after a bulk import before querying ElasticSearch.
func (b *Bridge) WaitUntilCaughtUp(ctx context.Context) errors.E {
	// Find the current maximum seq in CommitLog.
	var maxSeq int64
	errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT COALESCE(MAX("seq"), 0) FROM "`+b.Store.Prefix+`CommitLog"`).Scan(&maxSeq)
		return internal.WithPgxError(err)
	})
	if errE != nil {
		return errE
	}

	if maxSeq == 0 {
		return nil
	}

	// We first wait on lastSeq.
	errE = b.waitForLastSeq(ctx, maxSeq)
	if errE != nil {
		return errE
	}

	// And then we wait on inverseRelationsMinSeq.
	return b.waitForInverseRelationsMinSeq(ctx, maxSeq)
}

func (b *Bridge) run(ctx context.Context) errors.E {
	// Determine where we left off.
	lastSeq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
	}

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
			inverseRelations, errE := b.indexCommit(ctx, commit)
			if errE != nil {
				return errE
			}
			errE = b.updateSeq(ctx, commit.Seq, inverseRelations)
			if errE != nil {
				return errE
			}
			lastSeq = commit.Seq
		}
		if len(commits) < store.MaxPageLength {
			break
		}
	}

	ch, errE := b.Store.Committed.Get(ctx)
	if errE != nil {
		return errE
	}

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
			inverseRelations, errE := b.indexCommit(ctx, c)
			if errE != nil {
				return errE
			}
			// The bridge table is only advanced after indexing returned no error.
			errE = b.updateSeq(ctx, c.Seq, inverseRelations)
			if errE != nil {
				return errE
			}
			lastSeq = c.Seq
		}
	}
}

// TODO: We should batch multiple commits together if they are small and split them if they are large.
//       indexCommit operates on a single commit but those could be very small or very large.
//       Maybe a batch should be made when we reach 1000 documents or if more than 1 second has
//       passed since the batch was started (so that we index with at most 1 second delay).

// indexCommit collects all document changes from the commit, fetches the latest version
// of each document, and indexes them to ElasticSearch as a single bulk request.
//
// Documents are converted for indexing and inverse relations are collected.
// The returned map contains, for each target document ID, the inverse relations that
// should be stored in that document's metadata.
func (b *Bridge) indexCommit(
	ctx context.Context,
	committed store.CommittedChangesets[json.RawMessage, *internal.DocumentMetadata, *internal.NoMetadata, *internal.NoMetadata, *internal.NoMetadata, document.Changes],
) (map[identifier.Identifier][]internal.InverseRelation, errors.E) {
	// Reconstruct changesets with the store so we can query them.
	c, errE := committed.WithStore(ctx, b.Store)
	if errE != nil {
		errors.Details(errE)["seq"] = committed.Seq
		errors.Details(errE)["view"] = committed.View.Name()
		return nil, errE
	}

	bulkService := b.ESClient.Bulk()

	// Collect inverse relations from all processed documents.
	allInverseRelations := map[identifier.Identifier][]internal.InverseRelation{}

	for _, cs := range c.Changesets {
		var after *identifier.Identifier
		for {
			page, errE := cs.Changes(ctx, after)
			if errE != nil {
				errors.Details(errE)["seq"] = committed.Seq
				errors.Details(errE)["view"] = committed.View.Name()
				errors.Details(errE)["changeset"] = cs.String()
				return nil, errE
			}
			for _, change := range page {
				data, metadata, _, errE := b.Store.GetLatest(ctx, change.ID)
				if errE != nil {
					if errors.Is(errE, store.ErrValueDeleted) {
						// Document was deleted: remove it from the index.
						// TODO: We have to also remove all inverse relations from metadata and the index.
						bulkService.Add(elastic.NewBulkDeleteRequest().Index(b.Index).Id(change.ID.String()))
						continue
					}
					errors.Details(errE)["seq"] = committed.Seq
					errors.Details(errE)["view"] = committed.View.Name()
					errors.Details(errE)["changeset"] = cs.String()
					// We do not add "revision" (change.Version.Revision) because change.Version.Revision might
					// be unrelated to latest revision.
					errors.Details(errE)["change"] = change.ID.String()
					return nil, errE
				}

				// TODO: Use also information about the view so that documents are searchable by view as well.
				searchDoc, outgoing, errE := b.convertDocument(ctx, data, metadata)
				if errE != nil {
					errors.Details(errE)["seq"] = committed.Seq
					errors.Details(errE)["view"] = committed.View.Name()
					errors.Details(errE)["changeset"] = cs.String()
					// We do not add "revision" (change.Version.Revision) because change.Version.Revision might
					// be unrelated to latest revision.
					errors.Details(errE)["change"] = change.ID.String()
					return nil, errE
				}
				bulkService.Add(elastic.NewBulkIndexRequest().Index(b.Index).Id(change.ID.String()).Doc(searchDoc))
				for targetID, irs := range outgoing {
					allInverseRelations[targetID] = append(allInverseRelations[targetID], irs...)
				}
			}
			if len(page) < store.MaxPageLength {
				break
			}
			after = &page[store.MaxPageLength-1].ID
		}
	}

	if bulkService.NumberOfActions() == 0 {
		return allInverseRelations, nil
	}

	response, err := bulkService.Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if response.Errors {
		errE := errors.New("bulk indexing had failures")
		bulkErrors := []bulkError{}
		for _, item := range response.Failed() {
			bulkErrors = append(bulkErrors, bulkError{
				ID:    item.Id,
				Error: item.Error,
			})
		}
		errors.Details(errE)["seq"] = committed.Seq
		errors.Details(errE)["view"] = committed.View.Name()
		errors.Details(errE)["esErrors"] = bulkErrors
		return nil, errE
	}

	return allInverseRelations, nil
}

// convertDocument unmarshals data into a document.D, calls the converter's FromDocument
// with inverse relations from metadata, and returns the search document and outgoing
// inverse relations.
func (b *Bridge) convertDocument(
	ctx context.Context, data json.RawMessage, metadata *internal.DocumentMetadata,
) (*Document, map[identifier.Identifier][]internal.InverseRelation, errors.E) {
	var doc document.D
	errE := x.UnmarshalWithoutUnknownFields(data, &doc)
	if errE != nil {
		return nil, nil, errE
	}

	return b.converter.FromDocument(ctx, &doc, metadata.InverseRelations)
}

// getSeq reads the current last-indexed seq from the bridge table.
func (b *Bridge) getSeq(ctx context.Context) (int64, errors.E) {
	var seq int64
	errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT "seq" FROM "`+b.Store.Prefix+`Bridge"`).Scan(&seq)
		return internal.WithPgxError(err)
	})
	return seq, errE
}

// updateSeq advances the bridge table to seq and updates document metadata with
// inverse relations, all in a single transaction.
func (b *Bridge) updateSeq(
	ctx context.Context, seq int64, inverseRelations map[identifier.Identifier][]internal.InverseRelation,
) errors.E {
	// TODO: How to get MetricDatabaseRetries inside RetryTransaction to be incremented at every loop here?
	for range internal.MaxRetries {
		// Fetch latest metadata and merge inverse relations for all affected documents.
		type preparedUpdate struct {
			id       identifier.Identifier
			version  store.Version
			metadata *internal.DocumentMetadata
		}
		var updates []preparedUpdate
		for docID, irs := range inverseRelations {
			_, metadata, version, errE := b.Store.GetLatest(ctx, docID)
			if errE != nil {
				if errors.Is(errE, store.ErrValueNotFound) {
					// Document does not exist (yet or anymore), skip.
					// TODO: What do to here?
					continue
				}
				return errE
			}
			metadata.Merge(irs)
			updates = append(updates, preparedUpdate{id: docID, version: version, metadata: metadata})
		}

		// In a single transaction: update metadata, enqueue document IDs for re-indexing,
		// and then advance the bridge seq. The order matters: the INSERT into
		// BridgeInverseRelations triggers a notification with MIN(seq) BEFORE the UPDATE
		// of Bridge seq triggers the BridgeSeq notification. Since notifications are
		// delivered in order within a transaction and processed sequentially by the listener,
		// waitForInverseRelationsMinSeq sees the correct value before waitForLastSeq returns.
		errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			for _, u := range updates {
				_, errE := b.Store.UpdateExistingMetadata(ctx, u.id, u.version, u.metadata)
				if errE != nil {
					return errE
				}
			}

			if len(updates) > 0 {
				// Add document IDs with commit seq to the work queue for re-indexing.
				for _, u := range updates {
					_, err := tx.Exec(ctx, `
						INSERT INTO "`+b.Store.Prefix+`BridgeInverseRelations" ("id", "seq") VALUES ($1, $2)
							ON CONFLICT ("id", "seq") DO NOTHING
					`, u.id.String(), seq)
					if err != nil {
						return internal.WithPgxError(err)
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

			// Advance the bridge seq last, so its notification arrives after BridgeInverseRelationsMinSeq.
			_, err := tx.Exec(ctx, `UPDATE "`+b.Store.Prefix+`Bridge" SET "seq" = $1 WHERE "seq" < $1`, seq)
			if err != nil {
				return internal.WithPgxError(err)
			}

			return nil
		})
		if errors.Is(errE, store.ErrRevisionMismatch) {
			// Concurrent update changed a revision, refetch and retry.
			continue
		}
		return errE
	}

	return errors.WithStack(internal.ErrMaxRetriesReached)
}

func (b *Bridge) runIndexInverseRelations(ctx context.Context, _ *river.Job[jobArgs]) errors.E {
	for {
		// Fetch one document ID from the work queue with its max seq.
		// GROUP BY collapses multiple entries for the same document (from different commits).
		var docIDStr string
		var maxSeq int64
		errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
			return internal.WithPgxError(tx.QueryRow(ctx, `
				SELECT "id", MAX("seq") FROM "`+b.Store.Prefix+`BridgeInverseRelations"
					GROUP BY "id" LIMIT 1
			`).Scan(&docIDStr, &maxSeq))
		})
		if errors.Is(errE, pgx.ErrNoRows) {
			// No more documents to process.
			return nil
		} else if errE != nil {
			return errE
		}

		docID, errE := identifier.MaybeString(docIDStr)
		if errE != nil {
			return errE
		}

		// Fetch the document and its metadata, convert it, and index it.
		errE = b.indexDocument(ctx, docID)
		if errE != nil {
			return errE
		}

		// Remove entries for this document up to the seq we observed.
		// Entries with a higher seq (added during our processing) are kept for later re-indexing.
		errE = internal.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `
				DELETE FROM "`+b.Store.Prefix+`BridgeInverseRelations" WHERE "id" = $1 AND "seq" <= $2
			`, docIDStr, maxSeq)
			return internal.WithPgxError(err)
		})
		if errE != nil {
			return errE
		}
	}
}

// TODO: We should batch indexing of documents together instead of doing it one by one.
//       We could fetch up to 1000 rows from BridgeInverseRelations, convert them, index them and then remove them from BridgeInverseRelations.

// indexDocument fetches the latest version of a document, converts it to a search
// document, and indexes it to ElasticSearch.
func (b *Bridge) indexDocument(ctx context.Context, docID identifier.Identifier) errors.E {
	data, metadata, _, errE := b.Store.GetLatest(ctx, docID)
	if errE != nil {
		if errors.Is(errE, store.ErrValueNotFound) {
			// Document was deleted, remove from index.
			// TODO: We have to also remove all inverse relations from metadata and the index.
			_, err := b.ESClient.Delete().Index(b.Index).Id(docID.String()).Do(ctx)
			if err != nil && !elastic.IsNotFound(err) {
				return errors.WithStack(err)
			}
			return nil
		}
		return errE
	}

	// TODO: Use also information about the view so that documents are searchable by view as well.
	searchDoc, _, errE := b.convertDocument(ctx, data, metadata)
	if errE != nil {
		return errE
	}

	_, err := b.ESClient.Index().Index(b.Index).Id(docID.String()).BodyJson(searchDoc).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
