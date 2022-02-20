package main

import (
	"context"
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
	skippedCommonsFiles      = sync.Map{}
	skippedCommonsFilesCount int64
)

type CommonsFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save IDs of skipped files."`
	URL         string `placeholder:"URL" help:"URL of Wikimedia Commons image table SQL dump to use. It can be a local file path, too. Default: the latest."`
}

func (c *CommonsFilesCommand) Run(globals *Globals) errors.E {
	var urlFunc func(_ *retryablehttp.Client) (string, errors.E)
	if c.URL != "" {
		urlFunc = func(_ *retryablehttp.Client) (string, errors.E) {
			return c.URL, nil
		}
	} else {
		urlFunc = mediawiki.LatestCommonsImageMetadataRun
	}

	ctx, cancel, httpClient, _, processor, _, config, errE := initializeRun(globals, urlFunc, &skippedCommonsFilesCount)
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
			return processImage(
				ctx, globals, httpClient, processor, wikipedia.ConvertWikimediaCommonsImage,
				&skippedCommonsFiles, &skippedCommonsFilesCount, *i.(*wikipedia.Image),
			)
		},
		Progress:    config.Progress,
		Item:        &wikipedia.Image{},
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
	if errE != nil {
		return errE
	}

	errE = saveSkippedMap(c.SaveSkipped, &skippedCommonsFiles, &skippedCommonsFilesCount)
	if errE != nil {
		return errE
	}

	return nil
}

func processImage(
	ctx context.Context, globals *Globals, httpClient *retryablehttp.Client, processor *elastic.BulkProcessor,
	convert func(context.Context, *retryablehttp.Client, wikipedia.Image) (*search.Document, errors.E),
	skippedMap *sync.Map, count *int64,
	image wikipedia.Image,
) errors.E {
	document, err := convert(ctx, httpClient, image)
	if err != nil {
		if errors.Is(err, wikipedia.SilentSkippedError) {
			// We do not log stack trace.
			globals.Log.Debug().Str("file", image.Name).Msg(err.Error())
		} else if errors.Is(err, wikipedia.SkippedError) {
			// We do not log stack trace.
			globals.Log.Warn().Str("file", image.Name).Msg(err.Error())
		} else {
			globals.Log.Error().Str("file", image.Name).Err(err).Send()
		}
		_, loaded := skippedMap.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(count, 1)
		}
		return nil
	}

	saveDocument(globals, processor, document)

	return nil
}
