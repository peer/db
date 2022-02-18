package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
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

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

const (
	lruCacheSize = 1000000
)

func saveDocument(globals *Globals, processor *elastic.BulkProcessor, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(doc.ID)).Doc(doc)
	processor.Add(req)
}

func updateDocument(globals *Globals, processor *elastic.BulkProcessor, seqNo, primaryTerm int64, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(doc.ID)).IfSeqNo(seqNo).IfPrimaryTerm(primaryTerm).Doc(&doc)
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

// A silent logger.
type nullLogger struct{}

func (nullLogger) Error(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Info(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Debug(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Warn(msg string, keysAndValues ...interface{}) {}

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

	esClient, errE := search.EnsureIndex(ctx, httpClient)
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
				fmt.Fprintf(os.Stderr, "Indexing error: %s\n", err.Error())
			} else if response.Errors {
				for _, failed := range response.Failed() {
					fmt.Fprintf(os.Stderr, "Indexing error %d (%s): %s [type=%s]\n", failed.Status, http.StatusText(failed.Status), failed.Error.Reason, failed.Error.Type)
				}
				fmt.Fprintf(os.Stderr, "Indexing error\n")
			}
		},
	).Do(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, errors.WithStack(err)
	}

	return ctx, cancel, httpClient, esClient, processor, cache, nil
}

func initializeRun(globals *Globals, urlFunc func(*retryablehttp.Client) (
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

	// We silent debug logging from HTTP client.
	// TODO: Configure proper logger.
	httpClient.Logger = nullLogger{}

	// Set User-Agent header.
	httpClient.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision))
	}

	url, errE := urlFunc(httpClient)
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
		DecompressionThreads:   0,
		DecodingThreads:        0,
		ItemsProcessingThreads: 0,
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(
				os.Stderr,
				"Progress: %0.2f%%, ETA: %s, cache miss: %d, indexed: %d, skipped: %d, failed: %d\n",
				p.Percent(), p.Remaining().Truncate(time.Second), cache.MissCount(), stats.Succeeded, atomic.LoadInt64(count), stats.Failed,
			)
		},
	}, nil
}
