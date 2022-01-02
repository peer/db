package main

import (
	"fmt"
	"net/http"
	"path"

	"github.com/hashicorp/go-retryablehttp"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/mediawiki"
)

const (
	// TODO: Find the latest one automatically.
	latestWikipediaEn = "https://dumps.wikimedia.org/other/enterprise_html/runs/20211220/enwiki-NS0-20211220-ENTERPRISE-HTML.json.tar.gz" //nolint:lll
)

// TODO: Configure logger.
var client = retryablehttp.NewClient()

func convert(config *Config) errors.E {
	filename := path.Base(latestWikipediaEn)
	return mediawiki.Process(&mediawiki.ProcessConfig{
		URL:                    latestWikipediaEn,
		CacheDir:               config.CacheDir,
		CacheGlob:              "enwiki-NS0-*-ENTERPRISE-HTML.json.tar.gz",
		CacheFilename:          func(_ *http.Response) (string, errors.E) { return filename, nil },
		Client:                 client,
		DecompressionThreads:   0,
		JSONDecodeThreads:      0,
		ItemsProcessingThreads: 0,
		// TODO: Make contact e-mail into a CLI argument.
		UserAgent: fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision), //nolint:lll
		Process: func(i interface{}) errors.E {
			return processArticle(*(i.(*Article)))
		},
		Item:        &Article{}, //nolint:exhaustivestruct
		DumpType:    mediawiki.JSONL,
		Compression: mediawiki.GZIP,
	})
}
