package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
)

// TODO: Configure logger.
var client = retryablehttp.NewClient()

func convert(config *Config) errors.E {
	ctx := context.Background()
	return mediawiki.ProcessWikipediaDump(ctx, &mediawiki.ProcessDumpConfig{
		URL:                    "",
		CacheDir:               config.CacheDir,
		Client:                 client,
		DecompressionThreads:   0,
		JSONDecodeThreads:      0,
		ItemsProcessingThreads: 0,
		// TODO: Make contact e-mail into a CLI argument.
		UserAgent: fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision),
		Progress: func(ctx context.Context, p x.Progress) {
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s\n", p.Percent(), p.Remaining().Truncate(time.Second))
		},
	}, processArticle)
}
