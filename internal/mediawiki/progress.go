package mediawiki

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

type CountingReader struct {
	Reader io.Reader
	count  int64
}

func (c *CountingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	atomic.AddInt64(&c.count, int64(n))
	return n, err
}

func (c *CountingReader) Count() int64 {
	return c.count
}

type Counter interface {
	Count() int64
}

type Progress struct {
	Count     int64
	Size      int64
	Started   time.Time
	Current   time.Time
	Elapsed   time.Duration
	remaining time.Duration
	estimated time.Time
}

func (p Progress) Percent() float64 {
	return float64(p.Count) / float64(p.Size) * 100.0 //nolint:gomnd
}

func (p Progress) Remaining() time.Duration {
	return p.remaining
}

func (p Progress) Estimated() time.Time {
	return p.estimated
}

type Ticker struct {
	C    <-chan Progress
	stop func()
}

func (t *Ticker) Stop() {
	t.stop()
}

func NewTicker(ctx context.Context, counter Counter, size int64, interval time.Duration) *Ticker {
	ctx, cancel := context.WithCancel(ctx)
	started := time.Now()
	output := make(chan Progress)
	ticker := time.NewTicker(interval)
	go func() {
		defer cancel()
		defer close(output)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				progress := Progress{ //nolint:exhaustivestruct
					Count:   counter.Count(),
					Size:    size,
					Started: started,
					Current: now,
					Elapsed: now.Sub(started),
				}
				if progress.Count > 0 {
					ratio := float64(progress.Count) / float64(size)
					elapsed := float64(progress.Elapsed)
					total := time.Duration(elapsed / ratio)
					progress.estimated = started.Add(total)
					progress.remaining = progress.estimated.Sub(now)
				}
				if ctx.Err() != nil {
					return
				}
				output <- progress
			}
		}
	}()
	return &Ticker{
		C:    output,
		stop: cancel,
	}
}
