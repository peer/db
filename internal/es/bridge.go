// Package es provides ElasticSearch integration functionality for PeerDB.
package es

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgxlisten"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

const bridgeRetryDelay = 5 * time.Second

type bulkError struct {
	ID    string                `json:"id"`
	Error *elastic.ErrorDetails `json:"error,omitempty"`
}

// Bridge synchronizes changes from the store to ElasticSearch.
//
// It saves progress in a PostgreSQL table so it resumes from where it left off on restart.
type Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	// Store is the store to read documents from.
	Store *store.Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]

	// ESClient is the ElasticSearch client.
	ESClient *elastic.Client

	// Index is the ElasticSearch index name.
	Index string

	// Committed is the channel of newly committed changesets from the store listener.
	Committed <-chan store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]

	// Listener is the shared NOTIFY listener used to receive bridge table update notifications.
	Listener *pgxlisten.Listener

	dbpool  *pgxpool.Pool
	mu      sync.RWMutex
	seqCond *sync.Cond
	lastSeq int64
}

// Init creates the bridge progress table and registers a NOTIFY handler on the shared listener
// so that WaitUntilCaughtUp is notified immediately when the bridge seq advances.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Init(ctx context.Context, dbpool *pgxpool.Pool) errors.E {
	if b.dbpool != nil {
		return errors.New("already initialized")
	}
	if b.Committed == nil {
		return errors.New("committed channel is nil")
	}
	if b.Listener == nil {
		return errors.New("listener is nil")
	}

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

	b.seqCond = sync.NewCond(b.mu.RLocker())
	b.Listener.Handle(b.Store.Prefix+"BridgeSeq", b)

	b.dbpool = dbpool

	return nil
}

// HandleNotification implements pgxlisten.Handler interface.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) HandleNotification(
	ctx context.Context, notification *pgconn.Notification, conn *pgx.Conn,
) error {
	switch notification.Channel {
	case b.Store.Prefix + "BridgeSeq":
		return b.handleBridgeSeq(ctx, notification, conn)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = notification.Channel
		return errE
	}
}

// handleBridgeSeq handles BridgeSeq notifications from the Bridge table trigger and
// broadcasts to any goroutines waiting in WaitUntilCaughtUp.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) handleBridgeSeq(
	ctx context.Context, notification *pgconn.Notification, _ *pgx.Conn,
) error {
	seq, err := strconv.ParseInt(notification.Payload, 10, 64)
	if err != nil {
		errE := errors.WithMessage(err, "failed to parse bridge seq notification payload")
		errors.Details(errE)["payload"] = notification.Payload
		return errE
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if seq > b.lastSeq {
		b.lastSeq = seq
		b.seqCond.Broadcast()
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
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Start(ctx context.Context) {
	go func() {
		// TODO: Measure how many retries have to be made and abort if it is too much.
		//       The goal is that if this is happening too often, we should terminate the whole process and let the
		//       process supervisor decide what to do about instability (it is probably not a local thing).
		for {
			errE := b.run(ctx)
			if errors.Is(errE, context.Canceled) || errors.Is(errE, context.DeadlineExceeded) {
				// No need to retry. We are stopping.
				return
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
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) WaitUntilCaughtUp(ctx context.Context) errors.E {
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

	b.seqCond.L.Lock()
	defer b.seqCond.L.Unlock()

	// This is based on example for context.AfterFunc from the context package.
	// See comments there for explanation how it works and why.
	stop := context.AfterFunc(ctx, func() {
		b.seqCond.L.Lock()
		defer b.seqCond.L.Unlock()
		b.seqCond.Broadcast()
	})
	defer stop()

	for b.lastSeq < maxSeq {
		b.seqCond.Wait()
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
	}

	return nil
}

func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) run(ctx context.Context) errors.E {
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
			var errE errors.E
			errE = b.indexCommit(ctx, commit)
			if errE != nil {
				return errE
			}
			errE = b.updateSeq(ctx, commit.Seq)
			if errE != nil {
				return errE
			}
			lastSeq = commit.Seq
		}
		if len(commits) < store.MaxPageLength {
			break
		}
	}

	// Real-time: process new commits from the channel.
	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case c, ok := <-b.Committed:
			if !ok {
				// We never close the channel, so this should never happen.
				panic(errors.New("committed channel is closed"))
			}
			// Skip commits already processed during catch-up.
			if c.Seq <= lastSeq {
				continue
			}
			var errE errors.E
			errE = b.indexCommit(ctx, c)
			if errE != nil {
				return errE
			}
			// The bridge table is only advanced after indexing returned no error.
			errE = b.updateSeq(ctx, c.Seq)
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
// of each document, and index them to ElasticSearch as a single bulk request.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) indexCommit(
	ctx context.Context,
	committed store.CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) errors.E {
	// Reconstruct changesets with the store so we can query them.
	c, errE := committed.WithStore(ctx, b.Store)
	if errE != nil {
		errors.Details(errE)["seq"] = committed.Seq
		errors.Details(errE)["view"] = committed.View.Name()
		return errE
	}

	bulkService := b.ESClient.Bulk()

	for _, cs := range c.Changesets {
		var after *identifier.Identifier
		for {
			page, errE := cs.Changes(ctx, after)
			if errE != nil {
				errors.Details(errE)["seq"] = committed.Seq
				errors.Details(errE)["view"] = committed.View.Name()
				errors.Details(errE)["changeset"] = cs.String()
				return errE
			}
			for _, change := range page {
				data, _, _, errE := b.Store.GetLatest(ctx, change.ID)
				if errE != nil {
					if errors.Is(errE, store.ErrValueDeleted) {
						// Document was deleted: remove it from the index.
						bulkService.Add(elastic.NewBulkDeleteRequest().Index(b.Index).Id(change.ID.String()))
						continue
					}
					errors.Details(errE)["seq"] = committed.Seq
					errors.Details(errE)["view"] = committed.View.Name()
					errors.Details(errE)["changeset"] = cs.String()
					// We do not add "revision" (change.Version.Revision) because it might be unrelated to latest revision.
					errors.Details(errE)["change"] = change.ID.String()
					return errE
				}
				// TODO: Convert data into searchable document for the general case.
				// TODO: Use also information about the view so that documents are searchable by view as well.
				bulkService.Add(elastic.NewBulkIndexRequest().Index(b.Index).Id(change.ID.String()).Doc(data))
			}
			if len(page) < store.MaxPageLength {
				break
			}
			after = &page[store.MaxPageLength-1].ID
		}
	}

	if bulkService.NumberOfActions() == 0 {
		return nil
	}

	response, err := bulkService.Do(ctx)
	if err != nil {
		return errors.WithStack(err)
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
		errors.Details(errE)["errors"] = bulkErrors
		return errE
	}

	return nil
}

// getSeq reads the current last-indexed seq from the bridge table.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) getSeq(ctx context.Context) (int64, errors.E) {
	var seq int64
	errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(ctx, `SELECT "seq" FROM "`+b.Store.Prefix+`Bridge"`).Scan(&seq)
		return internal.WithPgxError(err)
	})
	return seq, errE
}

// updateSeq advances the bridge table to seq.
func (b *Bridge[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) updateSeq(ctx context.Context, seq int64) errors.E {
	// It updates the seq only if seq is greater than what is stored.
	return internal.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `UPDATE "`+b.Store.Prefix+`Bridge" SET "seq" = $1 WHERE "seq" < $1`, seq)
		return internal.WithPgxError(err)
	})
}
