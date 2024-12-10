package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/internal/wikipedia"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	lruCacheSize = 1000000
)

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
		sortedSkipped = append(sortedSkipped, key.(string)) //nolint:forcetypeassert
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

func initializeElasticSearch(globals *Globals) (
	context.Context, context.CancelFunc, *retryablehttp.Client,
	*store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	*elastic.Client, *elastic.BulkProcessor, *es.Cache, errors.E,
) {
	ctx, stop, httpClient, store, esClient, esProcessor, errE := es.Standalone(
		globals.Logger, string(globals.Postgres.URL), globals.Elastic.URL, globals.Postgres.Schema, globals.Elastic.Index, globals.Elastic.SizeField,
	)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, nil, errE
	}

	cache, errE := es.NewCache(lruCacheSize)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, nil, errE
	}

	return ctx, stop, httpClient, store, esClient, esProcessor, cache, errE
}

func initializeRun(
	globals *Globals,
	urlFunc func(context.Context, *retryablehttp.Client) (string, errors.E),
	count *int64,
) (
	context.Context, context.CancelFunc, *retryablehttp.Client,
	*store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes], *elastic.Client,
	*elastic.BulkProcessor, *es.Cache, *mediawiki.ProcessDumpConfig, errors.E,
) {
	ctx, stop, httpClient, store, esClient, esProcessor, cache, errE := initializeElasticSearch(globals)
	if errE != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, errE
	}

	if urlFunc != nil {
		url, errE := urlFunc(ctx, httpClient)
		if errE != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, errE
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

		return ctx, stop, httpClient, store, esClient, esProcessor, cache, &mediawiki.ProcessDumpConfig{
			URL:                    url,
			Path:                   dumpPath,
			Client:                 httpClient,
			DecompressionThreads:   globals.DecodingThreads,
			DecodingThreads:        globals.DecodingThreads,
			ItemsProcessingThreads: globals.ItemsProcessingThreads,
			Progress:               es.Progress(globals.Logger, esProcessor, cache, count, ""),
		}, nil
	}

	return ctx, stop, httpClient, store, esClient, esProcessor, cache, nil, nil
}

func templatesCommandRun(globals *Globals, site, skippedWikidataEntitiesPath, mnemonicPrefix, from string) errors.E {
	errE := populateSkippedMap(skippedWikidataEntitiesPath, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	ctx, stop, httpClient, store, esClient, esProcessor, _, _, errE := initializeRun(globals, nil, nil)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

	pages := make(chan wikipedia.AllPagesPage, wikipedia.APILimit)
	rateLimit := wikipediaRESTRateLimit / wikipediaRESTRatePeriod.Seconds()
	limiter := rate.NewLimiter(rate.Limit(rateLimit), wikipediaRESTRateLimit)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(pages)
		return wikipedia.ListAllPages(ctx, httpClient, []int{templatesWikipediaNamespace, modulesWikipediaNamespace}, site, limiter, pages)
	})

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, 0, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := esProcessor.Stats()
			globals.Logger.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("count", p.Count).
				Str("elapsed", p.Elapsed.Truncate(time.Second).String()).
				Send()
		}
	}()

	for range int(rateLimit) {
		g.Go(func() error {
			// Loop ends when pages is closed, which happens when context is cancelled, too.
			for page := range pages {
				if page.Properties["wikibase_item"] == "" {
					globals.Logger.Debug().Str("title", page.Title).Msg("template without Wikidata item")
					continue
				}

				err := limiter.Wait(ctx)
				if err != nil {
					// Context has been canceled.
					return errors.WithStack(err)
				}

				// First we try to get "/doc".
				html, errE := wikipedia.GetPageHTML(ctx, httpClient, site, page.Title+"/doc")
				if errE != nil {
					if errors.AllDetails(errE)["code"] != http.StatusNotFound {
						globals.Logger.Error().Err(errE).Send()
						continue
					}

					err := limiter.Wait(ctx)
					if err != nil {
						// Context has been canceled.
						return errors.WithStack(err)
					}

					// And if it does not exist, without "/doc".
					html, errE = wikipedia.GetPageHTML(ctx, httpClient, site, page.Title)
					if errE != nil {
						globals.Logger.Error().Err(errE).Send()
						continue
					}
				}

				count.Increment()

				errE = templatesCommandProcessPage(ctx, globals, store, esClient, page, html, mnemonicPrefix, from)
				if errE != nil {
					return errE
				}
			}
			return nil
		})
	}

	return errors.WithStack(g.Wait())
}

func templatesCommandProcessPage(
	ctx context.Context, globals *Globals,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	esClient *elastic.Client, page wikipedia.AllPagesPage, html, mnemonicPrefix, from string,
) errors.E { //nolint:unparam
	// We know this is available because we check before calling this method.
	id := page.Properties["wikibase_item"]

	if _, ok := skippedWikidataEntities.Load(wikipedia.GetWikidataDocumentID(id).String()); ok {
		globals.Logger.Debug().Str("entity", id).Str("title", page.Title).Msg("skipped entity")
		return nil
	}

	document, version, errE := wikipedia.GetWikidataItem(ctx, store, globals.Elastic.Index, esClient, id)
	if errE != nil {
		details := errors.Details(errE)
		details["entity"] = id
		details["title"] = page.Title
		if errors.Is(errE, wikipedia.ErrNotFound) {
			globals.Logger.Warn().Err(errE).Send()
		} else {
			globals.Logger.Error().Err(errE).Send()
		}
		return nil
	}

	// Page title we add only from English Wikipedia as NAME claim on the document when processing
	// Wikidata entities (we have it there through site links on Wikidata entities), but not for
	// Wikimedia Commons as they might not be in English.

	errE = wikipedia.SetPageID(wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page.Identifier, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertTemplateDescription(id, from, html, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageInCategories(globals.Logger, wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageUsedTemplates(globals.Logger, wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	errE = wikipedia.ConvertPageRedirects(globals.Logger, wikipedia.NameSpaceWikidata, id, page, document)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("entity", id).Str("title", page.Title).Msg("updating document")
	errE = peerdb.UpdateDocument(ctx, store, document, version)
	if errE != nil {
		details := errors.Details(errE)
		details["doc"] = document.ID.String()
		details["entity"] = id
		details["title"] = page.Title
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	return nil
}

func filesCommandRun(
	globals *Globals,
	urlFunc func(context.Context, *retryablehttp.Client) (string, errors.E),
	token string, apiLimit int, saveSkipped string, skippedMap *sync.Map, skippedCount *int64,
	convertImage func(context.Context, zerolog.Logger, *retryablehttp.Client, string, int, wikipedia.Image) (*document.D, errors.E),
) errors.E {
	ctx, stop, httpClient, store, _, esProcessor, _, config, errE := initializeRun(globals, urlFunc, skippedCount)
	if errE != nil {
		return errE
	}
	defer stop()
	defer esProcessor.Close()

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig[wikipedia.Image]{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i wikipedia.Image) errors.E {
			return filesCommandProcessImage(
				ctx, globals, httpClient, store, token, apiLimit, skippedMap, skippedCount, i, convertImage,
			)
		},
		Progress:    config.Progress,
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
	if errE != nil {
		return errE
	}

	errE = saveSkippedMap(saveSkipped, skippedMap, skippedCount)
	if errE != nil {
		return errE
	}

	return nil
}

func filesCommandProcessImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	token string, apiLimit int, skippedMap *sync.Map, skippedCount *int64, image wikipedia.Image,
	convertImage func(context.Context, zerolog.Logger, *retryablehttp.Client, string, int, wikipedia.Image) (*document.D, errors.E),
) errors.E {
	document, errE := convertImage(ctx, globals.Logger, httpClient, token, apiLimit, image)
	if errE != nil {
		details := errors.Details(errE)
		details["file"] = image.Name
		if errors.Is(errE, wikipedia.ErrSilentSkipped) {
			globals.Logger.Debug().Err(errE).Send()
		} else if errors.Is(errE, wikipedia.ErrSkipped) {
			globals.Logger.Warn().Err(errE).Send()
		} else {
			globals.Logger.Error().Err(errE).Send()
		}
		_, loaded := skippedMap.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(skippedCount, 1)
		}
		return nil
	}

	globals.Logger.Debug().Str("doc", document.ID.String()).Str("file", image.Name).Msg("saving document")
	errE = peerdb.InsertOrReplaceDocument(ctx, store, document)
	if errE != nil {
		globals.Logger.Error().Err(errE).Send()
		return nil
	}

	return nil
}
