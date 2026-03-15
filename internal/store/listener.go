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

// Handler interface extends pgxlisten.Handler and pgxlisten.BacklogHandler with HandlingReady method.
type Handler interface {
	pgxlisten.Handler
	pgxlisten.BacklogHandler

	// HandlingReady blocks until the listener is ready to handle requests.
	HandlingReady(ctx context.Context, channel string) errors.E
}

// Listener is a wrapper around pgxlisten.Listener that implements a background goroutine.
type Listener struct {
	*pgxlisten.Listener

	handlers map[string]Handler
	started  bool
}

// NewListener creates a Listener configured to acquire connections from the pool.
//
// Register handlers with listener.Handle before calling Start.
func NewListener(dbpool *pgxpool.Pool) *Listener {
	return &Listener{
		Listener: &pgxlisten.Listener{
			Connect: func(ctx context.Context) (*pgx.Conn, error) {
				// TODO: Measure how many re-connections have to be made to the database and abort if it is too much.
				//       The goal is that if this is happening too often, we should terminate the whole process and let the
				//       process supervisor decide what to do about instability of connections (it is probably not a local thing).
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
		},
		handlers: nil,
		started:  false,
	}
}

// Start starts listener in a background goroutine.
//
// It blocks until the listener successfully listens.
//
// All handlers must be registered on the listener before calling Start.
func (l *Listener) Start(ctx context.Context) errors.E {
	if l.started {
		return errors.New("already started")
	}
	l.started = true

	go func() {
		err := l.Listen(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			// We are stopping.
			return
		}
		// TODO: We should terminate the whole process and let the process supervisor decide what to do.
		zerolog.Ctx(ctx).Error().Err(err).Msg("NOTIFY listener stopped unexpectedly")
	}()

	// We wait for all handlers to be ready.
	for channel, handler := range l.handlers {
		errE := handler.HandlingReady(ctx, channel)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// Handle registers a handler on the listener.
func (l *Listener) Handle(channel string, handler Handler) {
	l.Listener.Handle(channel, handler)

	// We maintain a copy of handlers because l.Listener does not expose its.
	if l.handlers == nil {
		l.handlers = make(map[string]Handler)
	}

	l.handlers[channel] = handler
}
