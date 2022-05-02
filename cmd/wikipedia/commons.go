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
	// Set of filenames.
	skippedCommonsFiles      = sync.Map{}
	skippedCommonsFilesCount int64
)

// CommonsFilesCommand uses Wikimedia Commons images (really files) table SQL dump as input and creates a document for each file in the table.
//
// It creates claims with the following properties (not necessary all of them): WIKIMEDIA_COMMONS_FILE_NAME (just filename, without "File:"
// prefix, but with underscores and file extension), WIKIMEDIA_COMMONS_FILE (URL to file page), WIKIMEDIA_COMMONS_FILE_URL (URL to full
// resolution or raw file), FILE (is claim), MEDIA_TYPE, SIZE (in bytes), MEDIAWIKI_MEDIA_TYPE, multiple PREVIEW_URL (a list of URLs of previews),
// PAGE_COUNT, LENGTH (in seconds), WIDTH, HEIGHT. Name of the document is filename without file extension and without underscores.
// The idea is that these claims should be enough to populate a file claim (in other documents using these files).
//
// Files are skipped when metadata is invalid (e.g., unexpected media type, zero size, missing page count when it is expected, zero duration,
// missing width/height when they are expected).
//
// For some files (primarily PDFs and DJVU files) metadata is not stored in the SQL table but SQL table only contains a reference to additional
// blob storage (see: https://phabricator.wikimedia.org/T301039). Because of that this command uses Wikimedia Commons API to obtain metadata
// for those files. This introduces some issues. API is rate limited, so processing can be slower than pure offline processing would be (configuring
// high ItemsProcessingThreads can mitigate this somewhat, so that while some threads are blocked on API, other threads can continue to process other
// files which do not require API). There can be discrepancies between the table state and what is available through the API: files from the table might
// be deleted since the table dump has been made. On the other hand, metadata rarely changes (only if metadata is re-extracted/re-computed, or if a new version
// of a file has been uploaded) so the fact that metadata might be from a different file revision does not seem to be too problematic here. We anyway
// want the latest information about files because we directly use files hosted on Wikimedia Commons by displaying them, so if they are changed or deleted,
// we want to know that (otherwise we could try to display an image which does not exist anymore, which would fail to load).
type CommonsFilesCommand struct {
	SaveSkipped string `placeholder:"PATH" type:"path" help:"Save filenames of skipped files."`
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

	errE = mediawiki.Process(ctx, &mediawiki.ProcessConfig[wikipedia.Image]{
		URL:                    config.URL,
		Path:                   config.Path,
		Client:                 config.Client,
		DecompressionThreads:   config.DecompressionThreads,
		DecodingThreads:        config.DecodingThreads,
		ItemsProcessingThreads: config.ItemsProcessingThreads,
		Process: func(ctx context.Context, i wikipedia.Image) errors.E {
			return processImage(
				ctx, globals, httpClient, processor, wikipedia.ConvertWikimediaCommonsImage,
				&skippedCommonsFiles, &skippedCommonsFilesCount, i,
			)
		},
		Progress:    config.Progress,
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
	convert func(context.Context, *retryablehttp.Client, string, int, wikipedia.Image) (*search.Document, errors.E),
	skippedMap *sync.Map, count *int64,
	image wikipedia.Image,
) errors.E {
	document, err := convert(ctx, httpClient, globals.Token, globals.APILimit, image)
	if err != nil {
		if errors.Is(err, wikipedia.SilentSkippedError) {
			globals.Log.Debug().Str("file", image.Name).Err(err).Fields(errors.AllDetails(err)).Send()
		} else if errors.Is(err, wikipedia.SkippedError) {
			globals.Log.Warn().Str("file", image.Name).Err(err).Fields(errors.AllDetails(err)).Send()
		} else {
			globals.Log.Error().Str("file", image.Name).Err(err).Fields(errors.AllDetails(err)).Send()
		}
		_, loaded := skippedMap.LoadOrStore(image.Name, true)
		if !loaded {
			atomic.AddInt64(count, 1)
		}
		return nil
	}

	globals.Log.Debug().Str("doc", string(document.ID)).Str("file", image.Name).Msg("saving document")
	saveDocument(processor, document)

	return nil
}
