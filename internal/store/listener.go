package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgxlisten"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

const listenerReconnectDelay = 5 * time.Second

// NewListener creates a pgxlisten.Listener configured to acquire connections from the pool.
//
// Register handlers with listener.Handle before calling StartListener.
func NewListener(dbpool *pgxpool.Pool) *pgxlisten.Listener {
	return &pgxlisten.Listener{
		Connect: func(ctx context.Context) (*pgx.Conn, error) {
			conn, err := dbpool.Acquire(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			// Hijack detaches the connection from the pool so pgxlisten manages its lifetime.
			return conn.Hijack(), nil
		},
		LogError: func(ctx context.Context, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			zerolog.Ctx(ctx).Error().Err(err).Msg("NOTIFY listener error")
		},
		ReconnectDelay: listenerReconnectDelay,
	}
}

// StartListener starts listener in a background goroutine.
//
// All handlers must be registered on the listener before calling StartListener.
func StartListener(ctx context.Context, listener *pgxlisten.Listener) {
	go func() {
		err := listener.Listen(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		zerolog.Ctx(ctx).Error().Err(err).Msg("NOTIFY listener stopped unexpectedly")
	}()
}
