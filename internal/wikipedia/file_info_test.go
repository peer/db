package wikipedia

import (
	"context"
	"testing"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/peerdb/peerdb/internal/es"
)

func TestGetImageInfoForFilename(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	ctx := context.Background()
	httpClient := es.NewHTTPClient(cleanhttp.DefaultPooledClient(), logger)

	ii, errE := getImageInfoForFilename(ctx, httpClient, "commons.wikimedia.org", "", 50, "Logo_Google_2013_Official.svg")
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, ImageInfo{
		Mime:                "image/svg+xml",
		Size:                6380,
		Width:               750,
		Height:              258,
		PageCount:           0,
		Duration:            0,
		URL:                 "https://upload.wikimedia.org/wikipedia/commons/c/c9/Google_logo_%282013-2015%29.svg",
		DescriptionURL:      "https://commons.wikimedia.org/wiki/File:Google_logo_(2013-2015).svg",
		DescriptionShortURL: "https://commons.wikimedia.org/w/index.php?curid=29869044",
		Redirect:            "Google_logo_(2013-2015).svg",
	}, ii)
}
