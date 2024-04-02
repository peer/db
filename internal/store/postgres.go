package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

const (
	idleInTransactionSessionTimeout = 10 * time.Second
	statementTimeout                = 10 * time.Second

	defaultApplicationName = "peerdb"
)

// Standard error codes.
// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	ErrorCodeUniqueViolation      = "23505"
	ErrorCodeDuplicateSchema      = "42P06"
	ErrorCodeSerializationFailure = "40001"
	ErrorCodeDeadlockDetected     = "40P01"
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

func InitPostgres(ctx context.Context, databaseURI string, logger zerolog.Logger, getRequest func(context.Context) (string, string)) (*pgxpool.Pool, errors.E) {
	dbconfig, err := pgxpool.ParseConfig(databaseURI)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dbconfig.ConnConfig.Config.OnNotice = func(conn *pgconn.PgConn, notice *pgconn.Notice) {
		// TODO: Use SeverityUnlocalized instead of Severity.
		//       See: https://github.com/jackc/pgx/issues/1971
		l := logger.
			WithLevel(noticeSeverityToLogLevel[notice.Severity]).
			Fields(ErrorDetails((*pgconn.PgError)(notice))).
			Bool("postgres", true)
		applicationName := conn.ParameterStatus("application_name")
		if applicationName != defaultApplicationName {
			schema, request, ok := lastCut(applicationName, "/")
			if ok {
				l = l.Str("schema", schema).Str("request", request)
			}
		}
		l.Send()
	}
	dbconfig.ConnConfig.RuntimeParams["application_name"] = defaultApplicationName
	dbconfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(idleInTransactionSessionTimeout.Milliseconds(), 10)
	dbconfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)

	conn, err := pgx.ConnectConfig(ctx, dbconfig.ConnConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close(ctx)

	var maxConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW max_connections`).Scan(&maxConnectionsStr)
	if err != nil {
		return nil, WithPgxError(err)
	}
	maxConnections, err := strconv.Atoi(maxConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var reservedConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW reserved_connections`).Scan(&reservedConnectionsStr)
	if err != nil {
		return nil, WithPgxError(err)
	}
	reservedConnections, err := strconv.Atoi(reservedConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var superuserReservedConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW superuser_reserved_connections`).Scan(&superuserReservedConnectionsStr)
	if err != nil {
		return nil, WithPgxError(err)
	}
	superuserReservedConnections, err := strconv.Atoi(superuserReservedConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dbconfig.MaxConns = int32(maxConnections - reservedConnections - superuserReservedConnections)

	logger.Info().
		Str("serverVersion", conn.PgConn().ParameterStatus("server_version")).
		Str("serverEncoding", conn.PgConn().ParameterStatus("server_encoding")).
		Str("clientEncoding", conn.PgConn().ParameterStatus("client_encoding")).
		Str("sessionAuthorization", conn.PgConn().ParameterStatus("session_authorization")).
		Msg("database connection successful")

	dbconfig.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		schema, requestID := getRequest(ctx)

		_, err := conn.Exec(ctx, fmt.Sprintf(`SET application_name TO '%s/%s'`, schema, requestID)) //nolint:govet
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(WithPgxError(err)).Msg(`unable to set "application_name" for PostgreSQL connection`)
			return false
		}

		_, err = conn.Exec(ctx, fmt.Sprintf(`SET search_path TO "%s"`, schema))
		if err != nil {
			zerolog.Ctx(ctx).Err(WithPgxError(err)).Msg(`unable to set "search_path" for PostgreSQL connection`)
			return false
		}

		return true
	}
	dbconfig.AfterRelease = func(conn *pgx.Conn) bool {
		_, err := conn.Exec(ctx, `RESET application_name`) //nolint:govet
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
		return nil, errors.WithStack(err)
	}
	context.AfterFunc(ctx, dbpool.Close)

	return dbpool, nil
}
