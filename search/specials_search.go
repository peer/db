package search

import (
	"strings"

	"gitlab.com/peerdb/peerdb/internal/icu"
	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

// DirectValueID is the search token for the "direct" special. It has no standalone result row (the
// rendered direct entries carry DirectRefFilterPrefix followed by their value id); it exists only so
// the filter-pane value search can recognize the "direct" keyword like the other special labels.
const DirectValueID = "__DIRECT__"

// specialLabelKeyToID maps a frontend common.values.* key to the special value id it labels. It is the
// searchable subset of the loaded value labels: the filter-pane value search matches the typed text
// against these labels so a user can find a facet's special rows (missing, none, unknown, has property)
// and its direct entries by name, exactly as real values are searched. Elasticsearch cannot match these
// synthetic rows (they correspond to no indexed document text), so the label match is done here in Go
// and then translated into the existing special-presence queries and discovery gates. The labels
// themselves are loaded from the frontend translations by internalSearch.LoadSpecialSearchLabels.
//
//nolint:gochecknoglobals
var specialLabelKeyToID = map[string]string{
	"missing":     MissingValueID,
	"none":        NoneValueID,
	"unknown":     UnknownValueID,
	"hasProperty": HasPropertyValueID,
	"direct":      DirectValueID,
}

// Languages carries the resolved per-request language sets a filter search uses.
// Enabled is the site's enabled languages (its LanguagePriority keys plus the undetermined language).
// Special is the languages the special-value label search matches against: the client's resolved
// language and its fallback chain (see internalSearch.SpecialSearchLanguages), so the site controls
// whether a client-language search stays within that language or widens to its fallbacks. A nil
// *Languages means neither set is configured: the label queries then match every supported language
// and the special-value search matches every label language, which the tests rely on.
type Languages struct {
	Enabled []string
	Special []string
}

// enabled returns the site's enabled languages, or nil for a nil receiver (matching every supported
// language in the label queries).
func (l *Languages) enabled() []string {
	if l == nil {
		return nil
	}
	return l.Enabled
}

// special returns the special-value search languages, or nil for a nil receiver (matching every label
// language).
func (l *Languages) special() []string {
	if l == nil {
		return nil
	}
	return l.Special
}

// requestedSpecials records which special values a filter-pane value query matched by label.
type requestedSpecials struct {
	Missing     bool
	None        bool
	Unknown     bool
	HasProperty bool
	Direct      bool
}

// Any reports whether the query matched any special value label.
func (r requestedSpecials) Any() bool {
	return r.Missing || r.None || r.Unknown || r.HasProperty || r.Direct
}

// matchedSpecials reports which special value labels the filter-pane value query prefix-matches, in the
// given languages (the special-value search languages, the client's language and its fallback chain;
// empty matches every label language). The query and each label are folded and split into words the
// same way the und_text analyzer normalizes indexed text (icu.Tokens): folding drops case, accents and
// width, and tokenization drops the frontend's trailing prefix "*" and any quotes as punctuation and
// collapses whitespace, so the query needs no trimming. A query with no words matches nothing.
func matchedSpecials(valueQuery string, languages []string) requestedSpecials {
	queryTokens := icu.Tokens(valueQuery)
	if len(queryTokens) == 0 {
		return requestedSpecials{}
	}

	langWanted := map[string]bool{}
	for _, lang := range languages {
		langWanted[lang] = true
	}

	// The labels are the frontend common.values translations, keyed by key then language; the special
	// search matches only the keys in specialLabelKeyToID.
	labels := internalSearch.SpecialSearchLabels()
	var out requestedSpecials
	for key, id := range specialLabelKeyToID {
		matched := false
		for lang, label := range labels[key] {
			// An empty enabled-language set means match against every label language.
			if len(langWanted) > 0 && !langWanted[lang] {
				continue
			}
			if labelTokensMatch(queryTokens, icu.Tokens(label)) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		switch id {
		case MissingValueID:
			out.Missing = true
		case NoneValueID:
			out.None = true
		case UnknownValueID:
			out.Unknown = true
		case HasPropertyValueID:
			out.HasProperty = true
		case DirectValueID:
			out.Direct = true
		}
	}
	return out
}

// labelTokensMatch reports whether the folded query words match the folded label words. A single query
// word prefix-matches any label word, so "prop" finds "has property"; multiple query words must prefix
// the label words in order from the start, so "has pr" finds "has property" (and any extra whitespace
// between the query words is irrelevant, since tokenization dropped it). Both slices are already folded
// and tokenized by icu.Tokens.
func labelTokensMatch(queryTokens, labelTokens []string) bool {
	switch {
	case len(queryTokens) == 0:
		return false
	case len(queryTokens) == 1:
		for _, labelToken := range labelTokens {
			if strings.HasPrefix(labelToken, queryTokens[0]) {
				return true
			}
		}
		return false
	default:
		if len(queryTokens) > len(labelTokens) {
			return false
		}
		last := len(queryTokens) - 1
		for i := range last {
			if queryTokens[i] != labelTokens[i] {
				return false
			}
		}
		return strings.HasPrefix(labelTokens[last], queryTokens[last])
	}
}
