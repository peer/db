package main

import (
	"context"
	"encoding/json"
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
	latestWikipediaFiles = "https://dumps.wikimedia.org/enwiki/20220120/enwiki-20220120-image.sql.gz"
)

var (
	skippedWikipediaFiles      = sync.Map{}
	skippedWikipediaFilesCount int64
)

type WikipediaFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped files."`
}

func (c *WikipediaFilesCommand) Run(globals *Globals) errors.E {
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
		URL:       latestWikipediaFiles,
		CacheDir:  globals.CacheDir,
		CacheGlob: "enwiki-*-image.sql.gz",
		CacheFilename: func(_ *http.Response) (string, errors.E) {
			return "enwiki-20220120-image.sql.gz", nil
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
				p.Percent(), p.Remaining().Truncate(time.Second), stats.Succeeded, skippedWikipediaFilesCount, stats.Failed,
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
		sortedSkipped := make([]string, 0, skippedWikipediaFilesCount)
		skippedWikipediaFiles.Range(func(key, _ interface{}) bool {
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

func (c *WikipediaFilesCommand) processImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, processor *elastic.BulkProcessor, image wikipedia.Image,
) errors.E {
	document, err := wikipedia.ConvertWikipediaImage(ctx, httpClient, image)
	if errors.Is(err, wikipedia.SkippedError) {
		_, loaded := skippedWikipediaFiles.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(&skippedWikipediaFilesCount, 1)
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

type WikipediaFileDescriptionsCommand struct{}

func (c *WikipediaFileDescriptionsCommand) Run(globals *Globals) errors.E {
	return nil
}

type WikipediaArticlesCommand struct{}

func (c *WikipediaArticlesCommand) Run(globals *Globals) errors.E {
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

	return mediawiki.ProcessWikipediaDump(ctx, &mediawiki.ProcessDumpConfig{
		URL:                    "",
		CacheDir:               globals.CacheDir,
		Client:                 httpClient,
		DecompressionThreads:   0,
		DecodingThreads:        0,
		ItemsProcessingThreads: 0,
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s, indexed: %d, failed: %d\n", p.Percent(), p.Remaining().Truncate(time.Second), stats.Succeeded, stats.Failed)
		},
	}, func(ctx context.Context, article mediawiki.Article) errors.E {
		return c.processArticle(ctx, globals, esClient, processor, article)
	})
}

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata.
// TODO: Store categories and used templates into claims.
// TODO: Make internal links to other articles work in HTML (link to PeerDB documents instead).
// TODO: Remove links to other articles which do not exist, if there are any.
// TODO: Split article into summary and main part.
// TODO: Clean custom tags and attributes used in HTML to add metadata into HTML, potentially extract and store that.
//       See: https://www.mediawiki.org/wiki/Specs/HTML/2.4.0
// TODO: Make // links/src into https:// links/src.
// TODO: Remove some templates (e.g., infobox, top-level notices) and convert them to claims.
// TODO: Remove rendered links to categories (they should be claims).
// TODO: Extract all links pointing out of the article into claims and reverse claims (so if they point to other documents, they should have backlink as claim).
// TODO: Keep only contents of <body>.
// TODO: Skip disambiguation pages (remove corresponding document if we already have it).

func (c *WikipediaArticlesCommand) processArticle(
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article,
) errors.E {
	if article.MainEntity.Identifier == "" {
		return nil
	}
	id := wikipedia.GetWikidataDocumentID(article.MainEntity.Identifier)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	} else if !esDoc.Found {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	}
	var document search.Document
	err = json.Unmarshal(esDoc.Source, &document)
	if err != nil {
		return errors.Errorf(`error JSON decoding document %s for entity %s for article "%s": %w`, id, article.MainEntity.Identifier, article.Name, err)
	}
	claimID := search.GetID(wikipedia.NameSpaceWikidata, id, "ARTICLE", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.TextClaim)
		if !ok {
			return errors.Errorf(`document %s for entity %s for article "%s" has existing non-text claim with ID %s`, id, article.MainEntity.Identifier, article.Name, claimID)
		}
		claim.HTML["en"] = article.ArticleBody.HTML
	} else {
		claim := &search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: 1.0,
			},
			Prop: search.GetStandardPropertyReference("ARTICLE"),
			HTML: search.TranslatableHTMLString{
				"en": article.ArticleBody.HTML,
			},
		}
		err = document.Add(claim)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"article claim cannot be added to document %s for entity %s for article \"%s\": %s\n",
				id, article.MainEntity.Identifier, article.Name, err.Error(),
			)
			return nil
		}
	}

	updateDocument(globals, processor, *esDoc.SeqNo, *esDoc.PrimaryTerm, &document)

	return nil
}
