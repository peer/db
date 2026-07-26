package search

import (
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/fieldtype"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortmode"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// defaultSortTail is the stable default ordering appended after the user's sort keys (and used alone
// when no sort is configured): relevance first, then the document's earliest time (newest first), then
// its display label in the session's language. Documents missing a sort field sort last.
func defaultSortTail(lang string) []types.SortCombinationsVariant {
	return []types.SortCombinationsVariant{
		esdsl.NewSortOptions().Score_(esdsl.NewScoreSort().Order(sortorder.Desc)),
		esdsl.NewSortOptions().AddSortOption("time", esdsl.NewFieldSort(sortorder.Desc).Missing(esdsl.NewMissing().String("_last"))),
		esdsl.NewSortOptions().AddSortOption(
			"displaySort."+lang,
			// unmapped_type keeps the sort working if the field for the language is not present in the index mapping.
			esdsl.NewFieldSort(sortorder.Asc).UnmappedType(fieldtype.Keyword).Missing(esdsl.NewMissing().String("_last")),
		),
	}
}

// sortKeyToOption translates one sort key to an ElasticSearch sort option, or returns nil for an
// unknown column. Filter columns sort on a nested claim field scoped to the column's property: ref
// columns by their value's sort key (claims.rel.toPathSortKey, the folded display label with an id tiebreaker),
// amount/time columns by the numeric "from" endpoint. mode min (ascending) / max (descending) picks
// which of a document's several values for the property decides its position.
func sortKeyToOption(k SortKey, lang string) types.SortCombinationsVariant { //nolint:ireturn
	order := sortorder.Asc
	mode := sortmode.Min
	if k.Descending {
		order = sortorder.Desc
		mode = sortmode.Max
	}

	if !k.IsFilter() {
		switch k.Type {
		case SortScore:
			return esdsl.NewSortOptions().Score_(esdsl.NewScoreSort().Order(order))
		case SortTime:
			return esdsl.NewSortOptions().AddSortOption("time", esdsl.NewFieldSort(order).Missing(esdsl.NewMissing().String("_last")))
		case SortLabel:
			return esdsl.NewSortOptions().AddSortOption(
				"displaySort."+lang,
				esdsl.NewFieldSort(order).UnmappedType(fieldtype.Keyword).Missing(esdsl.NewMissing().String("_last")),
			)
		}
		return nil
	}

	prop := k.Prop[0]
	var path, field string
	musts := []types.QueryVariant{}
	keyword := false
	switch k.Type {
	case "ref":
		path = relPath
		field = relPath + ".toPathSortKey." + lang
		keyword = true
		musts = append(musts,
			esdsl.NewTermQuery(relPath+".claimType", esdsl.NewFieldValue().String(internalSearch.ClaimTypeRef)),
			esdsl.NewTermQuery(relPath+".prop", esdsl.NewFieldValue().String(prop)))
	case "amount":
		path = amountPath
		field = amountPath + ".from"
		musts = append(musts, esdsl.NewTermQuery(amountPath+".prop", esdsl.NewFieldValue().String(prop)))
		if k.Unit != "" {
			musts = append(musts, esdsl.NewTermQuery(amountPath+".unit", esdsl.NewFieldValue().String(k.Unit)))
		}
	case "time":
		path = timePath
		field = timePath + ".from"
		musts = append(musts, esdsl.NewTermQuery(timePath+".prop", esdsl.NewFieldValue().String(prop)))
	default:
		return nil
	}

	fieldSort := esdsl.NewFieldSort(order).
		Mode(mode).
		Nested(esdsl.NewNestedSortValue().Path(path).Filter(esdsl.NewBoolQuery().Must(musts...))).
		Missing(esdsl.NewMissing().String("_last"))
	if keyword {
		fieldSort = fieldSort.UnmappedType(fieldtype.Keyword)
	}
	return esdsl.NewSortOptions().AddSortOption(field, fieldSort)
}

// buildSort builds the ElasticSearch sort options for the given sort keys, followed by the default tail
// so ties always resolve to a stable order. With no keys it returns just the default tail (the previous
// hard-coded behavior).
func buildSort(keys []SortKey, lang string) []types.SortCombinationsVariant {
	sorts := make([]types.SortCombinationsVariant, 0, len(keys)+3) //nolint:mnd
	for i := range keys {
		opt := sortKeyToOption(keys[i], lang)
		if opt != nil {
			sorts = append(sorts, opt)
		}
	}
	return append(sorts, defaultSortTail(lang)...)
}
