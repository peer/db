package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/cli"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	lruCacheSize = 1000000
)

// insertOrReplaceDocument inserts or replaces the document based on its ID.
func insertOrReplaceDocument(processor *elastic.BulkProcessor, index string, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index(index).Id(string(doc.ID)).Doc(doc)
	processor.Add(req)
}

// updateDocument updates the document in the index, if it has not changed in the database since it was fetched (based on seqNo and primaryTerm).
func updateDocument(processor *elastic.BulkProcessor, index string, seqNo, primaryTerm int64, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index(index).Id(string(doc.ID)).IfSeqNo(seqNo).IfPrimaryTerm(primaryTerm).Doc(&doc)
	processor.Add(req)
}

func populateSkippedMap(path string, skippedMap *sync.Map, count *int64) errors.E {
	if path == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()

	r := bufio.NewReader(file)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return errors.WithStack(err)
		}
		line = strings.TrimSuffix(line, "\n")
		_, loaded := skippedMap.LoadOrStore(line, true)
		if !loaded {
			atomic.AddInt64(count, 1)
		}
	}

	return nil
}

func saveSkippedMap(path string, skippedMap *sync.Map, count *int64) errors.E {
	if path == "" {
		return nil
	}

	var w io.Writer
	if path == "-" {
		w = os.Stdout
	} else {
		file, err := os.Create(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer file.Close()
		w = file
	}

	sortedSkipped := make([]string, 0, atomic.LoadInt64(count))
	skippedMap.Range(func(key, _ interface{}) bool {
		sortedSkipped = append(sortedSkipped, key.(string))
		return true
	})
	sort.Strings(sortedSkipped)
	for _, key := range sortedSkipped {
		_, err := fmt.Fprintf(w, "%s\n", key)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func prepareFields(keysAndValues []interface{}) {
	for i, keyOrValue := range keysAndValues {
		// We want URLs logged as strings.
		u, ok := keyOrValue.(*url.URL)
		if ok {
			keysAndValues[i] = u.String()
		}
	}
}

type retryableHTTPLoggerAdapter struct {
	log zerolog.Logger
}

func (a retryableHTTPLoggerAdapter) Error(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Error().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Info().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Debug(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Debug().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Warn(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.log.Warn().Fields(keysAndValues).Msg(msg)
}

var _ retryablehttp.LeveledLogger = (*retryableHTTPLoggerAdapter)(nil)

func initializeElasticSearch(globals *Globals) (
	context.Context, context.CancelFunc, *http.Client, *elastic.Client,
	*elastic.BulkProcessor, *wikipedia.Cache, errors.E,
) {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	httpClient := cleanhttp.DefaultPooledClient()

	esClient, errE := search.EnsureIndex(ctx, httpClient, globals.Log, globals.Elastic, globals.Index)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	cache, errE := wikipedia.NewCache(lruCacheSize)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				globals.Log.Error().Err(err).Msg("indexing error")
			} else if failed := response.Failed(); len(failed) > 0 {
				for _, f := range failed {
					globals.Log.Error().
						Str("id", f.Id).Int("code", f.Status).
						Str("reason", f.Error.Reason).Str("type", f.Error.Type).
						Msg("indexing error")
				}
			}
		},
	).Do(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, errors.WithStack(err)
	}

	return ctx, cancel, httpClient, esClient, processor, cache, nil
}

func initializeRun(globals *Globals, urlFunc func(context.Context, *retryablehttp.Client) (
	string, errors.E), count *int64) (context.Context, context.CancelFunc, *retryablehttp.Client, *elastic.Client,
	*elastic.BulkProcessor, *wikipedia.Cache, *mediawiki.ProcessDumpConfig, errors.E,
) {
	ctx, cancel, simpleHTTPClient, esClient, processor, cache, errE := initializeElasticSearch(globals)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, nil, errE
	}

	httpClient := retryablehttp.NewClient()
	httpClient.HTTPClient = simpleHTTPClient
	httpClient.RetryWaitMax = clientRetryWaitMax
	httpClient.RetryMax = clientRetryMax
	httpClient.Logger = retryableHTTPLoggerAdapter{globals.Log}

	// Set User-Agent header.
	httpClient.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision))
	}

	if urlFunc != nil {
		url, errE := urlFunc(ctx, httpClient)
		if errE != nil {
			return nil, nil, nil, nil, nil, nil, nil, errE
		}

		// Is URL in fact a path to a local file?
		var dumpPath string
		_, err := os.Stat(url)
		if os.IsNotExist(err) {
			dumpPath = filepath.Join(globals.CacheDir, path.Base(url))
		} else {
			dumpPath = url
			url = ""
		}

		return ctx, cancel, httpClient, esClient, processor, cache, &mediawiki.ProcessDumpConfig{
			URL:                    url,
			Path:                   dumpPath,
			Client:                 httpClient,
			DecompressionThreads:   globals.DecodingThreads,
			DecodingThreads:        globals.DecodingThreads,
			ItemsProcessingThreads: globals.ItemsProcessingThreads,
			Progress: func(ctx context.Context, p x.Progress) {
				stats := processor.Stats()
				e := globals.Log.Info().
					Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).
					Uint64("cacheMiss", cache.MissCount()).Str("eta", p.Remaining().Truncate(time.Second).String())
				if count != nil {
					e = e.Int64("skipped", atomic.LoadInt64(count))
				}
				e.Msgf("progress %0.2f%%", p.Percent())
			},
		}, nil
	}

	return ctx, cancel, httpClient, esClient, processor, cache, nil, nil
}
