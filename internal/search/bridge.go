// Package search provides ElasticSearch integration functionality for PeerDB.
package search

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olivere/elastic/v7"
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

	dbpool    *pgxpool.Pool
	converter *Converter
	mu        sync.RWMutex
	seqCond   *sync.Cond
	lastSeq   int64
}

// Init creates the bridge progress table and registers a NOTIFY handler on the shared listener
// so that WaitUntilCaughtUp is notified immediately when the bridge seq advances.
func (b *Bridge) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *internal.Listener,
) errors.E {
	if b.dbpool != nil {
		return errors.New("already initialized")
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
	listener.Handle(b.Store.Prefix+"BridgeSeq", b)

	b.dbpool = dbpool

	return nil
}

// HandleNotification implements pgxlisten.Handler interface.
func (b *Bridge) HandleNotification(
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
		return b.fixBridgeSeq(ctx)
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
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
}

// handleBridgeSeq handles BridgeSeq notifications from the Bridge table trigger and
// broadcasts to any goroutines waiting in WaitUntilCaughtUp.
func (b *Bridge) handleBridgeSeq(
	_ context.Context, notification *pgconn.Notification, _ *pgx.Conn,
) errors.E {
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

// fixBridgeSeq fetches the last seq from the Bridge table and broadcasts to any goroutines
// waiting in WaitUntilCaughtUp.
func (b *Bridge) fixBridgeSeq(
	ctx context.Context,
) errors.E {
	seq, errE := b.getSeq(ctx)
	if errE != nil {
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

// waitForFixBridgeSeq is similar to WaitUntilCaughtUp but it does not wait for b.lastSeq to catch up with
// committed commits, but just that it catches up with the current last-indexed seq from the bridge table.
func (b *Bridge) waitForFixBridgeSeq(
	ctx context.Context,
) errors.E {
	seq, errE := b.getSeq(ctx)
	if errE != nil {
		return errE
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

	for b.lastSeq < seq {
		b.seqCond.Wait()
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
					// We do not add "revision" (change.Version.Revision) because it might be unrelated to latest revision.
					errors.Details(errE)["change"] = change.ID.String()
					return nil, errE
				}

				// TODO: Use also information about the view so that documents are searchable by view as well.
				searchDoc, outgoing, errE := b.convertDocument(ctx, data, metadata)
				if errE != nil {
					errors.Details(errE)["seq"] = committed.Seq
					errors.Details(errE)["view"] = committed.View.Name()
					errors.Details(errE)["changeset"] = cs.String()
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

		// In a single transaction, advance the bridge seq and update all metadata.
		errE := internal.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
			_, err := tx.Exec(ctx, `UPDATE "`+b.Store.Prefix+`Bridge" SET "seq" = $1 WHERE "seq" < $1`, seq)
			if err != nil {
				return internal.WithPgxError(err)
			}

			for _, u := range updates {
				_, errE := b.Store.UpdateExistingMetadata(ctx, u.id, u.version, u.metadata)
				if errE != nil {
					return errE
				}
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
