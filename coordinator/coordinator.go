// Package coordinator provides a coordinator for synchronizing real-time collaboration sessions.
//
// This is a low-level component.
package coordinator

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgxlisten"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	// MaxPageLength is the maximum number of results that can be returned in a single page.
	MaxPageLength    = 5000
	maxPageLengthStr = "5000"
)

const (
	// Our PostgreSQL error codes.
	errorCodeSessionNotFound  = "P1020"
	errorCodeAlreadyEnded     = "P1021"
	errorCodeNotEnded         = "P1022"
	errorCodeAlreadyCompleted = "P1023"
	errorCodeConflict         = "P1024"
)

type coordinatorJob interface {
	runCompleteSession(ctx context.Context, session identifier.Identifier, job *river.Job[jobArgs]) errors.E
}

type schemaPrefix struct {
	Schema string
	Prefix string
}

//nolint:gochecknoglobals
var (
	coordinators   = map[schemaPrefix]coordinatorJob{}
	coordinatorsMu = sync.RWMutex{}
)

type jobArgs struct {
	Schema  string                `json:"schema"`
	Prefix  string                `json:"prefix"`
	Session identifier.Identifier `json:"session"`
}

// Kind implements river.JobArgs interface.
func (jobArgs) Kind() string {
	return "CoordinatorCompleteSession"
}

type worker struct {
	river.WorkerDefaults[jobArgs]
}

// Work implements river.Worker interface.
func (w *worker) Work(ctx context.Context, job *river.Job[jobArgs]) error {
	c, errE := w.getCoordinator(job.Args.Schema, job.Args.Prefix)
	if errE != nil {
		return errE
	}

	return c.runCompleteSession(ctx, job.Args.Session, job)
}

func (w *worker) getCoordinator(schema, prefix string) (coordinatorJob, errors.E) { //nolint:ireturn
	coordinatorsMu.RLock()
	defer coordinatorsMu.RUnlock()

	c, ok := coordinators[schemaPrefix{Schema: schema, Prefix: prefix}]
	if !ok {
		errE := errors.New("coordinator not found")
		errors.Details(errE)["schema"] = schema
		errors.Details(errE)["prefix"] = prefix
		return nil, errE
	}

	return c, nil
}

// OperationAppended represents an operation appended to a session.
type OperationAppended struct {
	Session   identifier.Identifier `json:"session"`
	Operation int64                 `json:"operation"`
}

// SessionState represents the state of a session.
type SessionState string

// SessionState values.
const (
	SessionStateEnded     SessionState = "ended"
	SessionStateCompleted SessionState = "completed"
)

// SessionStateChanged represents a change in the state of a session.
type SessionStateChanged struct {
	Session identifier.Identifier `json:"session"`
	State   SessionState          `json:"state"`
}

// Coordinator provides an append-only log of operations to support
// synchronizing real-time collaboration sessions.
//
// For every operation, its metadata and optional data are stored.
// You configure Go types for them with type parameters.
//
// Every coordinator session goes through the following lifetime:
//
//   - First, you call [Coordinator.Begin] to create a new session.
//   - Then, you call [Coordinator.Append] to append operations to the session.
//   - Finally, you call [Coordinator.End] to end the session. After the session
//     has ended, you cannot append new operations to it. After the session ends,
//     the coordinator runs the `CompleteSession` function.
//   - After `CompleteSession` successfully completes, the session is considered
//     completed and all operations for the session are deleted.
type Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata any] struct {
	// Prefix to use when initializing PostgreSQL objects used by this coordinator.
	Prefix string

	// PostgreSQL column types to store data and metadata.
	// It should probably be one of the jsonb, bytea, or text.
	// Go types used for Coordinator type parameters should be compatible with
	// column types chosen.
	DataType     string
	MetadataType string

	// CompleteSession is called after a session has ended. You should use it to use operations' data
	// and metadata to complete the session (e.g., store session results into a database or file).
	//
	// After CompleteSession successfully completes, the session is considered
	// completed and all operations for the session are deleted.
	//
	// CompleteSession should be idempotent as might be called multiple times in the case of any issues.
	// In the case of errors, it should try to revert any changes made by it, but it should not rely on those
	// changes being reverted because it might be run again even if CompleteSession itself successfully runs.
	CompleteSession func(ctx context.Context, session identifier.Identifier) (CompleteMetadata, errors.E)

	// AppendedSize is the size of the channel to which operations are send when they are appended.
	//
	// Set to a negative value to disable creating the channel.
	AppendedSize int `exhaustruct:"optional"`

	// A channel to which operations are send when they are appended.
	// Operations are sent in the order in which they were appended to the database.
	//
	// Channel is created by the listener when started and recreated on reconnection.
	Appended x.RecreatableChannel[OperationAppended] `exhaustruct:"optional"`

	// EndedSize is the size of the channel to which session state changes are send.
	//
	// Set to a negative value to disable creating the channel.
	ChangedSize int `exhaustruct:"optional"`

	// A channel to which session state changes are send.
	// State changes are sent in the order in which they were serialized by the database.
	//
	// Channel is created by the listener when started and recreated on reconnection.
	Changed x.RecreatableChannel[SessionStateChanged] `exhaustruct:"optional"`

	dbpool      *pgxpool.Pool
	schema      string
	riverClient *river.Client[pgx.Tx]
	appended    chan<- OperationAppended
	changed     chan<- SessionStateChanged
}

// Init initializes the Coordinator.
//
// It creates and configures the PostgreSQL tables, indices, and
// stored procedures if they do not already exist.
//
// A non-nil listener is required when the Appended or Ended channel is set.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) Init(
	ctx context.Context, dbpool *pgxpool.Pool, listener *pgxlisten.Listener, schema string,
	riverClient *river.Client[pgx.Tx], workers *river.Workers,
) errors.E {
	if c.dbpool != nil {
		return errors.New("already initialized")
	}

	// TODO: Use schema management/migration instead.
	errE := internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `
			CREATE TABLE "`+c.Prefix+`Sessions" (
				-- ID of the session.
				"session" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"beginMetadata" `+c.MetadataType+` NOT NULL,
				"endMetadata" `+c.MetadataType+`,
				"completeMetadata" `+c.MetadataType+`,
				PRIMARY KEY ("session")
			);
			CREATE TABLE "`+c.Prefix+`Operations" (
				-- ID of the session this operation belongs to.
				"session" text STORAGE PLAIN COLLATE "C" NOT NULL,
				-- Sequence number of this operation.
				"operation" bigint NOT NULL,
				"data" `+c.DataType+`,
				"metadata" `+c.MetadataType+` NOT NULL,
				PRIMARY KEY ("session", "operation")
			);

			CREATE FUNCTION "`+c.Prefix+`EndSession"(_session text, _metadata `+c.MetadataType+`)
				RETURNS void LANGUAGE plpgsql AS $$
				DECLARE
					_sessionEnded boolean;
				BEGIN
					-- Does session exist and has not ended.
					SELECT "endMetadata" IS NOT NULL INTO _sessionEnded
						FROM "`+c.Prefix+`Sessions" WHERE "session"=_session;
					IF NOT FOUND THEN
						RAISE EXCEPTION 'session not found' USING ERRCODE='`+errorCodeSessionNotFound+`';
					ELSIF _sessionEnded THEN
						RAISE EXCEPTION 'session already ended' USING ERRCODE='`+errorCodeAlreadyEnded+`';
					END IF;
					UPDATE "`+c.Prefix+`Sessions" SET "endMetadata"=_metadata WHERE "session"=_session;
					PERFORM pg_notify('`+c.Prefix+`SessionStateChanged', json_build_object('session', _session, 'state', '`+string(SessionStateEnded)+`')::text);
				END;
			$$;

			CREATE FUNCTION "`+c.Prefix+`CompleteSession"(_session text, _metadata `+c.MetadataType+`)
				RETURNS void LANGUAGE plpgsql AS $$
				DECLARE
					_sessionEnded boolean;
					_sessionCompleted boolean;
				BEGIN
					-- Does session exist and has ended and not completed.
					SELECT "endMetadata" IS NOT NULL, "completeMetadata" IS NOT NULL INTO _sessionEnded, _sessionCompleted
						FROM "`+c.Prefix+`Sessions" WHERE "session"=_session;
					IF NOT FOUND THEN
						RAISE EXCEPTION 'session not found' USING ERRCODE='`+errorCodeSessionNotFound+`';
					ELSIF NOT _sessionEnded THEN
						RAISE EXCEPTION 'session not ended' USING ERRCODE='`+errorCodeNotEnded+`';
					ELSIF _sessionCompleted THEN
						RAISE EXCEPTION 'session already completed' USING ERRCODE='`+errorCodeAlreadyCompleted+`';
					END IF;
					DELETE FROM "`+c.Prefix+`Operations" WHERE "session"=_session;
					UPDATE "`+c.Prefix+`Sessions" SET "completeMetadata"=_metadata WHERE "session"=_session;
					PERFORM pg_notify('`+c.Prefix+`SessionStateChanged', json_build_object('session', _session, 'state', '`+string(SessionStateCompleted)+`')::text);
				END;
			$$;

			CREATE FUNCTION "`+c.Prefix+`AppendOperation"(_session text, _metadata `+c.MetadataType+`, _data `+c.DataType+`, _expectedOperation bigint)
				RETURNS bigint LANGUAGE plpgsql AS $$
				DECLARE
					_sessionEnded boolean;
					_operation bigint;
				BEGIN
					-- Does session exist and has not ended.
					SELECT "endMetadata" IS NOT NULL INTO _sessionEnded
						FROM "`+c.Prefix+`Sessions" WHERE "session"=_session;
					IF NOT FOUND THEN
						RAISE EXCEPTION 'session not found' USING ERRCODE='`+errorCodeSessionNotFound+`';
					ELSIF _sessionEnded THEN
						RAISE EXCEPTION 'session already ended' USING ERRCODE='`+errorCodeAlreadyEnded+`';
					END IF;
					INSERT INTO "`+c.Prefix+`Operations" SELECT _session, COALESCE(MAX("operation"), 0)+1, _data, _metadata
						FROM "`+c.Prefix+`Operations" WHERE "session"=_session
						HAVING _expectedOperation IS NULL OR COALESCE(MAX("operation"), 0)+1=_expectedOperation
						RETURNING "operation" INTO _operation;
					IF NOT FOUND THEN
						RAISE EXCEPTION 'conflict' USING ERRCODE='`+errorCodeConflict+`';
					END IF;
					PERFORM pg_notify('`+c.Prefix+`OperationAppended', json_build_object('session', _session, 'operation', _operation)::text);
					RETURN _operation;
				END;
			$$;
		`)
		if err != nil {
			return internal.WithPgxError(err)
		}

		return nil
	})
	if errE != nil {
		if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
			switch pgError.Code {
			case internal.ErrorCodeUniqueViolation:
				// Nothing.
			case internal.ErrorCodeDuplicateFunction:
				// Nothing.
			case internal.ErrorCodeDuplicateTable:
				// Nothing.
			default:
				return errE
			}
		} else {
			return errE
		}
	}

	c.dbpool = dbpool
	c.schema = schema
	c.riverClient = riverClient

	errE = c.registerCoordinator(workers)
	if errE != nil {
		return errE
	}

	if listener != nil {
		if c.AppendedSize >= 0 {
			listener.Handle(c.Prefix+"OperationAppended", c)
		}
		if c.ChangedSize >= 0 {
			listener.Handle(c.Prefix+"SessionStateChanged", c)
		}
	}

	return nil
}

func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) registerCoordinator(workers *river.Workers) errors.E {
	coordinatorsMu.Lock()
	defer coordinatorsMu.Unlock()

	sp := schemaPrefix{Schema: c.schema, Prefix: c.Prefix}

	_, ok := coordinators[sp]
	if ok {
		errE := errors.New("coordinator already registered")
		errors.Details(errE)["schema"] = c.schema
		errors.Details(errE)["prefix"] = c.Prefix
		return errE
	}

	if len(coordinators) == 0 {
		// We register the worker if this is the first coordinator.
		err := river.AddWorkerSafely(workers, &worker{})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	coordinators[sp] = c

	return nil
}

// Begin starts a new session.
//
// The session has to be explicitly ended by calling End.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) Begin(
	ctx context.Context, metadata BeginMetadata) (identifier.Identifier, errors.E,
) {
	session := identifier.New()
	arguments := []any{
		session.String(), metadata,
	}
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+c.Prefix+`Sessions" VALUES ($1, $2, NULL, NULL)`, arguments...)
		return internal.WithPgxError(err)
	})
	if errE != nil {
		return identifier.Identifier{}, errE
	}
	return session, nil
}

// End ends the session.
//
// Once the session has ended no more operations can be appended to it.
//
// After the session ends, the coordinator runs the `CompleteSession` function.
// After `CompleteSession` successfully completes, the session is considered
// completed and all operations associated with the session are deleted.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) End(
	ctx context.Context, session identifier.Identifier, metadata EndMetadata,
) errors.E {
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "`+c.Prefix+`EndSession"($1, $2)`, session.String(), metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeAlreadyEnded:
					return errors.WrapWith(errE, ErrAlreadyEnded)
				}
			}
			return errE
		}

		// We submit a job to the worker to call CompleteSession and complete the session.
		_, err = c.riverClient.InsertTx(ctx, tx, jobArgs{
			Schema:  c.schema,
			Prefix:  c.Prefix,
			Session: session,
		}, nil)
		return errors.WithStack(err)
	})

	if errE != nil {
		errors.Details(errE)["session"] = session.String()
	}
	return errE
}

// runCompleteSession runs the CompleteSession and if successfully runs, it completes the session.
//
// It deletes all operations associated with the session and marks the session as completed.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) runCompleteSession(
	ctx context.Context, session identifier.Identifier, job *river.Job[jobArgs],
) errors.E {
	metadata, errE := c.CompleteSession(ctx, session)
	if errE != nil {
		return errE
	}

	errE = internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "`+c.Prefix+`CompleteSession"($1, $2)`, session.String(), metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeNotEnded:
					return errors.WrapWith(errE, ErrNotEnded)
				case errorCodeAlreadyCompleted:
					return errors.WrapWith(errE, ErrAlreadyCompleted)
				}
			}
			return errE
		}

		// We mark the job as completed inside a transaction.
		_, err = river.JobCompleteTx[*riverpgxv5.Driver](ctx, tx, job)
		return errors.WithStack(err)
	})

	if errE != nil {
		errors.Details(errE)["session"] = session.String()
	}
	return errE
}

// Append appends a new operation into the log with the next available operation number.
//
// Data is optional and can be nil.
//
// Optional expected operation number can be provided in which case the next available
// operation number has to match the provided number for the call to succeed.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) Append(
	ctx context.Context, session identifier.Identifier, data Data, metadata OperationMetadata,
	expectedOperation *int64,
) (int64, errors.E) {
	arguments := []any{
		session.String(), metadata, data, expectedOperation,
	}
	var operation int64
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		operation = 0

		err := tx.QueryRow(ctx, `SELECT "`+c.Prefix+`AppendOperation"($1, $2, $3, $4)`, arguments...).Scan(&operation)
		if err != nil {
			errE := internal.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeAlreadyEnded:
					return errors.WrapWith(errE, ErrAlreadyEnded)
				case errorCodeConflict:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
		}
		return nil
	})

	if errE != nil {
		errors.Details(errE)["session"] = session.String()
	}
	return operation, errE
}

// List returns up to MaxPageLength operation numbers appended to the session, in decreasing order
// (newest operations first), before optional operation number, to support keyset pagination.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) List(
	ctx context.Context, session identifier.Identifier, before *int64,
) ([]int64, errors.E) {
	arguments := []any{
		session.String(),
	}
	beforeCondition := ""
	if before != nil {
		arguments = append(arguments, *before)
		// We want to make sure that before operation really exists.
		beforeCondition = `AND EXISTS (SELECT 1 FROM "` + c.Prefix + `Operations" WHERE "session"=$1 AND "operation"=$2) AND "operation"<$2`
	}
	var operations []int64
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		operations = make([]int64, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			SELECT "operation" FROM "`+c.Prefix+`Operations"
				WHERE "session"=$1
				`+beforeCondition+`
				-- We order by "operation" to enable keyset pagination.
				ORDER BY "operation" DESC
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var o int64
		_, err = pgx.ForEachRow(rows, []any{&o}, func() error {
			operations = append(operations, o)
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(operations) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var sessionCompleted bool
			err = tx.QueryRow(ctx, `SELECT "completeMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionCompleted)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.WithStack(ErrSessionNotFound)
				}
				return internal.WithPgxError(err)
			} else if sessionCompleted {
				return errors.WithStack(ErrAlreadyCompleted)
			}
			if before != nil {
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+c.Prefix+`Operations" WHERE "session"=$1 AND "operation"=$2)`, arguments...).Scan(&exists)
				if err != nil {
					return internal.WithPgxError(err)
				} else if !exists {
					return errors.WithStack(ErrOperationNotFound)
				}
			}
			// There is nothing wrong with having no operations.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["session"] = session.String()
		if before != nil {
			details["before"] = *before
		}
	}
	return operations, errE
}

// GetData returns data and metadata for the operation from the session.
//
// Data might be nil if the operation does not contain data.
//
// Data and metadata are not available anymore once the session completes.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) GetData( //nolint:ireturn
	ctx context.Context, session identifier.Identifier, operation int64,
) (Data, OperationMetadata, errors.E) {
	arguments := []any{
		session.String(), operation,
	}
	var data Data
	var metadata OperationMetadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		data = *new(Data)
		metadata = *new(OperationMetadata)

		err := tx.QueryRow(ctx, `
			SELECT "data", "metadata"
				FROM "`+c.Prefix+`Operations"
				WHERE "session"=$1 AND "operation"=$2
		`, arguments...).Scan(&data, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var sessionCompleted bool
				err = tx.QueryRow(ctx, `SELECT "completeMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionCompleted)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return errors.WrapWith(errE, ErrSessionNotFound)
					}
					return errors.Join(errE, internal.WithPgxError(err))
				} else if sessionCompleted {
					return errors.WrapWith(errE, ErrAlreadyCompleted)
				}
				return errors.WrapWith(errE, ErrOperationNotFound)
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["session"] = session.String()
		details["operation"] = operation
	}
	return data, metadata, errE
}

// GetMetadata returns metadata for the operation from the session.
//
// Metadata is not available anymore once the session completes.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) GetMetadata( //nolint:ireturn
	ctx context.Context, session identifier.Identifier, operation int64,
) (OperationMetadata, errors.E) {
	arguments := []any{
		session.String(), operation,
	}
	var metadata OperationMetadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		metadata = *new(OperationMetadata)

		err := tx.QueryRow(ctx, `
			SELECT "metadata"
				FROM "`+c.Prefix+`Operations"
				WHERE "session"=$1 AND "operation"=$2
		`, arguments...).Scan(&metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var sessionCompleted bool
				err = tx.QueryRow(ctx, `SELECT "completeMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionCompleted)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return errors.WrapWith(errE, ErrSessionNotFound)
					}
					return errors.Join(errE, internal.WithPgxError(err))
				} else if sessionCompleted {
					return errors.WrapWith(errE, ErrAlreadyCompleted)
				}
				return errors.WrapWith(errE, ErrOperationNotFound)
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["session"] = session.String()
		details["operation"] = operation
	}
	return metadata, errE
}

// Get returns initial, ending, and completed (once session has ended and/or completed, respectively, otherwise nil)
// metadata for the session.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) Get( //nolint:ireturn
	ctx context.Context, session identifier.Identifier,
) (BeginMetadata, EndMetadata, CompleteMetadata, errors.E) {
	arguments := []any{
		session.String(),
	}
	var beginMetadata BeginMetadata
	var endMetadata EndMetadata
	var completeMetadata CompleteMetadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		beginMetadata = *new(BeginMetadata)
		endMetadata = *new(EndMetadata)
		completeMetadata = *new(CompleteMetadata)

		err := tx.QueryRow(ctx, `
			SELECT "beginMetadata", "endMetadata", "completeMetadata"
				FROM "`+c.Prefix+`Sessions"
				WHERE "session"=$1
		`, arguments...).Scan(&beginMetadata, &endMetadata, &completeMetadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.WrapWith(errE, ErrSessionNotFound)
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["session"] = session.String()
	}
	return beginMetadata, endMetadata, completeMetadata, errE
}

// HandleNotification implements pgxlisten.Handler interface.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) HandleNotification(
	ctx context.Context, notification *pgconn.Notification, conn *pgx.Conn,
) error {
	switch notification.Channel {
	case c.Prefix + "OperationAppended":
		return c.handleOperationAppended(ctx, notification, conn)
	case c.Prefix + "SessionStateChanged":
		return c.handleSessionStateChanged(ctx, notification, conn)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = notification.Channel
		return errE
	}
}

// HandleBacklog implements pgxlisten.BacklogHandler interface.
//
// It recreates channels to signal to their consumers that notifications might have been
// missed and that they should take corrective actions, if possible.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) HandleBacklog(
	_ context.Context, channel string, _ *pgx.Conn,
) error {
	switch channel {
	case c.Prefix + "OperationAppended":
		// AppendedSize should be >= 0 here unless it was changed after initialization which is not allowed.
		c.appended = c.Appended.Recreate(c.AppendedSize)
	case c.Prefix + "SessionStateChanged":
		// ChangedSize should be >= 0 here unless it was changed after initialization which is not allowed.
		c.changed = c.Changed.Recreate(c.ChangedSize)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
	return nil
}

// handleOperationAppended handles OperationAppended notifications and forwards
// the operation to the Appended channel.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) handleOperationAppended(
	ctx context.Context, notification *pgconn.Notification, _ *pgx.Conn,
) error {
	var payload OperationAppended
	errE := x.UnmarshalWithoutUnknownFields([]byte(notification.Payload), &payload)
	if errE != nil {
		return errE
	}
	select {
	case c.appended <- payload:
	case <-ctx.Done():
	}
	return nil
}

// handleSessionStateChanged handles SessionStateChanged notifications and forwards
// the session state change to the Changed channel.
func (c *Coordinator[Data, OperationMetadata, BeginMetadata, EndMetadata, CompleteMetadata]) handleSessionStateChanged(
	ctx context.Context, notification *pgconn.Notification, _ *pgx.Conn,
) error {
	var payload SessionStateChanged
	errE := x.UnmarshalWithoutUnknownFields([]byte(notification.Payload), &payload)
	if errE != nil {
		return errE
	}
	select {
	case c.changed <- payload:
	case <-ctx.Done():
	}
	return nil
}
