package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

// A silent logger.
type nullLogger struct{}

func (nullLogger) Error(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Info(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Debug(msg string, keysAndValues ...interface{}) {}

func (nullLogger) Warn(msg string, keysAndValues ...interface{}) {}

var (
	skippedWikidataEntities      = sync.Map{}
	skippedWikidataEntitiesCount int64
)

type WikidataCommand struct {
	SkippedCommonsFiles   string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikimedia Commons files."`
	SkippedWikipediaFiles string `placeholder:"PATH" type:"path" help:"Load IDs of skipped Wikipedia files."`
	SaveSkipped           string `placeholder:"PATH" type:"path" help:"Save IDs of skipped entities."`
}

func (c *WikidataCommand) Run(globals *Globals) errors.E {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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

	httpClient := retryablehttp.NewClient()
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

	esClient, errE := search.EnsureIndex(ctx, httpClient.HTTPClient)
	if errE != nil {
		return errE
	}

	cache, err := wikipedia.NewCache(lruCacheSize)
	if err != nil {
		return errors.WithStack(err)
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
		return errors.WithStack(err)
	}
	defer processor.Close()

	if c.SkippedCommonsFiles != "" {
		file, err := os.Open(c.SkippedCommonsFiles)
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
			_, loaded := skippedCommonsFiles.LoadOrStore(line, true)
			if !loaded {
				atomic.AddInt64(&skippedCommonsFilesCount, 1)
			}
		}
	}

	if c.SkippedWikipediaFiles != "" {
		file, err := os.Open(c.SkippedWikipediaFiles)
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
			_, loaded := skippedWikipediaFiles.LoadOrStore(line, true)
			if !loaded {
				atomic.AddInt64(&skippedWikipediaFilesCount, 1)
			}
		}
	}

	errE = mediawiki.ProcessWikidataDump(ctx, &mediawiki.ProcessDumpConfig{
		URL:                    "",
		CacheDir:               globals.CacheDir,
		Client:                 httpClient,
		DecompressionThreads:   0,
		DecodingThreads:        0,
		ItemsProcessingThreads: 0,
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(
				os.Stderr,
				"Progress: %0.2f%%, ETA: %s, cache miss: %d, indexed: %d, skipped: %d, failed: %d\n",
				p.Percent(), p.Remaining().Truncate(time.Second), cache.MissCount(), stats.Succeeded, skippedWikidataEntitiesCount, stats.Failed,
			)
		},
	}, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return c.processEntity(ctx, globals, httpClient, esClient, processor, cache, entity)
	})
	if errE != nil {
		return errE
	}

	if c.SaveSkipped != "" {
		var w io.Writer
		if c.SaveSkipped == "-" {
			w = os.Stdout
		} else {
			file, err := os.Create(c.SaveSkipped)
			if err != nil {
				return errors.WithStack(err)
			}
			defer file.Close()
			w = file
		}
		sortedSkipped := make([]string, 0, skippedWikidataEntitiesCount)
		skippedWikidataEntities.Range(func(key, _ interface{}) bool {
			sortedSkipped = append(sortedSkipped, key.(string))
			return true
		})
		sort.Strings(sortedSkipped)
		for _, key := range sortedSkipped {
			fmt.Fprintf(w, "%s\n", key)
		}
	}

	return nil
}

func (c *WikidataCommand) processEntity(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, esClient *elastic.Client,
	processor *elastic.BulkProcessor, cache *wikipedia.Cache, entity mediawiki.Entity,
) errors.E {
	document, err := wikipedia.ConvertEntity(ctx, httpClient, esClient, cache, &skippedCommonsFiles, entity)
	if errors.Is(err, wikipedia.SkippedError) {
		_, loaded := skippedWikidataEntities.LoadOrStore(entity.ID, true)
		if !loaded {
			atomic.AddInt64(&skippedWikidataEntitiesCount, 1)
		}
		if !errors.Is(err, wikipedia.SilentSkippedError) {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		}
		return nil
	} else if err != nil {
		return err
	}

	saveDocument(globals, processor, document)

	return nil
}
