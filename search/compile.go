package search

import (
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"gitlab.com/tozd/identifier"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// Claims collections and sub paths. Every claims collection hosts a "sub" container with the same
// collection shapes nested one level inside, so a sub-claim condition compiles as a nested query on
// the sub path inside a nested query on its parent collection, which is what correlates conditions
// on the same parent claim.
const (
	relPath    = "claims.rel"
	amountPath = "claims.amount"
	timePath   = "claims.time"

	// relCollection is the name of the rel collection, used both as a parent collection name and as
	// a sub collection name under a parent.
	relCollection = "rel"
)

// parentCollections lists the claims collections, each a possible parent of sub-claims.
var parentCollections = internalSearch.ParentCollections() //nolint:gochecknoglobals

// parentPath returns the nested path of a parent collection.
func parentPath(parent string) string {
	return "claims." + parent
}

// subPath returns the nested path of one sub collection under a parent collection.
func subPath(parent, collection string) string {
	return "claims." + parent + ".sub." + collection
}

// term returns a term query on field for a string value.
func term(field, value string) types.QueryVariant { //nolint:ireturn
	return esdsl.NewTermQuery(field, esdsl.NewFieldValue().String(value))
}

// propTerm returns a term query on the prop field of the given nested path.
func propTerm(path string, prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return term(path+".prop", prop.String())
}

// claimTypeTerm returns a term query on the claimType field of the given rel path. It is needed
// for has, none, and unknown conditions; ref conditions imply the claimType through the to field
// (the other claim types have no to), so they carry no claimType term.
func claimTypeTerm(path, claimType string) types.QueryVariant { //nolint:ireturn
	return term(path+".claimType", claimType)
}

// oneOrShould returns the single clause, or the clauses OR'd with minimum_should_match 1.
func oneOrShould(clauses []types.QueryVariant) types.QueryVariant { //nolint:ireturn
	if len(clauses) == 1 {
		return clauses[0]
	}
	return esdsl.NewBoolQuery().Should(clauses...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// matchNone returns a query matching no documents, used when a selection is unsatisfiable by
// construction (for example a group whose parent constraints exclude every parent collection).
func matchNone() types.QueryVariant { //nolint:ireturn
	return esdsl.NewBoolQuery().MustNot(esdsl.NewMatchAllQuery())
}

// unitTerm returns the unit condition for an amount path: a term on the exact unit when set, or a
// must_not exists when the filter selects the unitless facet.
func unitTerm(path string, unit *identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if unit != nil {
		return term(path+".unit", unit.String())
	}
	return esdsl.NewBoolQuery().MustNot(esdsl.NewExistsQuery().Field(path + ".unit"))
}

// amountArms compiles an amount filter's arms against the amount collection at the given path: a
// range arm (the value window intersecting the selection, with the unit term when the filter has
// one) or an exists arm (any value record, with the exact-or-absent unit condition).
func amountArms(path string, f *AmountFilter) []types.QueryVariant {
	if f.Exists {
		return []types.QueryVariant{esdsl.NewBoolQuery().Must(unitTerm(path, f.Unit))}
	}
	musts := []types.QueryVariant{
		esdsl.NewNumberRangeQuery(path + ".range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte)),
	}
	if f.Unit != nil {
		musts = append(musts, term(path+".unit", f.Unit.String()))
	}
	if len(musts) == 1 {
		return musts
	}
	return []types.QueryVariant{esdsl.NewBoolQuery().Must(musts...)}
}

// timeArms compiles a time filter's arms against the time collection at the given path: a range
// arm, or an exists arm (any value record, compiled as match_all because the prop term is added by
// the caller).
func timeArms(path string, f *TimeFilter) []types.QueryVariant {
	if f.Exists {
		return []types.QueryVariant{esdsl.NewMatchAllQuery()}
	}
	return []types.QueryVariant{esdsl.NewNumberRangeQuery(path + ".range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))}
}

// facetableSubPresence matches, within the given parent collection's claim context, any facetable
// sub-claim for prop: a rel record of any claimType, an amount record, or a time record. It is the
// presence side of the sub-level missing definition (text-only sub-claims do not block missing).
func facetableSubPresence(parent string, prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return oneOrShould([]types.QueryVariant{
		esdsl.NewNestedQuery(propTerm(subPath(parent, relCollection), prop)).Path(subPath(parent, relCollection)),
		esdsl.NewNestedQuery(propTerm(subPath(parent, "amount"), prop)).Path(subPath(parent, "amount")),
		esdsl.NewNestedQuery(propTerm(subPath(parent, "time"), prop)).Path(subPath(parent, "time")),
	})
}

// topFacetablePresenceClauses returns, per facetable collection, a query matching documents with
// any facetable claim for prop. Their disjunction is the top-level universe-presence condition; a
// document matching none of them is missing the property.
func topFacetablePresenceClauses(prop identifier.Identifier) []types.QueryVariant {
	return []types.QueryVariant{
		esdsl.NewNestedQuery(propTerm(relPath, prop)).Path(relPath),
		esdsl.NewNestedQuery(propTerm(amountPath, prop)).Path(amountPath),
		esdsl.NewNestedQuery(propTerm(timePath, prop)).Path(timePath),
	}
}

// topMissingQuery matches documents that state nothing facetable for prop: no rel record of any
// claimType, no amount record, and no time record. Text-only claims do not block missing.
func topMissingQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return esdsl.NewBoolQuery().MustNot(topFacetablePresenceClauses(prop)...)
}

// parentConstraints holds the parent-claim selection arms contributed by the same filter set's
// top-level selections on a group's parent property, per parent collection. When any arm
// exists the group compiles only for the collections that have arms: the parent claim must itself
// satisfy the top-level selection, so a collection the selection cannot match contributes nothing.
// The same participation rule scopes the sub paths' universe (parentExists and the standalone
// missing), so with a top-level selection the universe narrows from all attachment points to the
// parent claims satisfying the selection.
type parentConstraints struct {
	Rel    []types.QueryVariant
	Amount []types.QueryVariant
	Time   []types.QueryVariant
}

// Any reports whether Any constraint arm exists.
func (c *parentConstraints) Any() bool {
	return len(c.Rel) > 0 || len(c.Amount) > 0 || len(c.Time) > 0
}

// ForCollection returns the constraint arms for the given parent collection and whether the
// collection participates in the group. Without any constraints every collection participates
// unconstrained; with constraints only the collections that have arms do.
func (c *parentConstraints) ForCollection(parent string) ([]types.QueryVariant, bool) {
	if !c.Any() {
		return nil, true
	}
	switch parent {
	case relCollection:
		return c.Rel, len(c.Rel) > 0
	case "amount":
		return c.Amount, len(c.Amount) > 0
	case "time":
		return c.Time, len(c.Time) > 0
	default:
		return nil, false
	}
}

// topPath aggregates the active top-level filters of one property path within one filter set.
type topPath struct {
	Prop     identifier.Identifier
	Refs     []*RefFilter
	Amounts  []*AmountFilter
	Times    []*TimeFilter
	Specials *SpecialsFilter
}

// RelArms compiles the path's selection arms that apply to rel records (used both for the
// path's own clause and as parent constraint arms inside a group): to terms for To values, to plus
// isLeaf for Direct values, and claimType terms for the none/unknown/hasProperty specials. Missing
// contributes no arm here.
func (t *topPath) RelArms() []types.QueryVariant {
	var arms []types.QueryVariant
	for _, f := range t.Refs {
		for _, to := range f.To {
			arms = append(arms, term(relPath+".to", to.ID.String()))
		}
		for _, to := range f.Direct {
			arms = append(arms, esdsl.NewBoolQuery().Must(
				term(relPath+".to", to.ID.String()),
				esdsl.NewTermQuery(relPath+".isLeaf", esdsl.NewFieldValue().Bool(true)),
			))
		}
	}
	if t.Specials != nil {
		if t.Specials.None {
			arms = append(arms, claimTypeTerm(relPath, internalSearch.ClaimTypeNone))
		}
		if t.Specials.Unknown {
			arms = append(arms, claimTypeTerm(relPath, internalSearch.ClaimTypeUnknown))
		}
		if t.Specials.HasProperty {
			arms = append(arms, claimTypeTerm(relPath, internalSearch.ClaimTypeHas))
		}
	}
	return arms
}

// Constraints returns the path's selections as parent constraint arms for a group on the path's
// property. The missing special contributes nothing: its own clause already makes the conjunction
// with any positive sub condition empty.
func (t *topPath) Constraints() parentConstraints {
	c := parentConstraints{Rel: nil, Amount: nil, Time: nil}
	c.Rel = t.RelArms()
	for _, f := range t.Amounts {
		c.Amount = append(c.Amount, amountArms(amountPath, f)...)
	}
	for _, f := range t.Times {
		c.Time = append(c.Time, timeArms(timePath, f)...)
	}
	return c
}

// Clause compiles the path's document-level Clause: the disjunction of its valued selection arms
// (each wrapped in its collection's nested query with the prop term) and its specials, missing
// included.
func (t *topPath) Clause() types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	for _, arm := range t.RelArms() {
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(relPath, t.Prop), arm)).Path(relPath))
	}
	for _, f := range t.Amounts {
		for _, arm := range amountArms(amountPath, f) {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(amountPath, t.Prop), arm)).Path(amountPath))
		}
	}
	for _, f := range t.Times {
		for _, arm := range timeArms(timePath, f) {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(timePath, t.Prop), arm)).Path(timePath))
		}
	}
	if t.Specials != nil && t.Specials.Missing {
		arms = append(arms, topMissingQuery(t.Prop))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// pathMember is one member of a correlation group: the combined selection (valued filters plus
// specials) of one sub path under the group's parent property, or the group's pooled sub-has
// selection (prop is zero then, and the has filters carry the properties).
type pathMember struct {
	Prop     identifier.Identifier
	Refs     []*RefFilter
	Amounts  []*AmountFilter
	Times    []*TimeFilter
	Has      []*HasFilter
	Specials *SpecialsFilter
}

// hasPropsTerms returns a terms query matching rel records at path whose prop is one of the has
// filter's selected properties.
func hasPropsTerms(path string, props []HasValue) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, 0, len(props))
	for _, p := range props {
		values = append(values, esdsl.NewFieldValue().String(p.ID.String()))
	}
	return esdsl.NewTermsQuery().AddTermsQuery(path+".prop", esdsl.NewTermsQueryField().FieldValues(values...))
}

// PositiveArms compiles the member's positive per-claim arms against the sub collections of the
// given parent collection: each arm holds on a sub record nested under the same parent claim.
// Missing is excluded; the caller composes it per the group's quantification rule.
func (m *pathMember) PositiveArms(parent string) []types.QueryVariant {
	var arms []types.QueryVariant
	subRel := subPath(parent, relCollection)
	subAmount := subPath(parent, "amount")
	subTime := subPath(parent, "time")
	for _, f := range m.Refs {
		for _, to := range f.To {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				propTerm(subRel, m.Prop),
				term(subRel+".to", to.ID.String()),
			)).Path(subRel))
		}
		for _, to := range f.Direct {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				propTerm(subRel, m.Prop),
				term(subRel+".to", to.ID.String()),
				esdsl.NewTermQuery(subRel+".isLeaf", esdsl.NewFieldValue().Bool(true)),
			)).Path(subRel))
		}
	}
	for _, f := range m.Amounts {
		for _, arm := range amountArms(subAmount, f) {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(subAmount, m.Prop), arm)).Path(subAmount))
		}
	}
	for _, f := range m.Times {
		for _, arm := range timeArms(subTime, f) {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(propTerm(subTime, m.Prop), arm)).Path(subTime))
		}
	}
	for _, f := range m.Has {
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			claimTypeTerm(subRel, internalSearch.ClaimTypeHas),
			hasPropsTerms(subRel, f.Props),
		)).Path(subRel))
	}
	if m.Specials != nil {
		if m.Specials.None {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				claimTypeTerm(subRel, internalSearch.ClaimTypeNone), propTerm(subRel, m.Prop),
			)).Path(subRel))
		}
		if m.Specials.Unknown {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				claimTypeTerm(subRel, internalSearch.ClaimTypeUnknown), propTerm(subRel, m.Prop),
			)).Path(subRel))
		}
		if m.Specials.HasProperty {
			arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				claimTypeTerm(subRel, internalSearch.ClaimTypeHas), propTerm(subRel, m.Prop),
			)).Path(subRel))
		}
	}
	return arms
}

// MissingSelected reports whether the member's specials select missing.
func (m *pathMember) MissingSelected() bool {
	return m.Specials != nil && m.Specials.Missing
}

// MemberClause compiles the member for one parent collection inside a multi-member group: the
// positive arms OR'd with, when missing is selected, a same-claim missing arm (the shared parent
// claim carries no facetable sub-claim for the member's property). Missing quantifies over the
// claims the group binds, so inside a multi-member group it is per-claim; the standalone
// (single-member) all-claims form is composed by groupQuery instead.
func (m *pathMember) MemberClause(parent string) types.QueryVariant { //nolint:ireturn
	arms := m.PositiveArms(parent)
	if m.MissingSelected() {
		arms = append(arms, esdsl.NewBoolQuery().MustNot(facetableSubPresence(parent, m.Prop)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// group is one correlation group: the sub filters and sub specials sharing a parent property
// within one filter set. All members match on the same parent claim.
type group struct {
	ParentProp identifier.Identifier
	Members    []*pathMember
}

// ParentCollectionArm compiles the group's nested query for one parent collection, or ok false
// when the collection does not participate (excluded by the constraints, or a single-member group
// whose member has no positive arms for it).
func (g *group) ParentCollectionArm(parent string, constraints parentConstraints) (types.QueryVariant, bool) { //nolint:ireturn
	constraintArms, ok := constraints.ForCollection(parent)
	if !ok {
		return nil, false
	}
	musts := []types.QueryVariant{propTerm(parentPath(parent), g.ParentProp)}
	if len(constraintArms) > 0 {
		musts = append(musts, oneOrShould(constraintArms))
	}
	single := len(g.Members) == 1
	for _, m := range g.Members {
		if single {
			// The standalone missing arm is composed at the document level by groupQuery; only the
			// positive arms go inside the parent claim here.
			arms := m.PositiveArms(parent)
			if len(arms) == 0 {
				return nil, false
			}
			musts = append(musts, oneOrShould(arms))
			continue
		}
		musts = append(musts, m.MemberClause(parent))
	}
	return esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(musts...)).Path(parentPath(parent)), true
}

// parentExists matches documents with a parent claim for the group's parent property satisfying
// the constraints, across the participating parent collections. It is the sub paths' universe
// condition.
//
// Unconstrained it spans all parent collections, including the text-only ones: any claim can host
// sub-claims, so a text claim is an attachment point. This is deliberately broader than top-level
// missing (topMissingQuery), which consults only the facetable collections, so a document whose
// property is stated only as a text claim is top-level missing yet inside that property's sub
// paths' universe.
func parentExists(parentProp identifier.Identifier, constraints parentConstraints) types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		constraintArms, ok := constraints.ForCollection(parent)
		if !ok {
			continue
		}
		musts := []types.QueryVariant{propTerm(parentPath(parent), parentProp)}
		if len(constraintArms) > 0 {
			musts = append(musts, oneOrShould(constraintArms))
		}
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(musts...)).Path(parentPath(parent)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// subMissingQuery compiles the standalone missing selection of sub path (parentProp, prop): a
// parent claim satisfying the constraints exists, and no such parent claim carries a facetable
// sub-claim for prop. A document with no matching parent claim is outside the path's universe and
// does not match.
func subMissingQuery(parentProp, prop identifier.Identifier, constraints parentConstraints) types.QueryVariant { //nolint:ireturn
	var withSub []types.QueryVariant
	for _, parent := range parentCollections {
		constraintArms, ok := constraints.ForCollection(parent)
		if !ok {
			continue
		}
		musts := []types.QueryVariant{propTerm(parentPath(parent), parentProp)}
		if len(constraintArms) > 0 {
			musts = append(musts, oneOrShould(constraintArms))
		}
		musts = append(musts, facetableSubPresence(parent, prop))
		withSub = append(withSub, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(musts...)).Path(parentPath(parent)))
	}
	q := esdsl.NewBoolQuery().Must(parentExists(parentProp, constraints))
	if len(withSub) > 0 {
		q = q.MustNot(withSub...)
	}
	return q
}

// Query compiles the group's document-level clause: the disjunction over the participating parent
// collections of one nested Query each, correlating every member on the same parent claim, plus,
// for a single-member group with missing selected, the standalone all-claims missing arm OR'd at
// the document level (so the member's values stay correlated while its missing arm quantifies over
// all the parent property's claims).
//
// Which form a missing selection takes is therefore observable when filters change: removing a
// group's last sibling shifts the remaining missing selection from the same-claim form (some
// parent claim satisfying the siblings lacks the sub-property) to the all-claims form (no parent
// claim carries it at all).
func (g *group) Query(constraints parentConstraints) types.QueryVariant { //nolint:ireturn
	var standaloneMissing types.QueryVariant
	if len(g.Members) == 1 && g.Members[0].MissingSelected() {
		standaloneMissing = subMissingQuery(g.ParentProp, g.Members[0].Prop, constraints)
	}
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		arm, ok := g.ParentCollectionArm(parent, constraints)
		if !ok {
			continue
		}
		arms = append(arms, arm)
	}
	var positive types.QueryVariant
	if len(arms) > 0 {
		positive = oneOrShould(arms)
	}
	switch {
	case positive != nil && standaloneMissing != nil:
		return oneOrShould([]types.QueryVariant{positive, standaloneMissing})
	case positive != nil:
		return positive
	case standaloneMissing != nil:
		return standaloneMissing
	default:
		return matchNone()
	}
}

// compiledSet is the bucketed view of one filter set: its top-level paths, its pooled top-level
// has filters, and its correlation groups, all in first-appearance order so compiled queries are
// deterministic.
type compiledSet struct {
	Tops       []*topPath
	TopIndex   map[identifier.Identifier]*topPath
	TopHas     []*HasFilter
	Groups     []*group
	GroupIndex map[identifier.Identifier]*group
}

// MemberFor returns the group's member for the given sub property, creating it on first use. The
// pooled sub-has member is keyed by the zero identifier.
func (g *group) MemberFor(prop identifier.Identifier) *pathMember {
	for _, m := range g.Members {
		if m.Prop == prop {
			return m
		}
	}
	m := &pathMember{Prop: prop, Refs: nil, Amounts: nil, Times: nil, Has: nil, Specials: nil}
	g.Members = append(g.Members, m)
	return m
}

// bucketFilters buckets one filter set into its compiled view, skipping filters whose ID is in
// excludeIDs.
func bucketFilters(filters []Filter, excludeIDs []identifier.Identifier) *compiledSet {
	set := &compiledSet{
		Tops:       nil,
		TopIndex:   map[identifier.Identifier]*topPath{},
		TopHas:     nil,
		Groups:     nil,
		GroupIndex: map[identifier.Identifier]*group{},
	}
	topFor := func(prop identifier.Identifier) *topPath {
		t, ok := set.TopIndex[prop]
		if !ok {
			t = &topPath{Prop: prop, Refs: nil, Amounts: nil, Times: nil, Specials: nil}
			set.TopIndex[prop] = t
			set.Tops = append(set.Tops, t)
		}
		return t
	}
	groupFor := func(parentProp identifier.Identifier) *group {
		g, ok := set.GroupIndex[parentProp]
		if !ok {
			g = &group{ParentProp: parentProp, Members: nil}
			set.GroupIndex[parentProp] = g
			set.Groups = append(set.Groups, g)
		}
		return g
	}
	for i := range filters {
		f := &filters[i]
		if f.ID != nil && idInList(*f.ID, excludeIDs) {
			continue
		}
		switch {
		case f.Has != nil && len(f.Prop) == 0:
			set.TopHas = append(set.TopHas, f.Has)
		case f.Has != nil:
			m := groupFor(f.Prop[0]).MemberFor(identifier.Identifier{})
			m.Has = append(m.Has, f.Has)
		case len(f.Prop) == 1:
			t := topFor(f.Prop[0])
			switch {
			case f.Ref != nil:
				t.Refs = append(t.Refs, f.Ref)
			case f.Amount != nil:
				t.Amounts = append(t.Amounts, f.Amount)
			case f.Time != nil:
				t.Times = append(t.Times, f.Time)
			case f.Specials != nil:
				t.Specials = f.Specials
			}
		default:
			m := groupFor(f.Prop[0]).MemberFor(f.Prop[1])
			switch {
			case f.Ref != nil:
				m.Refs = append(m.Refs, f.Ref)
			case f.Amount != nil:
				m.Amounts = append(m.Amounts, f.Amount)
			case f.Time != nil:
				m.Times = append(m.Times, f.Time)
			case f.Specials != nil:
				m.Specials = f.Specials
			}
		}
	}
	return set
}

// compileFilters compiles one filter set into document-level clauses: one clause per top-level
// path (its valued selections and specials OR'd), one per pooled top-level has filter, and one per
// correlation group. Groups take their parent constraints from the same set's top-level selections
// on the parent property; nothing crosses filter sets.
func compileFilters(filters []Filter, excludeIDs []identifier.Identifier) []types.QueryVariant {
	set := bucketFilters(filters, excludeIDs)
	clauses := make([]types.QueryVariant, 0, len(set.Tops)+len(set.TopHas)+len(set.Groups))
	for _, t := range set.Tops {
		clauses = append(clauses, t.Clause())
	}
	for _, f := range set.TopHas {
		clauses = append(clauses, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			claimTypeTerm(relPath, internalSearch.ClaimTypeHas),
			hasPropsTerms(relPath, f.Props),
		)).Path(relPath))
	}
	for _, g := range set.Groups {
		constraints := parentConstraints{Rel: nil, Amount: nil, Time: nil}
		if t, ok := set.TopIndex[g.ParentProp]; ok {
			constraints = t.Constraints()
		}
		clauses = append(clauses, g.Query(constraints))
	}
	return clauses
}

// ParentContext carries the parent-claim scoping for a sub path's facet aggregations: the group's
// parent constraints (from the session's top-level selections on the parent property) and the
// sibling members' conditions. Facet aggregations apply it at parent level so their counts match
// what selecting a value in the facet would return under the correlation semantics.
type ParentContext struct {
	parentProp  identifier.Identifier
	constraints parentConstraints
	siblings    []*pathMember
}

// ParentContextFor builds the ParentContext for facets of sub path (parentProp, prop): the
// session Filters group for parentProp without the focus path's own members and without the
// filters in excludeIDs. Prefilters contribute nothing, matching that filter sets do not
// correlate.
func (s *SessionData) ParentContextFor(parentProp, prop identifier.Identifier, excludeIDs ...identifier.Identifier) *ParentContext {
	set := bucketFilters(s.Filters, excludeIDs)
	constraints := parentConstraints{Rel: nil, Amount: nil, Time: nil}
	if t, ok := set.TopIndex[parentProp]; ok {
		constraints = t.Constraints()
	}
	var siblings []*pathMember
	if g, ok := set.GroupIndex[parentProp]; ok {
		for _, m := range g.Members {
			if m.Prop == prop {
				continue
			}
			siblings = append(siblings, m)
		}
	}
	return &ParentContext{parentProp: parentProp, constraints: constraints, siblings: siblings}
}

// Collections returns the parent collections participating under the context's constraints.
func (c *ParentContext) Collections() []string {
	var out []string
	for _, parent := range parentCollections {
		if _, ok := c.constraints.ForCollection(parent); ok {
			out = append(out, parent)
		}
	}
	return out
}

// CollectionFilter returns the parent-level filter for the given parent collection: the parent
// property term, the constraint arms, and the sibling members' clauses (with sibling missing in
// its same-claim form, since the focus path makes the group multi-member). ok is false when the
// constraints exclude the collection.
func (c *ParentContext) CollectionFilter(parent string) (types.QueryVariant, bool) { //nolint:ireturn
	constraintArms, ok := c.constraints.ForCollection(parent)
	if !ok {
		return nil, false
	}
	musts := []types.QueryVariant{propTerm(parentPath(parent), c.parentProp)}
	if len(constraintArms) > 0 {
		musts = append(musts, oneOrShould(constraintArms))
	}
	for _, m := range c.siblings {
		musts = append(musts, m.MemberClause(parent))
	}
	return esdsl.NewBoolQuery().Must(musts...), true
}

// ExistsQuery matches documents with a parent claim satisfying the context (constraints and
// sibling conditions): the sub path's universe under the current session.
func (c *ParentContext) ExistsQuery() types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		filter, ok := c.CollectionFilter(parent)
		if !ok {
			continue
		}
		arms = append(arms, esdsl.NewNestedQuery(filter).Path(parentPath(parent)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// MissingQuery matches documents in the sub path's universe whose qualifying parent claims all
// lack a facetable sub-claim for prop: the facet's missing bucket, compiled from the same context
// its value aggregations use so the count matches selecting missing.
func (c *ParentContext) MissingQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	var withSub []types.QueryVariant
	for _, parent := range parentCollections {
		filter, ok := c.CollectionFilter(parent)
		if !ok {
			continue
		}
		withSub = append(withSub, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(filter, facetableSubPresence(parent, prop))).Path(parentPath(parent)))
	}
	q := esdsl.NewBoolQuery().Must(c.ExistsQuery())
	if len(withSub) > 0 {
		q = q.MustNot(withSub...)
	}
	return q
}

// SpecialQuery matches documents with a sub rel record of the given claimType for prop under a
// qualifying parent claim: the facet's none/unknown/hasProperty special buckets.
func (c *ParentContext) SpecialQuery(prop identifier.Identifier, claimType string) types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		filter, ok := c.CollectionFilter(parent)
		if !ok {
			continue
		}
		subRel := subPath(parent, relCollection)
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
			filter,
			esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
				claimTypeTerm(subRel, claimType), propTerm(subRel, prop),
			)).Path(subRel),
		)).Path(parentPath(parent)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// OtherTypesQuery matches documents whose qualifying parent claims carry a facetable sub-claim for
// prop in a sub collection other than the given one ("rel", "amount", or "time"): the sub facet's
// other-value-types count, which closes the per-facet identity for a sub property stated in several
// value types.
func (c *ParentContext) OtherTypesQuery(prop identifier.Identifier, collection string) types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	for _, parent := range parentCollections {
		filter, ok := c.CollectionFilter(parent)
		if !ok {
			continue
		}
		var presence []types.QueryVariant
		if collection != "rel" {
			presence = append(presence, esdsl.NewNestedQuery(propTerm(subPath(parent, relCollection), prop)).Path(subPath(parent, relCollection)))
		}
		if collection != "amount" {
			presence = append(presence, esdsl.NewNestedQuery(propTerm(subPath(parent, "amount"), prop)).Path(subPath(parent, "amount")))
		}
		if collection != "time" {
			presence = append(presence, esdsl.NewNestedQuery(propTerm(subPath(parent, "time"), prop)).Path(subPath(parent, "time")))
		}
		arms = append(arms, esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(filter, oneOrShould(presence))).Path(parentPath(parent)))
	}
	if len(arms) == 0 {
		return matchNone()
	}
	return oneOrShould(arms)
}

// TopSpecialQuery matches documents with a top-level rel record of the given claimType for prop:
// the top-level facet's none/unknown/hasProperty special buckets.
func TopSpecialQuery(prop identifier.Identifier, claimType string) types.QueryVariant { //nolint:ireturn
	return esdsl.NewNestedQuery(esdsl.NewBoolQuery().Must(
		claimTypeTerm(relPath, claimType), propTerm(relPath, prop),
	)).Path(relPath)
}

// TopMissingQuery matches documents that state nothing facetable for prop: the top-level facet's
// missing bucket.
func TopMissingQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	return topMissingQuery(prop)
}

// TopOtherTypesQuery matches documents with a facetable claim for prop in a collection other than
// the given one ("rel", "amount", or "time"): the facet's other-value-types count, which closes
// the per-facet identity for a property stated in several value types.
func TopOtherTypesQuery(prop identifier.Identifier, collection string) types.QueryVariant { //nolint:ireturn
	var arms []types.QueryVariant
	if collection != "rel" {
		arms = append(arms, esdsl.NewNestedQuery(propTerm(relPath, prop)).Path(relPath))
	}
	if collection != "amount" {
		arms = append(arms, esdsl.NewNestedQuery(propTerm(amountPath, prop)).Path(amountPath))
	}
	if collection != "time" {
		arms = append(arms, esdsl.NewNestedQuery(propTerm(timePath, prop)).Path(timePath))
	}
	return oneOrShould(arms)
}

// reverseScopeQuery returns a query matching documents that have a ref record (top-level or nested
// under any parent collection) whose "to" target equals the given ID, regardless of property.
func reverseScopeQuery(id identifier.Identifier) types.QueryVariant { //nolint:ireturn
	shoulds := make([]types.QueryVariant, 0, 1+len(parentCollections))
	shoulds = append(shoulds, esdsl.NewNestedQuery(term(relPath+".to", id.String())).Path(relPath))
	for _, parent := range parentCollections {
		subRel := subPath(parent, relCollection)
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewNestedQuery(term(subRel+".to", id.String())).Path(subRel),
		).Path(parentPath(parent)))
	}
	return oneOrShould(shoulds)
}

// idsScopeQuery returns a query matching documents whose own ID is one of ids. It queries
// the indexed id field rather than the internal _id field because a document may have
// multiple IDs, and the id field can then hold all of them.
func idsScopeQuery(ids []identifier.Identifier) types.QueryVariant { //nolint:ireturn
	values := make([]types.FieldValueVariant, 0, len(ids))
	for _, id := range ids {
		values = append(values, esdsl.NewFieldValue().String(id.String()))
	}
	return esdsl.NewTermsQuery().AddTermsQuery("id", esdsl.NewTermsQueryField().FieldValues(values...))
}

// withFilters returns musts as a bool query, adding any non-nil filters as filter clauses.
//
// When there are no scoring (must) clauses the documents are selected purely by membership, and we
// want their score to be 0. An empty bool query would instead match all documents with score 1, so
// when neither musts nor filters are present we add a match_all filter clause. Filter clauses do not
// contribute to the score, so such results stay at score 0 (the counts.score function_score, being a
// multiply, then leaves them at 0). Only a query or filters (which go into musts) produce non-zero scores.
func withFilters(musts, filters []types.QueryVariant) types.QueryVariant { //nolint:ireturn
	query := esdsl.NewBoolQuery().Must(musts...)
	fs := make([]types.QueryVariant, 0, len(filters))
	for _, f := range filters {
		if f != nil {
			fs = append(fs, f)
		}
	}
	if len(musts) == 0 && len(fs) == 0 {
		fs = append(fs, esdsl.NewMatchAllQuery())
	}
	if len(fs) > 0 {
		return query.Filter(fs...)
	}
	return query
}

// toQuery converts the session to an ElasticSearch query, skipping filters whose ID is in
// excludeIDs. Filters compile into the scoring must clause and prefilters into the non-scoring
// filter clause, each set through its own correlation groups; the two sets never correlate.
func (s *SessionData) toQuery(excludeIDs []identifier.Identifier, enabledLanguages []string, extraFilters []types.QueryVariant) types.QueryVariant { //nolint:ireturn
	var musts []types.QueryVariant

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.And, enabledLanguages))
	}

	musts = append(musts, compileFilters(s.Filters, excludeIDs)...)

	filters := make([]types.QueryVariant, 0, len(extraFilters)+2) //nolint:mnd
	filters = append(filters, extraFilters...)

	// Prefilters constrain the result set like filters but go into the filter clause, so
	// they do not contribute to _score.
	filters = append(filters, compileFilters(s.Prefilters, excludeIDs)...)

	// Reverse scopes results to documents that reference the target (directly or via a
	// sub-reference). It is a pure membership constraint, so it goes in the filter clause
	// and does not contribute to _score.
	if s.Reverse != nil {
		filters = append(filters, reverseScopeQuery(*s.Reverse))
	}

	// IDs scope results to an explicit document set. It is a pure membership constraint,
	// so it goes in the filter clause and does not contribute to _score.
	if len(s.IDs) > 0 {
		filters = append(filters, idsScopeQuery(s.IDs))
	}

	return withFilters(musts, filters)
}

// ToQuery converts the session to an ElasticSearch query.
//
// enabledLanguages is the site's indexed language set, used to scope the text-search query
// to the languages the index actually has (empty falls back to the global default).
// extraFilters are added as bool filter clauses (used for the per-caller access restriction).
func (s *SessionData) ToQuery(enabledLanguages []string, extraFilters ...types.QueryVariant) types.QueryVariant { //nolint:ireturn
	return s.toQuery(nil, enabledLanguages, extraFilters)
}

// ToQueryExcluding converts the session to an ElasticSearch query, excluding the filters with the
// given IDs. Fetching a facet's data excludes the facet's own valued filter and the path's specials
// filter, so the facet's own selections do not narrow its available values; sibling selections
// (including the path's group siblings) still apply.
func (s *SessionData) ToQueryExcluding( //nolint:ireturn
	excludeFilterIDs []identifier.Identifier, enabledLanguages []string, extraFilters ...types.QueryVariant,
) types.QueryVariant {
	return s.toQuery(excludeFilterIDs, enabledLanguages, extraFilters)
}

// FacetExcludeIDs returns the filter IDs to exclude when rendering a facet of the given property
// path: the facet's own valued filter (identified by excludeFilterID, when the facet is active) and
// the path's specials filter. The specials are excluded path-wide so a special selected in one of
// the path's facets does not hide the values of another.
func (s *SessionData) FacetExcludeIDs(prop []identifier.Identifier, excludeFilterID *identifier.Identifier) []identifier.Identifier {
	var out []identifier.Identifier
	if excludeFilterID != nil {
		out = append(out, *excludeFilterID)
	}
	for i := range s.Filters {
		f := &s.Filters[i]
		if f.Specials != nil && SamePropPath(f.Prop, prop) && f.ID != nil && !idInList(*f.ID, out) {
			out = append(out, *f.ID)
		}
	}
	return out
}
