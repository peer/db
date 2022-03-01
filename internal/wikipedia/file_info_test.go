package wikipedia

import (
	"context"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
)

func TestGetImageInfoForFilename(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()

	ii, err := getImageInfoForFilename(ctx, httpClient, "commons.wikimedia.org", "Logo_Google_2013_Official.svg")
	assert.NoError(t, err)
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
