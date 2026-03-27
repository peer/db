package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/semaphore"
)

const (
	idleInTransactionSessionTimeout = 10 * time.Second
	statementTimeout                = 10 * time.Second

	initialApplicationName = "peerdb"

	// Number of connections which are left unused by all pools together for every system.
	reservedConnections = 5
)

//nolint:gochecknoglobals
var (
	// connectionsCountMap is a map of a system identifier to a semaphore tracking number
	// of reserved connections across all pools against a particular system.
	connectionsCountMap = map[string]*semaphore.Weighted{}
	// maxConnectionsMap is a map of a system identifier to the maximum number of
	// connections for the system. This allows us to query this information only once
	// per system. It assumes the number of connections does not change during the lifetime
	// of this process (which maybe is not true, but in that case one should restart the process).
	maxConnectionsMap = map[string]int32{}
	connectionsMu     sync.RWMutex
	// We allow only one system connection (a connection used in InitPostgres) at any given time,
	// regardless of the target system.
	systemConnection sync.Mutex
)

// Standard error codes.
// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	ErrorCodeUniqueViolation      = "23505"
	ErrorCodeDuplicateSchema      = "42P06"
	ErrorCodeDuplicateTable       = "42P07"
	ErrorCodeDuplicateFunction    = "42723"
	ErrorCodeSerializationFailure = "40001"
	ErrorCodeDeadlockDetected     = "40P01"
	ErrorCodeExclusionViolation   = "23P01"
)

// See: https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-CLIENT-MIN-MESSAGES
// See: https://www.postgresql.org/docs/current/plpgsql-errors-and-messages.html
var noticeSeverityToLogLevel = map[string]zerolog.Level{ //nolint:gochecknoglobals
	"DEBUG":   zerolog.DebugLevel,
	"LOG":     zerolog.InfoLevel,
	"INFO":    zerolog.InfoLevel,
	"NOTICE":  zerolog.InfoLevel,
	"WARNING": zerolog.WarnLevel,
}

func getMaxConnections(systemIdentifier string) (int32, *semaphore.Weighted) {
	connectionsMu.RLock()
	defer connectionsMu.RUnlock()

	maxConnections, ok := maxConnectionsMap[systemIdentifier]
	if !ok {
		return 0, nil
	}
	return maxConnections, connectionsCountMap[systemIdentifier]
}

func setMaxConnections(systemIdentifier string, maxConnections int32) *semaphore.Weighted {
	connectionsMu.Lock()
	defer connectionsMu.Unlock()

	existingMaxConnections, ok := maxConnectionsMap[systemIdentifier]
	if ok {
		if existingMaxConnections != maxConnections {
			errE := errors.New("max connections is inconsistent")
			errors.Details(errE)["existing"] = existingMaxConnections
			errors.Details(errE)["new"] = maxConnections
			errors.Details(errE)["systemIdentifier"] = systemIdentifier
			panic(errE)
		}
		return connectionsCountMap[systemIdentifier]
	}

	// We subtract reservedConnections from max connections because we want to keep some slots always available
	// to others. One of those reserved connections we use for the system connection in InitPostgres.
	connectionsCountMap[systemIdentifier] = semaphore.NewWeighted(int64(maxConnections) - reservedConnections)
	maxConnectionsMap[systemIdentifier] = maxConnections
	return connectionsCountMap[systemIdentifier]
}

// InitPostgres initializes and configures a PostgreSQL connection pool with the specified settings.
//
// It returns the pool and a cleanup function that closes the pool and releases the reserved connections.
// The caller must call the cleanup function when the pool is no longer needed.
// Do not call just dbpool.Close.
func InitPostgres(ctx context.Context, databaseURI string, logger zerolog.Logger, getRequest func(context.Context) (string, string)) (*pgxpool.Pool, func(), errors.E) {
	dbconfig, err := pgxpool.ParseConfig(strings.TrimSpace(databaseURI))
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	dbconfig.ConnConfig.OnNotice = func(conn *pgconn.PgConn, notice *pgconn.Notice) {
		l := logger.
			WithLevel(noticeSeverityToLogLevel[notice.SeverityUnlocalized]).
			Fields(ErrorDetails((*pgconn.PgError)(notice))).
			Bool("postgres", true)
		schema, ok := conn.CustomData()["schema"].(string)
		if ok && schema != "" {
			l = l.Str("schema", schema)
		}
		request, ok := conn.CustomData()["request"].(string)
		if ok && request != "" {
			l = l.Str("request", request)
		}
		l.Send()
	}
	dbconfig.AfterConnect = func(_ context.Context, c *pgx.Conn) error {
		c.TypeMap().RegisterType(&pgtype.Type{
			Name: "json", OID: pgtype.JSONOID, Codec: &pgtype.JSONCodec{
				Marshal: func(v any) ([]byte, error) {
					return x.MarshalWithoutEscapeHTML(v)
				},
				Unmarshal: func(data []byte, v any) error {
					return x.UnmarshalWithoutUnknownFields(data, v)
				},
			},
		})
		c.TypeMap().RegisterType(&pgtype.Type{
			Name: "jsonb", OID: pgtype.JSONBOID, Codec: &pgtype.JSONBCodec{
				Marshal: func(v any) ([]byte, error) {
					return x.MarshalWithoutEscapeHTML(v)
				},
				Unmarshal: func(data []byte, v any) error {
					return x.UnmarshalWithoutUnknownFields(data, v)
				},
			},
		})
		return nil
	}
	dbconfig.ConnConfig.RuntimeParams["application_name"] = initialApplicationName
	dbconfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(idleInTransactionSessionTimeout.Milliseconds(), 10)
	dbconfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)

	systemConnection.Lock()
	defer systemConnection.Unlock()

	conn, err := pgx.ConnectConfig(ctx, dbconfig.ConnConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	defer conn.Close(ctx) //nolint:errcheck

	var systemIdentifier string
	err = conn.QueryRow(ctx, `SELECT system_identifier FROM pg_control_system()`).Scan(&systemIdentifier)
	if err != nil {
		return nil, nil, WithPgxError(err)
	}

	maxConnectionsTotal, connectionsCount := getMaxConnections(systemIdentifier)
	if maxConnectionsTotal == 0 {
		var maxConnectionsStr string
		err = conn.QueryRow(ctx, `SHOW max_connections`).Scan(&maxConnectionsStr)
		if err != nil {
			return nil, nil, WithPgxError(err)
		}
		maxConnections, err := strconv.Atoi(maxConnectionsStr)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		var reservedConnectionsStr string
		err = conn.QueryRow(ctx, `SHOW reserved_connections`).Scan(&reservedConnectionsStr)
		if err != nil {
			return nil, nil, WithPgxError(err)
		}
		reservedConnections, err := strconv.Atoi(reservedConnectionsStr)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		var superuserReservedConnectionsStr string
		err = conn.QueryRow(ctx, `SHOW superuser_reserved_connections`).Scan(&superuserReservedConnectionsStr)
		if err != nil {
			return nil, nil, WithPgxError(err)
		}
		superuserReservedConnections, err := strconv.Atoi(superuserReservedConnectionsStr)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		maxConnectionsTotal = int32(maxConnections - reservedConnections - superuserReservedConnections) //nolint:gosec
		connectionsCount = setMaxConnections(systemIdentifier, maxConnectionsTotal)
	}

	// Allow overriding the maximum number of pool connections via context.
	if maxConns, _ := ctx.Value(maxDBPoolConnectionsContextKey).(int32); maxConns > 0 {
		dbconfig.MaxConns = maxConns
	} else {
		dbconfig.MaxConns = maxConnectionsTotal
	}

	err = connectionsCount.Acquire(ctx, int64(dbconfig.MaxConns))
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	connectionsPendingRelease := true
	defer func() {
		if connectionsPendingRelease {
			connectionsCount.Release(int64(dbconfig.MaxConns))
		}
	}()

	logger.Info().
		Str("serverVersion", conn.PgConn().ParameterStatus("server_version")).
		Str("serverEncoding", conn.PgConn().ParameterStatus("server_encoding")).
		Str("clientEncoding", conn.PgConn().ParameterStatus("client_encoding")).
		Str("sessionAuthorization", conn.PgConn().ParameterStatus("session_authorization")).
		Int32("maxConnections", dbconfig.MaxConns).
		Msg("database connection successful")

	dbconfig.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		schema, requestID := getRequest(ctx)

		if schema == "" {
			return false, errors.New("schema is not set")
		}

		_, err := conn.Exec(ctx, fmt.Sprintf(`SET application_name TO '%s/%s/%s'`, initialApplicationName, schema, requestID))
		if err != nil {
			return false, errors.WithMessage(WithPgxError(err), "unable to set \"application_name\" for PostgreSQL connection")
		}

		_, err = conn.Exec(ctx, fmt.Sprintf(`SET search_path TO "%s"`, schema))
		if err != nil {
			return false, errors.WithMessage(WithPgxError(err), "unable to set \"search_path\" for PostgreSQL connection")
		}

		conn.PgConn().CustomData()["schema"] = schema
		conn.PgConn().CustomData()["request"] = requestID

		return true, nil
	}
	dbconfig.AfterRelease = func(conn *pgx.Conn) bool {
		delete(conn.PgConn().CustomData(), "schema")
		delete(conn.PgConn().CustomData(), "request")

		_, err := conn.Exec(ctx, `RESET application_name`)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(WithPgxError(err)).Msg(`unable to reset "application_name" for PostgreSQL connection`)
			return false
		}

		_, err = conn.Exec(ctx, `RESET search_path`)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(WithPgxError(err)).Msg(`unable to reset "search_path" for PostgreSQL connection`)
			return false
		}

		return true
	}

	dbpool, err := pgxpool.NewWithConfig(ctx, dbconfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	cleanup := func() {
		dbpool.Close()
		connectionsCount.Release(int64(dbconfig.MaxConns))
	}
	connectionsPendingRelease = false

	return dbpool, cleanup, nil
}

// EnsureSchema creates a database schema if it doesn't exist, ignoring duplicate errors.
func EnsureSchema(ctx context.Context, tx pgx.Tx, schema string) errors.E {
	// TODO: Could we just use "CREATE SCHEMA IF NOT EXISTS" here?
	//       See: https://stackoverflow.com/questions/29900845/create-schema-if-not-exists-raises-duplicate-key-error
	_, err := tx.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	if err != nil {
		if pgError, ok := errors.AsType[*pgconn.PgError](err); ok {
			switch pgError.Code {
			case ErrorCodeUniqueViolation:
				return nil
			case ErrorCodeDuplicateSchema:
				return nil
			}
		}
		return WithPgxError(err)
	}
	return nil
}
