package eprel

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
	"gitlab.com/peerdb/peerdb/indexer"
	"gitlab.com/tozd/go/cli"
)

// NewHTTPClient creates a retryable HTTP client with the specified base HTTP client and logger.
func NewHTTPClient(logger zerolog.Logger) *http.Client {
	// TODO: Make contact e-mail into a CLI argument.
	return indexer.NewHTTPClient(logger, nil, fmt.Sprintf("OpenGoods/%s (build on %s, git revision %s) (mailto:mitar.opengoods@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision)) //nolint:lll
}
