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
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

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

func initializeRun(
	globals *Globals,
	urlFunc func(context.Context, *retryablehttp.Client) (string, errors.E),
	count *int64,
) (
	context.Context, context.CancelFunc, *retryablehttp.Client, *elastic.Client,
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

func templatesCommandRun(globals *Globals, site, skippedWikidataEntitiesPath, mnemonicPrefix, from string) errors.E {
	errE := populateSkippedMap(skippedWikidataEntitiesPath, &skippedWikidataEntities, &skippedWikidataEntitiesCount)
	if errE != nil {
		return errE
	}

	ctx, cancel, httpClient, esClient, processor, _, _, errE := initializeRun(globals, nil, nil)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	pages := make(chan wikipedia.AllPagesPage, wikipedia.APILimit)
	rateLimit := wikipediaRESTRateLimit / wikipediaRESTRatePeriod.Seconds()
	limiter := rate.NewLimiter(rate.Limit(rateLimit), wikipediaRESTRateLimit)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(pages)
		return wikipedia.ListAllPages(ctx, httpClient, []int{templatesWikipediaNamespace, modulesWikipediaNamespace}, site, limiter, pages)
	})

	var count x.Counter
	ticker := x.NewTicker(ctx, &count, 0, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			stats := processor.Stats()
			globals.Log.Info().
				Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).Int64("docs", count.Count()).
				Str("elapsed", p.Elapsed.Truncate(time.Second).String()).
				Send()
		}
	}()

	for i := 0; i < int(rateLimit); i++ {
		g.Go(func() error {
			// Loop ends with pages is closed, which happens when context is cancelled, too.
			for page := range pages {
				if page.Properties["wikibase_item"] == "" {
					globals.Log.Debug().Str("title", page.Title).Msg("template without Wikidata item")
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
						globals.Log.Error().Err(errE).Fields(errors.AllDetails(errE)).Send()
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
						globals.Log.Error().Err(errE).Fields(errors.AllDetails(errE)).Send()
						continue
					}
				}

				count.Increment()

				errE = templatesCommandProcessPage(ctx, globals, esClient, processor, page, html, mnemonicPrefix, from)
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
	ctx context.Context, globals *Globals, esClient *elastic.Client, processor *elastic.BulkProcessor,
	page wikipedia.AllPagesPage, html, mnemonicPrefix, from string,
) errors.E {
	// We know this is available because we check before calling this method.
	id := page.Properties["wikibase_item"]

	if _, ok := skippedWikidataEntities.Load(string(wikipedia.GetWikidataDocumentID(id))); ok {
		globals.Log.Debug().Str("entity", id).Str("title", page.Title).Msg("skipped entity")
		return nil
	}

	document, hit, err := wikipedia.GetWikidataItem(ctx, globals.Index, esClient, id)
	if err != nil {
		details := errors.AllDetails(err)
		details["entity"] = id
		details["title"] = page.Title
		if errors.Is(err, wikipedia.NotFoundError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		return nil
	}

	err = wikipedia.SetPageID(wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page.Identifier, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertTemplateDescription(id, from, html, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageInCategories(globals.Log, wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageUsedTemplates(globals.Log, wikipedia.NameSpaceWikidata, mnemonicPrefix, id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	err = wikipedia.ConvertPageRedirects(globals.Log, wikipedia.NameSpaceWikidata, id, page, document)
	if err != nil {
		details := errors.AllDetails(err)
		details["doc"] = string(document.ID)
		details["entity"] = id
		details["title"] = page.Title
		globals.Log.Error().Err(err).Fields(details).Send()
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("entity", id).Str("title", page.Title).Msg("updating document")
	search.UpdateDocument(processor, globals.Index, *hit.SeqNo, *hit.PrimaryTerm, document)

	return nil
}

func filesCommandRun(
	globals *Globals,
	urlFunc func(context.Context, *retryablehttp.Client) (string, errors.E),
	token string, apiLimit int, saveSkipped string, skippedMap *sync.Map, skippedCount *int64,
	convertImage func(context.Context, zerolog.Logger, *retryablehttp.Client, string, int, wikipedia.Image) (*search.Document, errors.E),
) errors.E {
	ctx, cancel, httpClient, _, processor, _, config, errE := initializeRun(globals, urlFunc, skippedCount)
	if errE != nil {
		return errE
	}
	defer cancel()
	defer processor.Close()

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig[wikipedia.Image]{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i wikipedia.Image) errors.E {
			return filesCommandProcessImage(
				ctx, globals, httpClient, processor, token, apiLimit, skippedMap, skippedCount, i, convertImage,
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
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, processor *elastic.BulkProcessor,
	token string, apiLimit int, skippedMap *sync.Map, skippedCount *int64, image wikipedia.Image,
	convertImage func(context.Context, zerolog.Logger, *retryablehttp.Client, string, int, wikipedia.Image) (*search.Document, errors.E),
) errors.E {
	document, err := convertImage(ctx, globals.Log, httpClient, token, apiLimit, image)
	if err != nil {
		details := errors.AllDetails(err)
		details["file"] = image.Name
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Err(err).Fields(details).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Err(err).Fields(details).Send()
		} else {
			globals.Log.Error().Err(err).Fields(details).Send()
		}
		_, loaded := skippedMap.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(skippedCount, 1)
		}
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", image.Name).Msg("saving document")
	search.InsertOrReplaceDocument(processor, globals.Index, document)

	return nil
}
