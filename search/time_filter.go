package search

import (
	"context"
	"math"
	"strconv"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// TimeFilterGet retrieves time filter data for search results.
func TimeFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64, int64), id, prop identifier.Identifier,
) ([]HistogramResult[int64], map[string]interface{}, errors.E) {
	filter := elastic.NewTermQuery("claims.time.prop", prop)
	return histogramFilterGet(
		ctx, getSearchService, id,
		"claims.time", filter,
		"claims.time.from", "claims.time.to", "claims.time.range",
		func(v int64) string { return strconv.FormatInt(v, 10) },
		func(from, to int64) (map[string]histogramRange[int64], string) {
			interval := int64(math.Ceil(float64(to-from) / float64(histogramBins)))
			bins := int(math.Ceil(float64(to-from) / float64(interval)))
			ranges := make(map[string]histogramRange[int64], bins)
			for i := range bins {
				ranges[strconv.Itoa(i)] = histogramRange[int64]{
					From: from + int64(i)*interval,
					To:   from + int64(i+1)*interval,
				}
			}
			return ranges, strconv.FormatInt(interval, 10)
		},
	)
}
