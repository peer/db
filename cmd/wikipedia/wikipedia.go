package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

var (
	skippedWikipediaFiles      = sync.Map{}
	skippedWikipediaFilesCount int64
)

type WikipediaFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped files."`
	URL         string `placeholder:"URL" help:"URL of Wikipedia image table SQL dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaFilesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaImageMetadataRun(client, "enwiki")
		}
	}

	ctx, cancel, httpClient, _, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i interface{}) errors.E {
			return c.processImage(ctx, globals, httpClient, processor, *i.(*wikipedia.Image))
		},
		Progress:    config.Progress,
		Item:        &wikipedia.Image{},
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
	if errE != nil {
		return errE
	}

	errE = saveSkippedMap(c.SaveSkipped, &skippedWikipediaFiles, &skippedWikipediaFilesCount)
	if errE != nil {
		return errE
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

type WikipediaArticlesCommand struct {
	URL string `placeholder:"URL" help:"URL of Wikipedia articles HTML dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *WikipediaArticlesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = func(client *retryablehttp.Client) (string, errors.E) {
			return mediawiki.LatestWikipediaRun(client, "enwiki", 0)
		}
	}

	ctx, cancel, _, esClient, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	return mediawiki.ProcessWikipediaDump(ctx, config, func(ctx context.Context, article mediawiki.Article) errors.E {
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
