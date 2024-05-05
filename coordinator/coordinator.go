package coordinator

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	MaxPageLength    = 5000
	maxPageLengthStr = "5000"
)

const (
	// Our PostgreSQL error codes.
	errorCodeSessionNotFound = "P1020"
	errorCodeAlreadyEnded    = "P1021"
)

// AppendedOperation represents an operation appended to a session.
type AppendedOperation struct {
	Session   identifier.Identifier
	Operation int64
}

// Coordinator provides an append-only log of operations to support
// synchronizing real-time collaboration sessions.
//
// For every operation, its metadata and optional data are stored.
// Go types for them you configure with type parameters.
type Coordinator[Data, Metadata any] struct {
	// Prefix to use when initializing PostgreSQL objects used by this coordinator.
	Prefix string

	// PostgreSQL column types to store data and metadata.
	// It should probably be one of the jsonb, bytea, or text.
	// Go types used for Coordinator type parameters should be compatible with
	// column types chosen.
	DataType     string
	MetadataType string

	// EndCallback is called inside a transaction before all operations
	// for the session are deleted and session is ended.
	EndCallback func(ctx context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E)

	// A channel to which operations are send when they are appended.
	//
	// The order in which they are sent is not necessary the order in which
	// they were appended. You should not rely on the order.
	Appended chan<- AppendedOperation

	// A channel to which sessions are send when they end.
	//
	// The order in which they are sent is not necessary the order in which
	// they ended. You should not rely on the order.
	Ended chan<- identifier.Identifier

	dbpool *pgxpool.Pool
}

// Init initializes the Coordinator.
//
// It creates and configures the PostgreSQL tables, indices, and
// stored procedures if they do not already exist.
func (c *Coordinator[Data, Metadata]) Init(ctx context.Context, dbpool *pgxpool.Pool) errors.E {
	if c.dbpool != nil {
		return errors.New("already initialized")
	}

	// TODO: Use schema management/migration instead.
	errE := internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		//nolint:goconst
		_, err := tx.Exec(ctx, `
			CREATE TABLE "`+c.Prefix+`Sessions" (
				-- ID of the session.
				"session" text STORAGE PLAIN COLLATE "C" NOT NULL,
				"beginMetadata" `+c.MetadataType+` NOT NULL,
				"endMetadata" `+c.MetadataType+`,
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
					DELETE FROM "`+c.Prefix+`Operations" WHERE "session"=_session;
					UPDATE "`+c.Prefix+`Sessions" SET "endMetadata"=_metadata WHERE "session"=_session;
				END;
			$$;

			CREATE FUNCTION "`+c.Prefix+`PushOperation"(_session text, _metadata `+c.MetadataType+`, _data `+c.DataType+`)
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
						RETURNING "operation" INTO _operation;
					RETURN _operation;
				END;
			$$;

			CREATE FUNCTION "`+c.Prefix+`SetOperation"(_session text, _operation bigint, _metadata `+c.MetadataType+`, _data `+c.DataType+`)
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
					INSERT INTO "`+c.Prefix+`Operations" VALUES (_session, _operation, _data, _metadata);
				END;
			$$;
		`)
		if err != nil {
			return internal.WithPgxError(err)
		}

		return nil
	})
	if errE != nil {
		var pgError *pgconn.PgError
		if errors.As(errE, &pgError) {
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

	return nil
}

// Begin starts a new session.
//
// The session has to be explicitly ended by calling End.
func (c *Coordinator[Data, Metadata]) Begin(ctx context.Context, metadata Metadata) (identifier.Identifier, errors.E) {
	session := identifier.New()
	arguments := []any{
		session.String(), metadata,
	}
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `INSERT INTO "`+c.Prefix+`Sessions" VALUES ($1, $2, NULL)`, arguments...)
		return internal.WithPgxError(err)
	})
	if errE != nil {
		return identifier.Identifier{}, errE
	}
	return session, nil
}

// End ends the session.
//
// It deletes all operations associated with the session and marks the session as ended.
// Once the session has ended no more operations can be appended to it.
//
// Just before all operations are deleted, EndCallback is called inside a transaction.
func (c *Coordinator[Data, Metadata]) End(ctx context.Context, session identifier.Identifier, metadata Metadata) (Metadata, errors.E) { //nolint:ireturn
	var m Metadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		m = *new(Metadata)

		var errE errors.E
		if c.EndCallback != nil {
			m, errE = c.EndCallback(ctx, session, metadata)
			if errE != nil {
				return errE
			}
		} else {
			m = metadata
		}
		_, err := tx.Exec(ctx, `SELECT "`+c.Prefix+`EndSession"($1, $2)`, session.String(), m) //nolint:goconst
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeAlreadyEnded:
					return errors.WrapWith(errE, ErrAlreadyEnded)
				}
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		errors.Details(errE)["session"] = session.String()
	} else if c.Ended != nil {
		c.Ended <- session
	}
	return m, errE
}

// Push appends a new operation into the log with the next available operation number.
//
// Data is optional and can be nil.
func (c *Coordinator[Data, Metadata]) Push(ctx context.Context, session identifier.Identifier, data Data, metadata Metadata) (int64, errors.E) {
	arguments := []any{
		session.String(), metadata, data,
	}
	var operation int64
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		operation = 0

		err := tx.QueryRow(ctx, `SELECT "`+c.Prefix+`PushOperation"($1, $2, $3)`, arguments...).Scan(&operation)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeAlreadyEnded:
					return errors.WrapWith(errE, ErrAlreadyEnded)
				}
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		errors.Details(errE)["session"] = session.String()
	} else if c.Appended != nil {
		c.Appended <- AppendedOperation{
			Session:   session,
			Operation: operation,
		}
	}
	return operation, errE
}

// Set appends a new operation into the log with the provided operation number.
//
// The provided operation number has to be available for the call to succeed.
//
// Data is optional and can be nil.
func (c *Coordinator[Data, Metadata]) Set(ctx context.Context, session identifier.Identifier, operation int64, data Data, metadata Metadata) errors.E {
	arguments := []any{
		session.String(), operation, metadata, data,
	}
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		_, err := tx.Exec(ctx, `SELECT "`+c.Prefix+`SetOperation"($1, $2, $3, $4)`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code {
				case errorCodeSessionNotFound:
					return errors.WrapWith(errE, ErrSessionNotFound)
				case errorCodeAlreadyEnded:
					return errors.WrapWith(errE, ErrAlreadyEnded)
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
		}
		return nil
	})
	if errE != nil {
		errors.Details(errE)["session"] = session.String()
		errors.Details(errE)["operation"] = operation
	} else if c.Appended != nil {
		c.Appended <- AppendedOperation{
			Session:   session,
			Operation: operation,
		}
	}
	return errE
}

// List returns up to MaxPageLength operation numbers appended to the session, in decreasing order
// (newest operations first), before optional operation number, to support keyset pagination.
func (c *Coordinator[Data, Metadata]) List(ctx context.Context, session identifier.Identifier, before *int64) ([]int64, errors.E) {
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
			var sessionEnded bool
			err = tx.QueryRow(ctx, `SELECT "endMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionEnded) //nolint:goconst
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.WithStack(ErrSessionNotFound)
				}
				return internal.WithPgxError(err)
			} else if sessionEnded {
				return errors.WithStack(ErrAlreadyEnded)
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
// Data and metadata are not available anymore once the session ends.
func (c *Coordinator[Data, Metadata]) GetData(ctx context.Context, session identifier.Identifier, operation int64) (Data, Metadata, errors.E) { //nolint:ireturn
	arguments := []any{
		session.String(), operation,
	}
	var data Data
	var metadata Metadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		data = *new(Data)
		metadata = *new(Metadata)

		err := tx.QueryRow(ctx, `
			SELECT "data", "metadata"
				FROM "`+c.Prefix+`Operations"
				WHERE "session"=$1 AND "operation"=$2
		`, arguments...).Scan(&data, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var sessionEnded bool
				err = tx.QueryRow(ctx, `SELECT "endMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionEnded)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return errors.WrapWith(errE, ErrSessionNotFound)
					}
					return errors.Join(errE, internal.WithPgxError(err))
				} else if sessionEnded {
					return errors.WrapWith(errE, ErrAlreadyEnded)
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
// Metadata is not available anymore once the session ends.
func (c *Coordinator[Data, Metadata]) GetMetadata(ctx context.Context, session identifier.Identifier, operation int64) (Metadata, errors.E) { //nolint:ireturn
	arguments := []any{
		session.String(), operation,
	}
	var metadata Metadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		metadata = *new(Metadata)

		err := tx.QueryRow(ctx, `
			SELECT "metadata"
				FROM "`+c.Prefix+`Operations"
				WHERE "session"=$1 AND "operation"=$2
		`, arguments...).Scan(&metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var sessionEnded bool
				err = tx.QueryRow(ctx, `SELECT "endMetadata" IS NOT NULL FROM "`+c.Prefix+`Sessions" WHERE "session"=$1`, session.String()).Scan(&sessionEnded)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return errors.WrapWith(errE, ErrSessionNotFound)
					}
					return errors.Join(errE, internal.WithPgxError(err))
				} else if sessionEnded {
					return errors.WrapWith(errE, ErrAlreadyEnded)
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

// Get returns initial and ending (once session has ended, otherwise it is nil)
// metadata for the session.
func (c *Coordinator[Data, Metadata]) Get(ctx context.Context, session identifier.Identifier) (Metadata, Metadata, errors.E) { //nolint:ireturn
	arguments := []any{
		session.String(),
	}
	var beginMetadata Metadata
	var endMetadata Metadata
	errE := internal.RetryTransaction(ctx, c.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		beginMetadata = *new(Metadata)
		endMetadata = *new(Metadata)

		err := tx.QueryRow(ctx, `
			SELECT "beginMetadata", "endMetadata"
				FROM "`+c.Prefix+`Sessions"
				WHERE "session"=$1
		`, arguments...).Scan(&beginMetadata, &endMetadata)
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
	return beginMetadata, endMetadata, errE
}
