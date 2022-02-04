package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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

type WikipediaMediaInfoCommand struct{}

func (c *WikipediaMediaInfoCommand) Run(globals *Globals) errors.E {
	return nil
}

type WikipediaFilesCommand struct{}

func (c *WikipediaFilesCommand) Run(globals *Globals) errors.E {
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

	client := retryablehttp.NewClient()
	client.RetryWaitMax = clientRetryWaitMax
	client.RetryMax = clientRetryMax

	// Set User-Agent header.
	client.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision))
	}

	esClient, errE := search.EnsureIndex(ctx, client.HTTPClient)
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
		Client:                 client,
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
	id := wikipedia.GetDocumentID(article.MainEntity.Identifier)
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
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(id)).IfSeqNo(*esDoc.SeqNo).IfPrimaryTerm(*esDoc.PrimaryTerm).Doc(&document)
	processor.Add(req)
	return nil
}
