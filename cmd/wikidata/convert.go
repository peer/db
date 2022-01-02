package main

import (
	"fmt"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search/internal/mediawiki"
)

const (
	latestWikidataAll = "https://dumps.wikimedia.org/wikidatawiki/entities/latest-all.json.bz2"
)

// TODO: Configure logger.
var client = retryablehttp.NewClient()

func convert(config *Config) errors.E {
	return mediawiki.Process(&mediawiki.ProcessConfig{
		URL:       latestWikidataAll,
		CacheDir:  config.CacheDir,
		CacheGlob: "wikidata-*-all.json.bz2",
		CacheFilename: func(resp *http.Response) (string, errors.E) {
			lastModifiedStr := resp.Header.Get("Last-Modified")
			if lastModifiedStr == "" {
				return "", errors.Errorf("missing Last-Modified header in response")
			}
			lastModified, err := http.ParseTime(lastModifiedStr)
			if err != nil {
				return "", errors.WithStack(err)
			}
			return fmt.Sprintf("wikidata-%s-all.json.bz2", lastModified.UTC().Format("20060102")), nil
		},
		Client:                 client,
		DecompressionThreads:   0,
		JSONDecodeThreads:      0,
		ItemsProcessingThreads: 0,
		// TODO: Make contact e-mail into a CLI argument.
		UserAgent: fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision), //nolint:lll
		Process: func(i interface{}) errors.E {
			return processEntity(*(i.(*Entity)))
		},
		Item:        &Entity{}, //nolint:exhaustivestruct
		DumpType:    mediawiki.JSONArray,
		Compression: mediawiki.BZIP2,
	})
}
