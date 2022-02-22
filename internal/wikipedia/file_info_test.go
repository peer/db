package wikipedia

import (
	"context"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
)

func TestGetImageInfo(t *testing.T) {
	ctx := context.Background()
	httpClient := retryablehttp.NewClient()

	ii, err := getImageInfo(ctx, httpClient, "Logo_Google_2013_Official.svg")
	assert.NoError(t, err)
	assert.Equal(t, imageInfo{
		Mime:      "image/svg+xml",
		Size:      6380,
		Width:     750,
		Height:    258,
		PageCount: 0,
		Duration:  0,
		Redirect:  "Google_logo_(2013-2015).svg",
	}, ii)
}
