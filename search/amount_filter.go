package search

import (
	"context"
	"strconv"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// amountUnitFilter returns a query that matches the unit field.
// If unit is provided, it matches the exact value. If nil, it matches documents where unit does not exist.
func amountUnitFilter(unit *identifier.Identifier) elastic.Query { //nolint:ireturn
	if unit != nil {
		return elastic.NewTermQuery("claims.amount.unit", *unit)
	}
	return elastic.NewBoolQuery().MustNot(elastic.NewExistsQuery("claims.amount.unit"))
}

// AmountFilterGet retrieves amount filter data for search results.
func AmountFilterGet(
	ctx context.Context, getSearchService func() (*elastic.SearchService, int64, int64), id, prop identifier.Identifier, unit *identifier.Identifier,
) ([]HistogramResult[float64], map[string]interface{}, errors.E) {
	filter := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("claims.amount.prop", prop),
		amountUnitFilter(unit),
	)
	return histogramFilterGet(
		ctx, getSearchService, id,
		"claims.amount", filter,
		"claims.amount.from", "claims.amount.to", "claims.amount.range",
		func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) },
		func(from, to float64) (map[string]histogramRange[float64], string) {
			interval := (to - from) / float64(histogramBins)
			ranges := make(map[string]histogramRange[float64], histogramBins)
			for i := range histogramBins {
				ranges[strconv.Itoa(i)] = histogramRange[float64]{
					From: from + float64(i)*interval,
					To:   from + float64(i+1)*interval,
				}
			}
			return ranges, strconv.FormatFloat(interval, 'f', -1, 64)
		},
	)
}
