package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
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

const (
	// TODO: Determine full latest dump dynamically (not in progress/partial).
	latestCommonsImages = "https://dumps.wikimedia.org/commonswiki/20220120/commonswiki-20220120-image.sql.gz"
)

var (
	skippedcommonsImages      = sync.Map{}
	skippedcommonsImagesCount int64
)

type CommonsImagesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped files."`
}

func (c *CommonsImagesCommand) Run(globals *Globals) errors.E {
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

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig{
		URL:       latestCommonsImages,
		CacheDir:  globals.CacheDir,
		CacheGlob: "commonswiki-*-image.sql.gz",
		CacheFilename: func(_ *http.Response) (string, errors.E) {
			return "commonswiki-20220120-image.sql.gz", nil
		},
		Client:                 httpClient,
		DecompressionThreads:   0,
		DecodingThreads:        0,
		ItemsProcessingThreads: 0,
		Process: func(ctx context.Context, i interface{}) errors.E {
			return c.processImage(ctx, globals, httpClient, processor, *i.(*wikipedia.Image))
		},
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(
				os.Stderr,
				"Progress: %0.2f%%, ETA: %s, indexed: %d, skipped: %d, failed: %d\n",
				p.Percent(), p.Remaining().Truncate(time.Second), stats.Succeeded, skippedcommonsImagesCount, stats.Failed,
			)
		},
		Item:        &wikipedia.Image{},
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
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
		sortedSkipped := make([]string, 0, skippedcommonsImagesCount)
		skippedcommonsImages.Range(func(key, _ interface{}) bool {
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

func (c *CommonsImagesCommand) processImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, processor *elastic.BulkProcessor, image wikipedia.Image,
) errors.E {
	document, err := wikipedia.ConvertWikimediaCommonsImage(ctx, httpClient, image)
	if errors.Is(err, wikipedia.SkippedError) {
		skippedcommonsImages.Store(image.Name, true)
		atomic.AddInt64(&skippedcommonsImagesCount, 1)
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return nil
	} else if err != nil {
		return err
	}

	saveDocument(globals, processor, document)

	return nil
}
