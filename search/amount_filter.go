package search

import (
	"context"
	"math"
	"strconv"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// computeInterval computes the histogram interval.
//
// It ensures that exactly histogramBins buckets are produced by slightly widening
// the interval so that the max value falls inside the last bucket, not in a 101st bucket.
func computeInterval(from, to float64) (float64, float64, string) {
	// Bins are intervals [from, to). So for upperBound we want the next value after "to".
	upperBound := math.Nextafter(to, math.Inf(1))
	interval := (upperBound - from) / float64(histogramBins)
	interval2 := (to - from) / float64(histogramBins)
	if interval == interval2 {
		// The difference between upperBound and "to" was too small so the interval does not represent it.
		// Let's increase the interval to the next value to make sure "to" falls inside the last bin
		// and is not moved into its own bin.
		interval = math.Nextafter(interval, math.Inf(1))
	}
	// Extended bounds include both endpoints, interval [min, max], so we return "to" as the upper bound
	// (to not include the upperBound which we used to compute the interval).
	return interval, to, strconv.FormatFloat(interval, 'f', -1, 64)
}

// histogramCountsOrder is the order the identity and special-value counts fold into histogram facet
// metadata: the exists count (documents with an actual value record for the facet), the specials,
// the missing count, and the universe size, whose identity is exists + unknown + none + missing =
// universe (with the usual multi-statement caveat) plus the other-value-types remainder.
var histogramCountsOrder = []string{existsKey, hasPropertyKey, unknownKey, noneKey, missingKey, universeKey, otherTypesKey} //nolint:gochecknoglobals

// topHistogramCounts builds the identity and special-value count queries of a top-level amount or
// time facet. existsQuery matches documents with an actual value record for the facet (including
// its unit condition for amounts).
func topHistogramCounts(prop identifier.Identifier, collection string, existsQuery types.QueryVariant) map[string]types.QueryVariant {
	return map[string]types.QueryVariant{
		existsKey:      existsQuery,
		hasPropertyKey: TopSpecialQuery(prop, internalSearch.ClaimTypeHas),
		unknownKey:     TopSpecialQuery(prop, internalSearch.ClaimTypeUnknown),
		noneKey:        TopSpecialQuery(prop, internalSearch.ClaimTypeNone),
		missingKey:     TopMissingQuery(prop),
		universeKey:    esdsl.NewMatchAllQuery(),
		otherTypesKey:  TopOtherTypesQuery(prop, collection),
	}
}

// subHistogramCounts builds the identity and special-value count queries of a sub amount or time
// facet under the given parent context.
func subHistogramCounts(parentCtx *ParentContext, prop identifier.Identifier, collection string, existsQuery types.QueryVariant) map[string]types.QueryVariant {
	return map[string]types.QueryVariant{
		existsKey:      existsQuery,
		hasPropertyKey: parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeHas),
		unknownKey:     parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeUnknown),
		noneKey:        parentCtx.SpecialQuery(prop, internalSearch.ClaimTypeNone),
		missingKey:     parentCtx.MissingQuery(prop),
		universeKey:    parentCtx.ExistsQuery(),
		otherTypesKey:  parentCtx.OtherTypesQuery(prop, collection),
	}
}

// subValueExistsQuery matches documents with a value record for the sub facet (prop, with the
// per-collection valueFilter applied at the value level, for example the amount unit condition)
// under a qualifying parent claim. valueFilter receives the value collection's path (which differs
// per parent collection) and may be nil.
//
//nolint:ireturn
func subValueExistsQuery(
	parentCtx *ParentContext, collection string, prop identifier.Identifier, valueFilter func(path string) types.QueryVariant,
) types.QueryVariant {
	var arms []types.QueryVariant
	for _, parent := range parentCtx.Collections() {
		pf, ok := parentCtx.CollectionFilter(parent)
		if !ok {
			continue
		}
		path := subPath(parent, collection)
		musts := []types.QueryVariant{propTerm(path, prop)}
		if valueFilter != nil {
			if vf := valueFilter(path); vf != nil {
				musts = append(musts, vf)
			}
		}
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			pf,
			esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(musts...)).Path(path),
		)).Path(parentPath(parent)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// Get retrieves amount filter data for a top-level property facet: the histogram (or single
// bucket), the identity counts (exists, specials, missing, universe, other value types), and the
// range metadata.
func (f *AmountFilter) Get(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier,
) ([]HistogramResult, map[string]any, errors.E) {
	filter := esdsl.NewBoolQuery().Must(
		propTerm(amountPath, prop),
		unitTerm(amountPath, f.Unit),
	)
	contexts := []histContext{{
		Name:         "amount",
		Parent:       "",
		Path:         amountPath,
		ParentFilter: nil,
		Filter:       filter,
	}}
	existsQuery := esdsl.NewNestedQuery(filter).Path(amountPath)
	return histogramGet(
		ctx, getSearchService, query, contexts,
		f.Gte, f.Lte,
		amountStepDown,
		topHistogramCounts(prop, "amount", existsQuery), histogramCountsOrder,
	)
}

// GetSubAmount retrieves amount filter data for a sub facet: the histogram merged across the
// parent collections the context allows, the identity counts, and the range metadata. parentCtx
// scopes every aggregation to qualifying parent claims.
func (f *AmountFilter) GetSubAmount(
	ctx context.Context, getSearchService func() *esSearch.Search,
	query types.QueryVariant, prop identifier.Identifier, parentCtx *ParentContext,
) ([]HistogramResult, map[string]any, errors.E) {
	var contexts []histContext
	for _, parent := range parentCtx.Collections() {
		pf, ok := parentCtx.CollectionFilter(parent)
		if !ok {
			continue
		}
		path := subPath(parent, "amount")
		contexts = append(contexts, histContext{
			Name:         "amount:" + parent,
			Parent:       parent,
			Path:         path,
			ParentFilter: pf,
			Filter:       esdsl.NewBoolQuery().Must(propTerm(path, prop), unitTerm(path, f.Unit)),
		})
	}
	existsQuery := subValueExistsQuery(parentCtx, "amount", prop, func(path string) types.QueryVariant {
		return unitTerm(path, f.Unit)
	})
	return histogramGet(
		ctx, getSearchService, query, contexts,
		f.Gte, f.Lte,
		amountStepDown,
		subHistogramCounts(parentCtx, prop, "amount", existsQuery), histogramCountsOrder,
	)
}
