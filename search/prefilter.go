package search

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// FullPathsResolver returns a value's full hierarchy paths in the indexed toFullPath form
// ("<hierarchyProp>:<root>/.../<id>"), for the caller's visibility level.
// A value with no value hierarchy returns no path.
type FullPathsResolver func(ctx context.Context, id identifier.Identifier) ([]string, errors.E)

// prefilterSubRefKey identifies a sub-reference facet by its parent property and property.
type prefilterSubRefKey struct {
	parentProp identifier.Identifier
	prop       identifier.Identifier
}

// PrefilterExcludes holds, per reference and sub-reference facet, the toFullPath values that an active
// prefilter makes redundant. When a prefilter pins a hierarchical value, every record that value expanded
// into (the value itself and each of its ancestors) carries that value's toFullPath. Dropping those
// records keeps a facet from re-counting the prefilter's own value hierarchy: only sibling and narrower
// values survive, and a facet left with no values is skipped entirely.
type PrefilterExcludes struct {
	ref    map[identifier.Identifier][]string
	subRef map[prefilterSubRefKey][]string
}

// Ref returns the toFullPath values to drop from a top-level reference facet on prop, or nil when no
// prefilter on prop contributes any.
func (e PrefilterExcludes) Ref(prop identifier.Identifier) []string {
	return e.ref[prop]
}

// SubRef returns the toFullPath values to drop from a sub-reference facet on (parentProp, prop), or nil
// when no prefilter on that combination contributes any.
func (e PrefilterExcludes) SubRef(parentProp, prop identifier.Identifier) []string {
	return e.subRef[prefilterSubRefKey{parentProp: parentProp, prop: prop}]
}

// refDiscoveryFilter matches claims.ref records except those a prefilter makes redundant: for each
// prefilter property, records whose prop matches and whose toFullPath is one of that prefilter value's
// paths. With no excludes it is an empty bool query, which matches every record.
func (e PrefilterExcludes) refDiscoveryFilter() types.QueryVariant { //nolint:ireturn
	clauses := make([]types.QueryVariant, 0, len(e.ref))
	for prop, paths := range e.ref {
		clauses = append(clauses, esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
			toFullPathTermsQuery("claims.ref", paths),
		))
	}
	return esdsl.NewBoolQuery().MustNot(clauses...)
}

// subRefDiscoveryFilter matches claims.subRef records except those a prefilter makes redundant: for each
// prefilter (parentProp, prop), records whose parentProp and prop match and whose toFullPath is one of
// that prefilter value's paths. With no excludes it matches every record.
func (e PrefilterExcludes) subRefDiscoveryFilter() types.QueryVariant { //nolint:ireturn
	clauses := make([]types.QueryVariant, 0, len(e.subRef))
	for key, paths := range e.subRef {
		clauses = append(clauses, esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.subRef.parentProp", esdsl.NewFieldValue().String(key.parentProp.String())),
			esdsl.NewTermQuery("claims.subRef.prop", esdsl.NewFieldValue().String(key.prop.String())),
			toFullPathTermsQuery("claims.subRef", paths),
		))
	}
	return esdsl.NewBoolQuery().MustNot(clauses...)
}

// PrefilterExcludeFullPaths resolves, from the session's reference prefilters, the toFullPath values each
// facet should drop. resolve returns a value's full hierarchy paths. Both the To and Direct values of
// a prefilter name a value whose hierarchy the facet must not re-count, so both contribute.
// Prefilters that are not reference filters do not create hierarchy redundancy and are skipped.
func (s *Session) PrefilterExcludeFullPaths(ctx context.Context, resolve FullPathsResolver) (PrefilterExcludes, errors.E) {
	excludes := PrefilterExcludes{ref: map[identifier.Identifier][]string{}, subRef: map[prefilterSubRefKey][]string{}}
	for i := range s.Prefilters {
		pf := &s.Prefilters[i]
		if pf.Ref == nil {
			continue
		}
		seen := map[string]bool{}
		var paths []string
		add := func(id identifier.Identifier) errors.E {
			resolved, errE := resolve(ctx, id)
			if errE != nil {
				errors.Details(errE)["id"] = id.String()
				if pf.ID != nil {
					errors.Details(errE)["filter"] = pf.ID.String()
				}
				return errE
			}
			for _, p := range resolved {
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
			return nil
		}
		for _, v := range pf.Ref.To {
			errE := add(v.ID)
			if errE != nil {
				return PrefilterExcludes{}, errE
			}
		}
		for _, v := range pf.Ref.Direct {
			errE := add(v.ID)
			if errE != nil {
				return PrefilterExcludes{}, errE
			}
		}
		if len(paths) == 0 {
			continue
		}
		switch len(pf.Prop) {
		case 1:
			excludes.ref[pf.Prop[0]] = append(excludes.ref[pf.Prop[0]], paths...)
		case 2: //nolint:mnd
			key := prefilterSubRefKey{parentProp: pf.Prop[0], prop: pf.Prop[1]}
			excludes.subRef[key] = append(excludes.subRef[key], paths...)
		default:
			// This should not be possible: a reference prefilter is validated to have either one property
			// (top-level) or two (sub-reference).
			errE := errors.New("invalid prefilter property length")
			errors.Details(errE)["length"] = len(pf.Prop)
			errors.Details(errE)["prop"] = pf.Prop
			if pf.ID != nil {
				errors.Details(errE)["filter"] = pf.ID.String()
			}
			panic(errE)
		}
	}
	return excludes, nil
}

// toFullPathTermsQuery matches reference records on the given nested field ("claims.ref" or
// "claims.subRef") whose toFullPath is one of paths. It is the prefilter exclusion clause for a facet
// whose property is already constrained by the surrounding aggregation filter.
func toFullPathTermsQuery(field string, paths []string) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, len(paths))
	for i, p := range paths {
		values[i] = esdsl.NewFieldValue().String(p)
	}
	return esdsl.NewTermsQuery().AddTermsQuery(field+".toFullPath", esdsl.NewTermsQueryField().FieldValues(values...))
}
