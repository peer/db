package wikipedia

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

var NameSpaceWikipediaFile = uuid.MustParse("94b1c372-bc28-454c-a45a-2e4d29d15146")

func ConvertWikipediaImage(ctx context.Context, httpClient *retryablehttp.Client, image Image) (*search.Document, errors.E) {
	return convertImage(ctx, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", "ENGLISH_WIKIPEDIA", image)
}
