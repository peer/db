package search

import (
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
)

// AggAs extracts a typed aggregation from a map of aggregations.
//
// TODO: Contribute upstream. See: https://github.com/elastic/go-elasticsearch/issues/1367
func AggAs[T any](aggs map[string]types.Aggregate, key string) (*T, errors.E) {
	raw, ok := aggs[key]
	if !ok {
		errE := errors.New("aggregation not found")
		errors.Details(errE)["key"] = key
		return nil, errE
	}
	typed, ok := raw.(*T)
	if !ok {
		errE := errors.New("unexpected aggregation type")
		errors.Details(errE)["key"] = key
		errors.Details(errE)["type"] = fmt.Sprintf("%T", raw)
		return nil, errE
	}
	return typed, nil
}
