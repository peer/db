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

	// conn is the current pool connection used by the listener.
	// It is set in Connect and released in Connect (on reconnect)
	// and after Listen returns (for final cleanup).
	conn *pgxpool.Conn
}

// NewListener creates a Listener configured to acquire connections from the pool.
//
// Register handlers with listener.Handle before calling Start.
func NewListener(dbpool *pgxpool.Pool) *Listener {
	l := &Listener{
		Listener: nil,
		handlers: nil,
		started:  false,
		conn:     nil,
	}
	l.Listener = &pgxlisten.Listener{
		Connect: func(ctx context.Context) (*pgx.Conn, error) {
			// TODO: Measure how many re-connections have to be made to the database and abort if it is too much.
			//       The goal is that if this is happening too often, we should terminate the whole process and let the
			//       process supervisor decide what to do about instability of connections (it is probably not a local thing).

			// Release previous connection on reconnect. pgxlisten has already closed the underlying
			// connection, so Release will notice it is dead and destroy it for the pool to recreate it.
			l.releaseConn()

			var err error
			l.conn, err = dbpool.Acquire(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			// We do not hijack the connection because we want to prevent the pool from making a new connection
			// while this one is in use, so that the max connections limit of the pool is also the limit on number
			// of total connections we are making against the database.
			return l.conn.Conn(), nil
		},
		LogError: func(ctx context.Context, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			zerolog.Ctx(ctx).Error().Err(err).Msg("NOTIFY listener error")
		},
		ReconnectDelay: listenerReconnectDelay,
	}
	return l
}

// releaseConn releases the current pool connection, if any.
func (l *Listener) releaseConn() {
	if l.conn != nil {
		l.conn.Release()
		l.conn = nil
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
		defer l.releaseConn()

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
