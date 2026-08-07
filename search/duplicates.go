package search

import (
	"context"
	"maps"
	"slices"
	"strconv"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// Duplicate detection compares a document against the corpus by structure rather than by free text.
// Each of the document's stated claims becomes one scoring clause that matches existing documents
// sharing that field: a reference to the same target, an identifier with the same value, a string with
// the same or a near-matching name (fuzzy, to catch typos and reordered words), the same rich-text
// (HTML) body, an amount/time whose value window overlaps, and so on. A candidate's score is the sum of
// the weights of the clauses it matches, so a document that agrees on more (and on more identifying)
// fields ranks higher. We then keep the highest-scoring candidates above a small threshold.
//
// We deliberately do not use ElasticSearch More-Like-This or other term-similarity queries: they
// score on shared analyzed tokens across a flat text field and ignore the claim structure (which
// property a value belongs to, whether two documents reference the same entity, whether two numbers
// are the same measurement). They are also IDF-driven, which both misranks for this purpose and is a
// term-statistics side channel under per-document access control (see the note in ResultsGet). Every
// clause here is wrapped in constant_score, so a match contributes a fixed weight independent of
// corpus term statistics: the score is structural and stable across corpora, and leaks nothing.
//
// INSTANCE_OF is not special-cased and is not a hard filter. It is simply one of the reference
// fields, so sharing it contributes its weight and pushes same-typed documents up the ranking, while
// a strong non-type signal (a shared external identifier, the same name) can still surface a
// duplicate whose type differs or is not yet set. The threshold keeps a bare shared type (or a single
// weak field) from flooding the results with same-typed documents.
//
// DISTINCT_FROM is the exception: a document the input is asserted to be distinct from is known not to
// be a duplicate, so it does not contribute a scoring clause and is excluded from the results outright.
// The relation is symmetric, so documents that assert they are distinct from the input are excluded too.
// It is not transitive (A distinct from B and B distinct from C does not make A distinct from C, since A
// and C may be the same), so only directly asserted pairs are excluded, never chains.
const (
	// identifierDuplicateWeight is the score a shared identifier value contributes. External
	// identifiers (Wikidata IDs, ISBNs, ...) are near-unique, so a single shared one is on its own a
	// strong duplicate signal and clears minDuplicateScore alone.
	identifierDuplicateWeight = float32(10)
	// linkDuplicateWeight is the score a shared link IRI contributes. A shared canonical URL (an
	// official website, a source page) is almost as identifying as an external identifier.
	linkDuplicateWeight = float32(6)
	// stringDuplicateWeight is the score a shared string value contributes. A matching name or title
	// is a strong signal, enough to surface a candidate on its own.
	stringDuplicateWeight = float32(5)
	// referenceDuplicateWeight is the score a shared reference contributes (INSTANCE_OF and every
	// other relation alike). A single shared relation is weak on its own (many documents share a
	// type or a publisher), so it needs corroboration to clear minDuplicateScore.
	referenceDuplicateWeight = float32(2)
	// amountDuplicateWeight is the score a shared amount contributes (its value window overlaps).
	amountDuplicateWeight = float32(2)
	// timeDuplicateWeight is the score a shared time contributes (its value window overlaps).
	timeDuplicateWeight = float32(2)
	// hasDuplicateWeight is the score a shared "has" property contributes. Mere presence of the same
	// property is the weakest signal, so it only nudges ranking among already-matching candidates.
	hasDuplicateWeight = float32(1)
	// htmlDuplicateWeight is the score a shared HTML body contributes. Matching long rich-text content
	// is weak and noisy (boilerplate descriptions repeat), so it only nudges ranking among already-
	// matching candidates and never surfaces a candidate on its own.
	htmlDuplicateWeight = float32(1)

	// minDuplicateScore is the smallest total score a candidate must reach to be reported. It is
	// tuned against the weights above so that one identifying field (identifier, link, name), or at
	// least two corroborating weaker fields, is required: a single shared reference (including a bare
	// shared INSTANCE_OF) or a single shared "has" property does not qualify.
	minDuplicateScore = float64(4)
)

// duplicateClauses builds one scoring should-clause per distinct stated claim of doc that can be
// matched structurally against the index, together with the targets doc is asserted to be distinct
// from. Each clause is a nested query matching documents that have the same property and value,
// wrapped in constant_score so that matching it contributes exactly the claim type's weight to the
// document's score, regardless of corpus term statistics.
//
// Only top-level claims are considered (sub-claims are not walked), and only those at or above
// LowConfidence, mirroring what the indexer keeps. Identical clauses (same type, property and value)
// are emitted once, so repeating a value does not double-count. enabledLanguages scopes the
// per-language string fields the string clauses query; it falls back to all supported languages.
func duplicateClauses(doc *document.D, enabledLanguages []string) ([]types.QueryVariant, []identifier.Identifier) {
	if doc == nil || doc.Claims == nil {
		return nil, nil
	}
	v := &duplicateVisitor{
		EnabledLanguages: enabledLanguages,
		Clauses:          nil,
		DistinctFrom:     nil,
		Seen:             map[string]bool{},
	}
	// The visit methods never fail and never drop a claim, so the walk cannot return an error and
	// leaves the document unchanged. The document's own claims are visited, not its sub-claims: the
	// visit methods do not recurse.
	_ = doc.Claims.Visit(v)
	return v.Clauses, v.DistinctFrom
}

// duplicateVisitor turns a document's stated claims into the duplicate-detection scoring clauses.
// Implementing document.Visitor makes every claim type a decision taken here: a type which
// contributes no clause says so in its own method, instead of being left out of a walk over some of
// the types, where a type added later would be silently ignored.
type duplicateVisitor struct {
	// EnabledLanguages scopes the per-language fields the string and HTML clauses query.
	EnabledLanguages []string
	// Clauses are the scoring clauses collected so far.
	Clauses []types.QueryVariant
	// DistinctFrom are the targets of DISTINCT_FROM claims, which are excluded from the results (in
	// duplicatesQuery) instead of contributing a clause.
	DistinctFrom []identifier.Identifier
	// Seen keys the claims already turned into a clause, by claim type, property, and value.
	Seen map[string]bool
}

var _ document.Visitor = (*duplicateVisitor)(nil)

// Add collects the clause for the claim identified by key, weighted by its claim type. A key already
// collected contributes nothing, so repeating a value does not double-count, and so does a nil query,
// which is how a claim whose clause cannot be built is skipped.
func (v *duplicateVisitor) Add(key string, weight float32, query types.QueryVariant) (document.VisitResult, errors.E) {
	if query != nil && !v.Seen[key] {
		v.Seen[key] = true
		v.Clauses = append(v.Clauses, esdsl.NewConstantScoreQuery(query).Boost(weight))
	}
	return document.Keep, nil
}

// Skip reports whether the claim contributes nothing because it is below the confidence at which the
// indexer keeps a claim, so nothing in the index could match it anyway.
func (v *duplicateVisitor) Skip(claim document.Claim) bool {
	return claim.GetConfidence() < document.LowConfidence
}

func (v *duplicateVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) || claim.Value == "" {
		return document.Keep, nil
	}
	return v.Add(
		"id\x00"+claim.Prop.ID.String()+"\x00"+claim.Value, identifierDuplicateWeight,
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.id.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewMatchPhraseQuery("claims.id.value", claim.Value),
			),
		).Path("claims.id"),
	)
}

func (v *duplicateVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) || claim.String == "" {
		return document.Keep, nil
	}
	return v.Add(
		"string\x00"+claim.Prop.ID.String()+"\x00"+claim.String, stringDuplicateWeight,
		stringDuplicateNested(claim.Prop.ID, claim.String, v.EnabledLanguages),
	)
}

func (v *duplicateVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) || claim.HTML == "" {
		return document.Keep, nil
	}
	return v.Add(
		"html\x00"+claim.Prop.ID.String()+"\x00"+claim.HTML, htmlDuplicateWeight,
		htmlDuplicateNested(claim.Prop.ID, claim.HTML, v.EnabledLanguages),
	)
}

func (v *duplicateVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	return v.Add(
		"amount\x00"+claim.Prop.ID.String()+"\x00"+claim.Amount.String()+"\x00"+strconv.FormatFloat(claim.Precision, 'g', -1, 64),
		amountDuplicateWeight, amountDuplicateNested(claim),
	)
}

func (v *duplicateVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	from, to, ok := amountIntervalWindow(claim)
	if !ok {
		return document.Keep, nil
	}
	return v.Add(
		intervalKey("amountInterval", claim.Prop.ID, from, to), amountDuplicateWeight,
		rangeDuplicateNested(amountPath, claim.Prop.ID, from, to),
	)
}

func (v *duplicateVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	return v.Add(
		"time\x00"+claim.Prop.ID.String()+"\x00"+claim.Time.String()+"\x00"+strconv.Itoa(int(claim.Precision)),
		timeDuplicateWeight, timeDuplicateNested(claim),
	)
}

func (v *duplicateVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	from, to, ok := timeIntervalWindow(claim)
	if !ok {
		return document.Keep, nil
	}
	return v.Add(
		intervalKey("timeInterval", claim.Prop.ID, from, to), timeDuplicateWeight,
		rangeDuplicateNested(timePath, claim.Prop.ID, from, to),
	)
}

func (v *duplicateVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) || claim.IRI == "" {
		return document.Keep, nil
	}
	return v.Add(
		"link\x00"+claim.Prop.ID.String()+"\x00"+claim.IRI, linkDuplicateWeight,
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.link.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewMatchPhraseQuery("claims.link.iri", claim.IRI),
			),
		).Path("claims.link"),
	)
}

func (v *duplicateVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	if claim.Prop.ID == internalCore.DistinctFromPropID {
		// The target is asserted to be a different entity, so it is excluded from the results (in
		// duplicatesQuery) rather than scored as a similarity.
		v.DistinctFrom = append(v.DistinctFrom, claim.To.ID)
		return document.Keep, nil
	}
	// The index expands a reference to the target and all its hierarchy ancestors, so matching the
	// stated (most-specific) target also matches documents that reference a narrower value of it.
	// A term on "to" matches only rel records with the ref claimType, so no claimType term is needed.
	return v.Add(
		"ref\x00"+claim.Prop.ID.String()+"\x00"+claim.To.ID.String(), referenceDuplicateWeight,
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery(relPath+".prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewTermQuery(relPath+".to", esdsl.NewFieldValue().String(claim.To.ID.String())),
			),
		).Path(relPath),
	)
}

func (v *duplicateVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	if v.Skip(claim) {
		return document.Keep, nil
	}
	return v.Add(
		"has\x00"+claim.Prop.ID.String(), hasDuplicateWeight,
		esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				claimTypeTerm(relPath, internalSearch.ClaimTypeHas),
				esdsl.NewTermQuery(relPath+".prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
			),
		).Path(relPath),
	)
}

// A none claim asserts that the document has no value for the property. Two documents agreeing that
// something is absent does not make them the same entity, so it contributes nothing.
func (v *duplicateVisitor) VisitNone(*document.NoneClaim) (document.VisitResult, errors.E) {
	return document.Keep, nil
}

// An unknown claim asserts that a value exists but is not known, which is not identifying either.
func (v *duplicateVisitor) VisitUnknown(*document.UnknownClaim) (document.VisitResult, errors.E) {
	return document.Keep, nil
}

// stringDuplicateNested matches documents that have a string claim for prop whose value matches value
// in any enabled language. The value is matched against each per-language string field with edit-distance
// fuzziness (AUTO) so typos and minor spelling/transliteration differences still match, and operator AND
// so every token must be present (in any order), which keeps it from matching on a single shared word.
// Querying every enabled language matches regardless of which language the candidate indexed the string
// under, with that language's analyzer applied on both sides.
func stringDuplicateNested(prop identifier.Identifier, value string, enabledLanguages []string) types.QueryVariant { //nolint:ireturn
	langs := enabledLanguages
	if len(langs) == 0 {
		langs = slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	}
	shoulds := make([]types.QueryVariant, 0, len(langs))
	for _, lang := range langs {
		shoulds = append(shoulds, esdsl.NewMatchQuery("claims.string.string."+lang, value).
			Fuzziness(esdsl.NewFuzziness().String("AUTO")).
			Operator(operator.And))
	}
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.string.prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)),
		),
	).Path("claims.string")
}

// htmlDuplicateNested matches documents whose HTML claim for prop has the same text content. The claim's
// HTML is stripped to plain text exactly as the indexer strips it (per language, via StripHTML), then
// matched against each per-language html field requiring every token (operator AND, any order), so it
// fires on a near-identical body. It is intentionally not fuzzy: HTML bodies are long and duplicates are
// copy-pasted, so token equality is the useful signal and fuzziness would be costly and noisy. Languages
// whose HTML cannot be parsed or strips to nothing are skipped; it returns nil when none remain.
func htmlDuplicateNested(prop identifier.Identifier, html string, enabledLanguages []string) types.QueryVariant { //nolint:ireturn
	langs := enabledLanguages
	if len(langs) == 0 {
		langs = slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	}
	var shoulds []types.QueryVariant
	for _, lang := range langs {
		stripped, errE := internalSearch.StripHTML(html, lang)
		if errE != nil || stripped == "" {
			continue
		}
		shoulds = append(shoulds, esdsl.NewMatchQuery("claims.html.html."+lang, stripped).Operator(operator.And))
	}
	if len(shoulds) == 0 {
		return nil
	}
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.html.prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1)),
		),
	).Path("claims.html")
}

// rangeDuplicateNested matches documents whose claim for prop in the amount or time collection at path
// has a value window overlapping [from, to]. Both sides are ranges, so the default range-on-range
// INTERSECTS relation finds the same or an indistinguishable value. Units are not constrained: a
// property carries a consistent measure, so the property already scopes the comparison.
func rangeDuplicateNested(path string, prop identifier.Identifier, from, to float64) types.QueryVariant { //nolint:ireturn
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery(path+".prop", esdsl.NewFieldValue().String(prop.String())),
			esdsl.NewNumberRangeQuery(path+".range").Gte(types.Float64(from)).Lte(types.Float64(to)),
		),
	).Path(path)
}

// intervalKey keys an interval's clause by the window it queries rather than by the bounds as they
// are stated, so two claims stating the same interval differently (decreasing order, an unknown bound
// beside a stated one) share one clause.
func intervalKey(claimType string, prop identifier.Identifier, from, to float64) string {
	return claimType + "\x00" + prop.String() +
		"\x00" + strconv.FormatFloat(from, 'g', -1, 64) +
		"\x00" + strconv.FormatFloat(to, 'g', -1, 64)
}

// amountDuplicateNested matches documents whose amount claim for the same property has a value window
// overlapping this claim's window. The window is computed exactly as the indexer computes the stored
// range. It returns nil (the claim is skipped) when the window cannot be computed.
func amountDuplicateNested(claim *document.AmountClaim) types.QueryVariant { //nolint:ireturn
	from, errE := claim.Amount.WindowStartFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	to, errE := claim.Amount.WindowEndFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	return rangeDuplicateNested(amountPath, claim.Prop.ID, from, to)
}

// timeDuplicateNested matches documents whose time claim for the same property has a value window
// overlapping this claim's window, computed exactly as the indexer computes the stored range. It
// returns nil (the claim is skipped) when the window cannot be computed.
func timeDuplicateNested(claim *document.TimeClaim) types.QueryVariant { //nolint:ireturn
	from, errE := claim.Time.WindowStartFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	to, errE := claim.Time.WindowEndFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	return rangeDuplicateNested(timePath, claim.Prop.ID, from, to)
}

// An interval claim occupies a value window just like a point claim does, so it is matched the same
// way, against the window the indexer computes for it (see convertAmountInterval and
// convertTimeInterval):
//
//   - Two stated bounds span from the lower bound's window to the upper one's, with a bound stated as
//     open excluding its own window. Bounds stated in decreasing order are swapped, like the indexer
//     swaps them.
//   - A bound stated beside one which is unknown collapses the interval to a point at the stated
//     bound, which is how the indexer stores it too.
//   - Anything else occupies no window worth matching: an interval with a bound stated as absent is
//     unbounded on that side, so it overlaps a large part of the corpus and says nothing about two
//     documents being the same, and an interval with neither bound stated is stored as an unknown
//     record, which is not a duplicate signal either (see VisitUnknown).
//
// The window functions report an error for a value a precision cannot represent; such a claim is
// skipped, like one whose window cannot be computed on the point path.

// amountIntervalWindow returns the value window an amount interval claim occupies, and whether it
// occupies one at all.
func amountIntervalWindow(claim *document.AmountIntervalClaim) (float64, float64, bool) {
	var from, to *document.Amount
	var fromPrecision, toPrecision *float64
	var fromIsOpen, toIsOpen bool
	switch {
	case claim.From != nil && claim.To != nil:
		from, fromPrecision, fromIsOpen = claim.From, claim.FromPrecision, claim.FromIsOpen
		to, toPrecision, toIsOpen = claim.To, claim.ToPrecision, claim.ToIsOpen
	case claim.From != nil && claim.ToIsUnknown:
		from, fromPrecision = claim.From, claim.FromPrecision
		to, toPrecision = claim.From, claim.FromPrecision
	case claim.To != nil && claim.FromIsUnknown:
		from, fromPrecision = claim.To, claim.ToPrecision
		to, toPrecision = claim.To, claim.ToPrecision
	default:
		return 0, 0, false
	}
	window := func(lo *document.Amount, loPrecision *float64, loIsOpen bool, hi *document.Amount, hiPrecision *float64, hiIsOpen bool) (float64, float64, bool) {
		if loPrecision == nil || hiPrecision == nil {
			return 0, 0, false
		}
		lower, errE := lo.WindowStartFloat64(*loPrecision, loIsOpen)
		if errE != nil {
			return 0, 0, false
		}
		upper, errE := hi.WindowEndFloat64(*hiPrecision, hiIsOpen)
		if errE != nil {
			return 0, 0, false
		}
		return lower, upper, true
	}
	lower, upper, ok := window(from, fromPrecision, fromIsOpen, to, toPrecision, toIsOpen)
	if !ok {
		return 0, 0, false
	}
	if lower <= upper {
		return lower, upper, true
	}
	// The bounds are stated in decreasing order, so they are swapped, like the indexer swaps them, and
	// the window is computed again: a bound stated as open excludes the window at the end it bounds,
	// which is the other end now.
	return window(to, toPrecision, toIsOpen, from, fromPrecision, fromIsOpen)
}

// timeIntervalWindow returns the value window a time interval claim occupies, and whether it occupies
// one at all.
func timeIntervalWindow(claim *document.TimeIntervalClaim) (float64, float64, bool) {
	var from, to *document.Time
	var fromPrecision, toPrecision *document.TimePrecision
	var fromIsOpen, toIsOpen bool
	switch {
	case claim.From != nil && claim.To != nil:
		from, fromPrecision, fromIsOpen = claim.From, claim.FromPrecision, claim.FromIsOpen
		to, toPrecision, toIsOpen = claim.To, claim.ToPrecision, claim.ToIsOpen
	case claim.From != nil && claim.ToIsUnknown:
		from, fromPrecision = claim.From, claim.FromPrecision
		to, toPrecision = claim.From, claim.FromPrecision
	case claim.To != nil && claim.FromIsUnknown:
		from, fromPrecision = claim.To, claim.ToPrecision
		to, toPrecision = claim.To, claim.ToPrecision
	default:
		return 0, 0, false
	}
	window := func(
		lo *document.Time, loPrecision *document.TimePrecision, loIsOpen bool,
		hi *document.Time, hiPrecision *document.TimePrecision, hiIsOpen bool,
	) (float64, float64, bool) {
		if loPrecision == nil || hiPrecision == nil {
			return 0, 0, false
		}
		lower, errE := lo.WindowStartFloat64(*loPrecision, loIsOpen)
		if errE != nil {
			return 0, 0, false
		}
		upper, errE := hi.WindowEndFloat64(*hiPrecision, hiIsOpen)
		if errE != nil {
			return 0, 0, false
		}
		return lower, upper, true
	}
	lower, upper, ok := window(from, fromPrecision, fromIsOpen, to, toPrecision, toIsOpen)
	if !ok {
		return 0, 0, false
	}
	if lower <= upper {
		return lower, upper, true
	}
	// The bounds are stated in decreasing order, so they are swapped, like the indexer swaps them, and
	// the window is computed again: a bound stated as open excludes the window at the end it bounds,
	// which is the other end now.
	return window(to, toPrecision, toIsOpen, from, fromPrecision, fromIsOpen)
}

// duplicatesQuery builds the ElasticSearch query that finds potential duplicates of doc: a bool whose
// should clauses are the document's structural field matches (see duplicateClauses), requiring at
// least one to match, excluding the document itself and any document it is distinct from (in either
// direction), and applying any extra filter clauses (for example the per-caller access restriction).
// It returns nil when doc has no matchable claims, in which case there is nothing to search for.
//
// The returned query does not itself enforce the minDuplicateScore threshold; DuplicatesGet applies
// it as the search min_score.
func duplicatesQuery(doc *document.D, enabledLanguages []string, exclude identifier.Identifier, extraFilters ...types.QueryVariant) types.QueryVariant { //nolint:ireturn
	shoulds, distinctFrom := duplicateClauses(doc, enabledLanguages)
	if len(shoulds) == 0 {
		return nil
	}

	query := esdsl.NewBoolQuery().
		Should(shoulds...).
		MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))

	// Exclude the document itself and every document it is asserted to be distinct from, so they are
	// never reported as duplicates.
	excluded := map[identifier.Identifier]bool{}
	var mustNot []types.QueryVariant
	excludeID := func(id identifier.Identifier) {
		if id == (identifier.Identifier{}) || excluded[id] {
			return
		}
		excluded[id] = true
		mustNot = append(mustNot, esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(id.String())))
	}
	excludeID(exclude)
	for _, id := range distinctFrom {
		excludeID(id)
	}
	if exclude != (identifier.Identifier{}) {
		// DISTINCT_FROM is symmetric, so also exclude candidates that assert they are distinct from this
		// document (the reverse of the forward exclusions above). It is not transitive, so we only follow
		// the direct assertion, not chains of it.
		mustNot = append(mustNot, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery(relPath+".prop", esdsl.NewFieldValue().String(internalCore.DistinctFromPropID.String())),
				esdsl.NewTermQuery(relPath+".to", esdsl.NewFieldValue().String(exclude.String())),
			),
		).Path(relPath))
	}
	if len(mustNot) > 0 {
		query = query.MustNot(mustNot...)
	}

	filters := make([]types.QueryVariant, 0, len(extraFilters))
	for _, f := range extraFilters {
		if f != nil {
			filters = append(filters, f)
		}
	}
	if len(filters) > 0 {
		query = query.Filter(filters...)
	}

	return query
}

// DuplicatesGet returns up to limit potential duplicates of doc, highest structural score first.
//
// It runs duplicatesQuery and keeps only hits scoring at least minDuplicateScore (the search
// min_score), so candidates matching just one weak field (or a bare shared type) are dropped. Scores
// are sums of constant per-field weights, so the ranking and the threshold are independent of corpus
// term statistics. extraFilters are added as bool filter clauses (the per-caller access restriction).
// When doc has no matchable claims it returns an empty list without querying ElasticSearch.
func DuplicatesGet(
	ctx context.Context, getSearchService func() *esSearch.Search, doc *document.D,
	exclude identifier.Identifier, enabledLanguages []string, limit int, extraFilters ...types.QueryVariant,
) ([]Result, errors.E) {
	query := duplicatesQuery(doc, enabledLanguages, exclude, extraFilters...)
	if query == nil {
		return []Result{}, nil
	}

	searchService := getSearchService().
		From(0).
		Size(limit).
		MinScore(types.Float64(minDuplicateScore)).
		Query(query)

	res, err := searchService.Do(ctx)
	if err != nil {
		return nil, WithESError(err)
	}

	results := make([]Result, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		results = append(results, Result{ID: *hit.Id_}) //nolint:exhaustruct
	}
	return results, nil
}
