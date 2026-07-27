package search

import (
	"slices"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

// ReadAccessQuery returns the filter matching the documents a caller with the given roles and subject
// may read: documents whose readableByRoles field contains one of the roles (pass the reserved
// everyone role among them, its grants apply to every caller) or whose readableByUsers field contains
// the subject. An empty subject (an anonymous caller) matches through roles only. Both fields are
// computed at indexing time by the converter with the same auth code the read path runs, so the
// filter admits exactly the documents the read path allows the caller to read.
func ReadAccessQuery(roles []string, subject string) types.QueryVariant { //nolint:ireturn
	// Sorted, so an equal set of roles produces an identical query (also for the query cache).
	roles = slices.Clone(roles)
	slices.Sort(roles)
	values := make([]types.FieldValueVariant, 0, len(roles))
	for _, role := range slices.Compact(roles) {
		values = append(values, esdsl.NewFieldValue().String(role))
	}
	should := []types.QueryVariant{
		esdsl.NewTermsQuery().AddTermsQuery("readableByRoles", esdsl.NewTermsQueryField().FieldValues(values...)),
	}
	if subject != "" {
		should = append(should, esdsl.NewTermQuery("readableByUsers", esdsl.NewFieldValue().String(subject)))
	}
	return esdsl.NewBoolQuery().Should(should...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}
