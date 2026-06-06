package store //nolint:testpackage

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/go/errors"
	z "gitlab.com/tozd/go/zerolog"
)

// TestJobLoggingMiddleware verifies that the per-job context logger buffers the job's debug logs and
// only flushes them when the job fails or panics, while leaving successful jobs and River's control
// sentinels (snooze, cancel without a wrapped error) quiet.
func TestJobLoggingMiddleware(t *testing.T) {
	t.Parallel()

	// newWithContext builds a z.WithContextFunc backed by a real TriggerLevelWriter writing into buf,
	// with the production defaults (buffer debug, trigger on error).
	newWithContext := func(buf *bytes.Buffer) z.WithContextFunc {
		return func(ctx context.Context) (context.Context, func(), func()) {
			w := &zerolog.TriggerLevelWriter{
				Writer:           zerolog.LevelWriterAdapter{Writer: buf},
				ConditionalLevel: zerolog.DebugLevel,
				TriggerLevel:     zerolog.ErrorLevel,
			}
			logger := zerolog.New(w).Level(zerolog.DebugLevel)
			return logger.WithContext(ctx), func() { _ = w.Close() }, func() { _ = w.Trigger() }
		}
	}

	for _, tc := range []struct {
		name    string
		err     error
		panics  bool
		flushed bool
	}{
		{"success", nil, false, false},
		{"failure", errors.New("boom"), false, true},
		{"snooze", river.JobSnooze(time.Second), false, false},
		{"cancelNil", river.JobCancel(nil), false, false},
		{"cancelErr", river.JobCancel(errors.New("reason")), false, true},
		{"panic", nil, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			m := &jobLoggingMiddleware{MiddlewareDefaults: river.MiddlewareDefaults{}, WithContext: newWithContext(&buf)}
			doInner := func(ctx context.Context) error {
				zerolog.Ctx(ctx).Debug().Msg("buffered-debug")
				if tc.panics {
					panic("worker panic")
				}
				return tc.err
			}

			work := func() error {
				return m.Work(t.Context(), &rivertype.JobRow{}, doInner)
			}
			if tc.panics {
				assert.Panics(t, func() { _ = work() })
			} else {
				assert.Equal(t, tc.err, work())
			}

			if tc.flushed {
				assert.Contains(t, buf.String(), "buffered-debug", "buffered debug should be flushed")
			} else {
				assert.NotContains(t, buf.String(), "buffered-debug", "buffered debug should be discarded")
			}
		})
	}
}
