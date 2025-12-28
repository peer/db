// Package indexer provides utilities for downloading and indexing web content.
package indexer

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/cockroachdb/field-eng-powertools/notify"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"golang.org/x/sync/errgroup"
)

type downloadingReader struct {
	Path      string
	URL       string
	WriteFile *os.File
	ReadFile  *os.File

	read       int64
	size       int64
	downloaded *notify.Var[int64]
	ctx        context.Context //nolint:containedctx
	g          *errgroup.Group
	cancel     context.CancelFunc
	ticker     *x.Ticker
}

func (r *downloadingReader) Read(p []byte) (int, error) {
	downloaded, updated := r.downloaded.Get()
	for {
		if r.size == downloaded {
			// Once the file is fully downloaded, we can just call Read on the underlying file.
			return r.ReadFile.Read(p) //nolint:wrapcheck
		}

		if r.read < downloaded {
			// There should be something to read.
			nr, err := r.ReadFile.Read(p)
			r.read += int64(nr)
			if err == io.EOF {
				if nr > 0 {
					return nr, nil
				}
				// See: https://github.com/golang/go/issues/39155
				return nr, err //nolint:wrapcheck
			}
			return nr, errors.WithStack(err)
		}

		// We wait for more data to be downloaded.
		select {
		case <-updated:
			downloaded, updated = r.downloaded.Get()
		case <-r.ctx.Done():
			err := r.ctx.Err()
			if err == io.EOF { //nolint:errorlint
				// See: https://github.com/golang/go/issues/39155
				return 0, err //nolint:wrapcheck
			}
			return 0, errors.WithStack(err)
		}
	}
}

func (r *downloadingReader) Close() error {
	defer func() {
		if r.WriteFile != nil {
			r.WriteFile.Close() //nolint:errcheck,gosec
		}
		if r.ReadFile != nil {
			r.ReadFile.Close() //nolint:errcheck,gosec
		}
		if r.ticker != nil {
			r.ticker.Stop()
		}
	}()
	if r.g != nil {
		r.cancel()
		return r.g.Wait() //nolint:wrapcheck
	}
	return nil
}

func (r *downloadingReader) Start(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger) (int64, errors.E) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.g, r.ctx = errgroup.WithContext(ctx)

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		r.WriteFile.Close() //nolint:errcheck,gosec
		r.ReadFile.Close()  //nolint:errcheck,gosec
		_ = os.Remove(r.Path)
		return 0, errors.WithStack(err)
	}
	httpResponseReader, errE := x.NewRetryableResponse(httpClient, req)
	if errE != nil {
		r.WriteFile.Close() //nolint:errcheck,gosec
		r.ReadFile.Close()  //nolint:errcheck,gosec
		_ = os.Remove(r.Path)
		return 0, errE
	}
	r.size = httpResponseReader.Size()

	countingReader := &x.CountingReader{Reader: httpResponseReader}
	r.ticker = x.NewTicker(ctx, countingReader, x.NewCounter(r.size), ProgressPrintRate)
	go func() {
		for p := range r.ticker.C {
			logger.Info().
				Int64("count", p.Count).
				Int64("total", r.size).
				// We format it ourselves. See: https://github.com/rs/zerolog/issues/709
				Str("eta", p.Remaining().Truncate(time.Second).String()).
				Float64("%", p.Percent()).
				Str("url", r.URL).
				Msg("downloading")
		}
	}()

	r.g.Go(func() error {
		defer func() {
			info, err := os.Stat(r.Path)
			if err != nil || r.size != info.Size() {
				// Incomplete file. Delete.
				_ = os.Remove(r.Path)
			}
		}()
		defer r.WriteFile.Close() //nolint:errcheck
		defer r.ticker.Stop()
		defer httpResponseReader.Close() //nolint:errcheck

		var written int64
		var errE errors.E
		buf := make([]byte, 32*1024) //nolint:mnd

		for {
			if ctx.Err() != nil {
				errE = errors.WithStack(ctx.Err())
				break
			}

			nr, er := countingReader.Read(buf)
			if nr > 0 {
				nw, ew := r.WriteFile.Write(buf[0:nr])
				if nw < 0 || nr < nw {
					nw = 0
					if ew == nil {
						ew = errors.New("invalid write result")
					}
				}
				written += int64(nw)
				r.downloaded.Set(written)
				if ew != nil {
					errE = errors.WithStack(ew)
					break
				}
				if nr != nw {
					errE = errors.New("short write")
					break
				}
			}
			if er != nil {
				if !errors.Is(er, io.EOF) {
					errE = errors.WithStack(er)
				}
				break
			}
		}

		if errE != nil {
			logger.Error().Str("url", r.URL).
				Err(errE).
				Msg("error downloading")
			return errE
		}

		logger.Info().Str("url", r.URL).
			Int64("count", written).
			Int64("total", r.size).
			Msg("downloading done")

		return nil
	})

	return r.size, nil
}

func getPathAndURL(cacheDir, url string) (string, string) {
	_ = os.MkdirAll(cacheDir, 0o755) //nolint:mnd,gosec
	_, err := os.Stat(url)
	if os.IsNotExist(err) {
		// TODO: Do something better and more secure for the filename (escape path from the URL, use query string, etc.).
		return filepath.Join(cacheDir, path.Base(url)), url
	}
	return url, url
}

// CachedDownload downloads the file from the URL to the cache directory and returns a reader for
// the cached file.
//
// The returned reader can be read from while download is in progress and the file is being written.
//
// It should be used only once at a time for a given URL, otherwise the file might be incomplete.
func CachedDownload(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) (io.ReadCloser, int64, errors.E) {
	// If url points to a local file, cachedPath is set to url.
	cachedPath, url := getPathAndURL(cacheDir, url)

	// The try to create the cached file.
	cachedWriteFile, err := os.OpenFile(filepath.Clean(cachedPath), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:mnd,gosec
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			// The simple code path: the cached file already exist.
			// We open it for reading and return it. We do not attempt to resume downloading
			// because we assume that the file has already been fully downloaded. We are using
			// x.NewRetryableResponse to make sure that downloading is transparently retried and we
			// attempt to delete the file if it has not been downloaded fully for any reason.
			// TODO: But it might be that the file exists because it is being downloaded in parallel so this would return incomplete file.
			cachedReadFile, err := os.Open(filepath.Clean(cachedPath))
			if err != nil {
				return nil, 0, errors.WithStack(err)
			}

			// Determining the size of the cached file.
			cachedSize, err := cachedReadFile.Seek(0, io.SeekEnd)
			if err != nil {
				return nil, 0, errors.WithStack(err)
			}
			_, err = cachedReadFile.Seek(0, io.SeekStart)
			if err != nil {
				return nil, 0, errors.WithStack(err)
			}

			logger.Info().Str("url", url).
				Int64("total", cachedSize).
				Msg("using cached file")

			return cachedReadFile, cachedSize, nil
		}

		return nil, 0, errors.WithStack(err)
	}

	cachedReadFile, err := os.Open(filepath.Clean(cachedPath))
	if err != nil {
		cachedWriteFile.Close() //nolint:errcheck,gosec
		_ = os.Remove(cachedPath)
		return nil, 0, errors.WithStack(err)
	}

	reader := &downloadingReader{
		Path:       cachedPath,
		URL:        url,
		WriteFile:  cachedWriteFile,
		ReadFile:   cachedReadFile,
		read:       0,
		size:       0,
		downloaded: notify.VarOf[int64](0),
		ctx:        nil,
		g:          nil,
		cancel:     nil,
		ticker:     nil,
	}

	size, errE := reader.Start(ctx, httpClient, logger)
	if errE != nil {
		return nil, 0, errE
	}

	return reader, size, nil
}
