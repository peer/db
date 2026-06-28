package search

import (
	"context"
	"fmt"
	"slices"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
	"gitlab.com/peerdb/peerdb/store"
)

// ClassCreateOption is one class offered in the create-document view. Paths are the SUBCLASS_OF ancestor
// chains (root to immediate parent), one entry per parent class, so the frontend renders the class once
// under each parent (matching the instance-of filter tree). CanCreate is true when a document can be
// created for the class (it is not abstract and it defines fields); a non-creatable class is included only
// as a structural ancestor of a creatable one.
type ClassCreateOption struct {
	ID        string     `json:"id"`
	Paths     [][]string `json:"paths,omitempty"`
	CanCreate bool       `json:"canCreate"`
}

// classCreatable reports whether a document can be created for the class: it must not be abstract and must
// define at least one field or section under its FIELDS has-claim. This mirrors isAbstractClass and
// hasFields in the frontend (src/fields.ts) and reads claims the same way buildFieldInverseProperties does.
func classCreatable(cls *document.D) bool {
	if cls == nil {
		return false
	}
	if len(document.GetClaimsOfTypeWithConfidence[document.HasClaim](cls, internalCore.AbstractClassPropID, document.LowConfidence)) > 0 {
		return false
	}
	for _, fields := range document.GetClaimsOfTypeWithConfidence[document.HasClaim](cls, internalCore.FieldsPropID, document.LowConfidence) {
		if len(document.GetClaimsOfTypeWithConfidence[document.HasClaim](fields, internalCore.FieldPropID, document.LowConfidence)) > 0 ||
			len(document.GetClaimsOfTypeWithConfidence[document.HasClaim](fields, internalCore.SectionPropID, document.LowConfidence)) > 0 {
			return true
		}
	}
	return false
}

// ancestorChains parses a document's indexed full hierarchy paths (the documentFullPaths form
// "<hierProp>:<root>/.../<this>") into ancestor chains (root to immediate parent), dropping paths
// without ancestors. It is the documentFullPaths analogue of collectPaths.
func ancestorChains(fullPaths []string) [][]string {
	if len(fullPaths) == 0 {
		return nil
	}
	out := make([][]string, 0, len(fullPaths))
	for _, raw := range fullPaths {
		ancestors := parseToPath(raw)
		if len(ancestors) == 0 {
			continue
		}
		out = append(out, ancestors)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// pathsContain reports whether id appears anywhere in the given ancestor chains.
func pathsContain(paths [][]string, id string) bool {
	for _, path := range paths {
		if slices.Contains(path, id) {
			return true
		}
	}
	return false
}

// CreateOptions returns the classes to offer in the create-document view, ordered for tree rendering.
//
// The set is every class (a document that is an instance of the core CLASS), each tagged with whether a
// document can be created for it (classCreatable). Classes whose whole subtree contains no creatable class
// are pruned, since the create view has nothing to offer under them. Instance-document counts (how many
// documents are an instance of the class or any of its sub-classes) order the result so classes with many
// documents come first and zero-document classes come last; the counts themselves are not returned.
//
// accessFilter, when non-nil, scopes both the enumerated classes and the counts to the documents the
// caller may access. loadDocument reads a class document (returning a nil document, no error, when it is
// not found or not accessible, so the class is skipped) to determine createability, and documentFullPaths
// resolves its SUBCLASS_OF ancestor paths.
//
// When limit is non-empty, the offering is restricted to that class id and its descendants: only they keep
// their creatability, the limit class's ancestors are kept solely as structural labels (forced
// non-creatable) so the tree still renders from a root down to the limit, and every other class is dropped.
// An unknown limit id yields an empty result.
func CreateOptions(
	ctx context.Context,
	getSearchService func() *esSearch.Search,
	accessFilter types.QueryVariant,
	loadDocument func(context.Context, identifier.Identifier) (*document.D, errors.E),
	documentFullPaths func(context.Context, identifier.Identifier) ([]string, errors.E),
	limit string,
) ([]ClassCreateOption, errors.E) {
	counts, errE := instanceCounts(ctx, getSearchService, accessFilter)
	if errE != nil {
		return nil, errE
	}
	ids, errE := classIDs(ctx, getSearchService, accessFilter)
	if errE != nil {
		return nil, errE
	}

	type classEntry struct {
		res       RefFilterResult
		canCreate bool
	}
	entries := make([]classEntry, 0, len(ids))
	for _, id := range ids {
		doc, errE := loadDocument(ctx, id)
		if errE != nil {
			// A class enumerated a moment ago may be gone or hidden by the caller's access level; skip it
			// rather than failing the whole listing.
			if errors.Is(errE, store.ErrValueNotFound) || errors.Is(errE, store.ErrAccessDenied) {
				continue
			}
			return nil, errE
		}
		if doc == nil {
			continue
		}
		fullPaths, errE := documentFullPaths(ctx, id)
		if errE != nil {
			return nil, errE
		}
		entries = append(entries, classEntry{
			res: RefFilterResult{
				ID:         id.String(),
				Count:      counts[id.String()],
				ChildCount: 0,
				Paths:      ancestorChains(fullPaths),
			},
			canCreate: classCreatable(doc),
		})
	}

	if limit != "" {
		// Find the limit class so its ancestors can be kept as labels. A nil set after the loop means the
		// limit id is not a known class.
		var limitAncestors map[string]bool
		for i := range entries {
			if entries[i].res.ID != limit {
				continue
			}
			limitAncestors = map[string]bool{}
			for _, path := range entries[i].res.Paths {
				for _, ancestor := range path {
					limitAncestors[ancestor] = true
				}
			}
			break
		}
		if limitAncestors == nil {
			return []ClassCreateOption{}, nil
		}
		// Keep the limit class and its descendants with their own creatability, and the limit class's
		// ancestors as non-creatable labels; drop everything else.
		scoped := make([]classEntry, 0, len(entries))
		for _, e := range entries {
			switch {
			case e.res.ID == limit:
			case limitAncestors[e.res.ID]:
				e.canCreate = false
			case pathsContain(e.res.Paths, limit):
			default:
				continue
			}
			scoped = append(scoped, e)
		}
		entries = scoped
	}

	// Collect every class that is an ancestor of a creatable class; these are kept as structural nodes.
	creatableAncestors := map[string]bool{}
	for _, e := range entries {
		if !e.canCreate {
			continue
		}
		for _, path := range e.res.Paths {
			for _, ancestor := range path {
				creatableAncestors[ancestor] = true
			}
		}
	}

	// Prune classes whose subtree holds no creatable class: keep a class only if it is itself creatable or
	// it is an ancestor of a creatable class. The kept set is closed under ancestry (a creatable class's
	// ancestors are all marked above), so no kept class ever references a dropped parent.
	kept := make([]classEntry, 0, len(entries))
	for _, e := range entries {
		if e.canCreate || creatableAncestors[e.res.ID] {
			kept = append(kept, e)
		}
	}

	// Order for tree rendering: by instance count descending, then hierarchy depth ascending, the same
	// ordering the reference filter uses, so ancestors precede descendants and busier classes come first.
	slices.SortStableFunc(kept, func(a, b classEntry) int {
		return compareRefFilterResults(a.res, b.res)
	})

	options := make([]ClassCreateOption, 0, len(kept))
	for _, e := range kept {
		options = append(options, ClassCreateOption{
			ID:        e.res.ID,
			Paths:     e.res.Paths,
			CanCreate: e.canCreate,
		})
	}
	return options, nil
}

// instanceCounts aggregates, over the accessible corpus, the number of documents that are an instance of
// each class (including its sub-classes, via the index-time ancestor expansion of INSTANCE_OF references).
// It returns a map from class id to document count; classes with no instances are simply absent.
func instanceCounts(ctx context.Context, getSearchService func() *esSearch.Search, accessFilter types.QueryVariant) (map[string]int64, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	var query types.QueryVariant = esdsl.NewMatchAllQuery()
	if accessFilter != nil {
		query = esdsl.NewBoolQuery().Filter(accessFilter)
	}

	agg := esdsl.NewAggregations().
		Nested(esdsl.NewNestedAggregation().Path("claims.ref")).
		AddAggregation("filter", esdsl.NewAggregations().
			Filter(esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(internalCore.InstanceOfPropID.String()))).
			AddAggregation("props", esdsl.NewAggregations().
				Terms(esdsl.NewTermsAggregation().Field("claims.ref.to").Size(MaxResultsCount)).
				AddAggregation("docs", esdsl.NewAggregations().
					ReverseNested(esdsl.NewReverseNestedAggregation()))))

	searchService := getSearchService().Size(0).Query(query).AddAggregation("ref", agg)

	// CreateOptions runs two ES searches per request (this one and classIDs), so they use distinct metric
	// keys, the same way the amount filter's two searches do.
	m := metrics.Duration(internalStore.MetricElasticSearch1).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal1).Duration = time.Duration(res.Took) * time.Millisecond

	refNested, errE := internalSearch.AggAs[types.NestedAggregate](res.Aggregations, "ref")
	if errE != nil {
		return nil, errE
	}
	refFilter, errE := internalSearch.AggAs[types.FilterAggregate](refNested.Aggregations, "filter")
	if errE != nil {
		return nil, errE
	}
	refTerms, errE := internalSearch.AggAs[types.StringTermsAggregate](refFilter.Aggregations, "props")
	if errE != nil {
		return nil, errE
	}
	buckets, ok := refTerms.Buckets.([]types.StringTermsBucket)
	if !ok {
		errE := errors.New("unexpected bucket type for instance counts")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", refTerms.Buckets)
		return nil, errE
	}

	counts := make(map[string]int64, len(buckets))
	for _, bucket := range buckets {
		key, ok := bucket.Key.(string)
		if !ok {
			continue
		}
		docs, errE := internalSearch.AggAs[types.ReverseNestedAggregate](bucket.Aggregations, "docs")
		if errE != nil {
			return nil, errE
		}
		counts[key] = docs.DocCount
	}
	return counts, nil
}

// classIDs enumerates every class: a document that is an instance of the core CLASS. Because INSTANCE_OF
// references are expanded over the SUBCLASS_OF hierarchy at index time, a document that is an instance of
// any sub-class of CLASS (a class declared via a metaclass) also matches, so this captures all classes.
func classIDs(ctx context.Context, getSearchService func() *esSearch.Search, accessFilter types.QueryVariant) ([]identifier.Identifier, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	filters := []types.QueryVariant{
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(internalCore.InstanceOfPropID.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(internalCore.ClassClassID.String())),
			),
		).Path("claims.ref"),
	}
	if accessFilter != nil {
		filters = append(filters, accessFilter)
	}

	searchService := getSearchService().Size(MaxResultsCount).Query(esdsl.NewBoolQuery().Filter(filters...))

	m := metrics.Duration(internalStore.MetricElasticSearch2).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, WithESError(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal2).Duration = time.Duration(res.Took) * time.Millisecond

	ids := make([]identifier.Identifier, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		if hit.Id_ == nil {
			continue
		}
		id, errE := identifier.MaybeString(*hit.Id_)
		if errE != nil {
			return nil, errE
		}
		ids = append(ids, id)
	}
	return ids, nil
}
