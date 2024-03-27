package peerdb

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	idleInTransactionSessionTimeout = 10 * time.Second
	statementTimeout                = 10 * time.Second
)

func (c *ServeCommand) initPostgres(ctx context.Context, globals *Globals) (*pgxpool.Pool, errors.E) {
	dbconfig, err := pgxpool.ParseConfig(string(globals.Database))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	conn, err := pgx.ConnectConfig(ctx, dbconfig.ConnConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer conn.Close(ctx)

	var maxConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW max_connections`).Scan(&maxConnectionsStr)
	if err != nil {
		return nil, internal.WithPgxError(err)
	}
	maxConnections, err := strconv.Atoi(maxConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var reservedConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW reserved_connections`).Scan(&reservedConnectionsStr)
	if err != nil {
		return nil, internal.WithPgxError(err)
	}
	reservedConnections, err := strconv.Atoi(reservedConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var superuserReservedConnectionsStr string
	err = conn.QueryRow(ctx, `SHOW superuser_reserved_connections`).Scan(&superuserReservedConnectionsStr)
	if err != nil {
		return nil, internal.WithPgxError(err)
	}
	superuserReservedConnections, err := strconv.Atoi(superuserReservedConnectionsStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	dbconfig.MaxConns = int32(maxConnections - reservedConnections - superuserReservedConnections)
	dbconfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(idleInTransactionSessionTimeout.Milliseconds(), 10)
	dbconfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)

	dbconfig.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		requestID := waf.MustRequestID(ctx)
		site := waf.MustGetSite[*Site](ctx)

		_, err := conn.Exec(ctx, `SET application_name TO $1`, fmt.Sprintf("%s/%s", site.Schema, requestID)) //nolint:govet
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(internal.WithPgxError(err)).Msg(`unable to set "application_name" for PostgreSQL connection`)
			return false
		}

		_, err = conn.Exec(ctx, `SET search_path TO $1`, site.Schema)
		if err != nil {
			zerolog.Ctx(ctx).Err(internal.WithPgxError(err)).Msg(`unable to set "search_path" for PostgreSQL connection`)
			return false
		}

		return true
	}
	dbconfig.AfterRelease = func(conn *pgx.Conn) bool {
		_, err := conn.Exec(ctx, `RESET application_name`) //nolint:govet
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(internal.WithPgxError(err)).Msg(`unable to reset "application_name" for PostgreSQL connection`)
			return false
		}

		_, err = conn.Exec(ctx, `RESET search_path`)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(internal.WithPgxError(err)).Msg(`unable to reset "search_path" for PostgreSQL connection`)
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
