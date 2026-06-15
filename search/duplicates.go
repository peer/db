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
// matched structurally against the index. Each clause is a nested query matching documents that have
// the same property and value, wrapped in constant_score so that matching it contributes exactly the
// claim type's weight to the document's score, regardless of corpus term statistics.
//
// Only top-level claims are considered (sub-claims are not walked), and only those at or above
// LowConfidence, mirroring what the indexer keeps. Identical clauses (same type, property and value)
// are emitted once, so repeating a value does not double-count. enabledLanguages scopes the
// per-language string fields the string clauses query; it falls back to all supported languages.
func duplicateClauses(doc *document.D, enabledLanguages []string) ([]types.QueryVariant, []identifier.Identifier) {
	if doc == nil || doc.Claims == nil {
		return nil, nil
	}

	var clauses []types.QueryVariant
	var distinctFrom []identifier.Identifier
	seen := map[string]bool{}
	add := func(key string, weight float32, query types.QueryVariant) {
		if query == nil || seen[key] {
			return
		}
		seen[key] = true
		clauses = append(clauses, esdsl.NewConstantScoreQuery(query).Boost(weight))
	}

	c := doc.Claims

	for i := range c.Identifier {
		claim := &c.Identifier[i]
		if claim.GetConfidence() < document.LowConfidence || claim.Value == "" {
			continue
		}
		add("id\x00"+claim.Prop.ID.String()+"\x00"+claim.Value, identifierDuplicateWeight, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.id.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewMatchPhraseQuery("claims.id.value", claim.Value),
			),
		).Path("claims.id"))
	}

	for i := range c.String {
		claim := &c.String[i]
		if claim.GetConfidence() < document.LowConfidence || claim.String == "" {
			continue
		}
		add("string\x00"+claim.Prop.ID.String()+"\x00"+claim.String, stringDuplicateWeight,
			stringDuplicateNested(claim.Prop.ID, claim.String, enabledLanguages))
	}

	for i := range c.HTML {
		claim := &c.HTML[i]
		if claim.GetConfidence() < document.LowConfidence || claim.HTML == "" {
			continue
		}
		add("html\x00"+claim.Prop.ID.String()+"\x00"+claim.HTML, htmlDuplicateWeight,
			htmlDuplicateNested(claim.Prop.ID, claim.HTML, enabledLanguages))
	}

	for i := range c.Link {
		claim := &c.Link[i]
		if claim.GetConfidence() < document.LowConfidence || claim.IRI == "" {
			continue
		}
		add("link\x00"+claim.Prop.ID.String()+"\x00"+claim.IRI, linkDuplicateWeight, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.link.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewMatchPhraseQuery("claims.link.iri", claim.IRI),
			),
		).Path("claims.link"))
	}

	for i := range c.Reference {
		claim := &c.Reference[i]
		if claim.GetConfidence() < document.LowConfidence {
			continue
		}
		if claim.Prop.ID == internalCore.DistinctFromPropID {
			// The target is asserted to be a different entity, so it is excluded from the results (in
			// duplicatesQuery) rather than scored as a similarity.
			distinctFrom = append(distinctFrom, claim.To.ID)
			continue
		}
		// The index expands a reference to the target and all its hierarchy ancestors, so matching the
		// stated (most-specific) target also matches documents that reference a narrower value of it.
		add("ref\x00"+claim.Prop.ID.String()+"\x00"+claim.To.ID.String(), referenceDuplicateWeight, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(claim.To.ID.String())),
			),
		).Path("claims.ref"))
	}

	for i := range c.Amount {
		claim := &c.Amount[i]
		if claim.GetConfidence() < document.LowConfidence {
			continue
		}
		add("amount\x00"+claim.Prop.ID.String()+"\x00"+claim.Amount.String()+"\x00"+strconv.FormatFloat(claim.Precision, 'g', -1, 64),
			amountDuplicateWeight, amountDuplicateNested(claim))
	}

	for i := range c.Time {
		claim := &c.Time[i]
		if claim.GetConfidence() < document.LowConfidence {
			continue
		}
		add("time\x00"+claim.Prop.ID.String()+"\x00"+claim.Time.String()+"\x00"+strconv.Itoa(int(claim.Precision)),
			timeDuplicateWeight, timeDuplicateNested(claim))
	}

	for i := range c.Has {
		claim := &c.Has[i]
		if claim.GetConfidence() < document.LowConfidence {
			continue
		}
		add("has\x00"+claim.Prop.ID.String(), hasDuplicateWeight, esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.has.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
		).Path("claims.has"))
	}

	return clauses, distinctFrom
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

// amountDuplicateNested matches documents whose amount claim for the same property has a value window
// overlapping this claim's window. The window is computed exactly as the indexer computes the stored
// range, so an overlapping range query (the default range-on-range INTERSECTS relation) finds the same
// or an indistinguishable value. It returns nil (the claim is skipped) when the window cannot be
// computed. Units are not constrained: a property carries a consistent measure, so the property already
// scopes the comparison.
func amountDuplicateNested(claim *document.AmountClaim) types.QueryVariant { //nolint:ireturn
	from, errE := claim.Amount.WindowStartFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	to, errE := claim.Amount.WindowEndFloat64(claim.Precision, false)
	if errE != nil {
		return nil
	}
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
			esdsl.NewNumberRangeQuery("claims.amount.range").Gte(types.Float64(from)).Lte(types.Float64(to)),
		),
	).Path("claims.amount")
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
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(claim.Prop.ID.String())),
			esdsl.NewNumberRangeQuery("claims.time.range").Gte(types.Float64(from)).Lte(types.Float64(to)),
		),
	).Path("claims.time")
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
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(internalCore.DistinctFromPropID.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(exclude.String())),
			),
		).Path("claims.ref"))
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
