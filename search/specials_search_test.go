package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/search"
)

func TestMatchedSpecials(t *testing.T) {
	t.Parallel()

	en := []string{"en"}
	sl := []string{"sl"}

	for _, tt := range []struct {
		name        string
		query       string
		langs       []string
		missing     bool
		none        bool
		unknown     bool
		hasProperty bool
		direct      bool
	}{
		{"empty", "", en, false, false, false, false, false},
		{"no special", "col*", en, false, false, false, false, false},
		{"missing prefix", "mis*", en, true, false, false, false, false},
		{"missing full", "missing*", en, true, false, false, false, false},
		{"none full", "none*", en, false, true, false, false, false},
		{"unknown prefix", "unk*", en, false, false, true, false, false},
		{"direct prefix", "dir*", en, false, false, false, false, true},
		{"has property whole prefix", "has pr*", en, false, false, false, true, false},
		// Whitespace between the query words is irrelevant: tokenization collapses it.
		{"has property double space", "has  pr*", en, false, false, false, true, false},
		{"has property tab", "has\tpr*", en, false, false, false, true, false},
		{"has property second word", "prop*", en, false, false, false, true, false},
		// A single leading letter prefixes a label.
		{"letter n", "n*", en, false, true, false, false, false},
		// A longer query that is not a prefix of the label matches nothing.
		{"beyond label", "missing person*", en, false, false, false, false, false},
		// Quoted phrases arrive with the quotes and without the wildcard.
		{"quoted none", "\"none\"", en, false, true, false, false, false},
		// Matching is case and diacritic insensitive against the Slovenian labels.
		{"sl missing folded", "manjkajoc*", sl, true, false, false, false, false},
		{"sl has property", "ima l*", sl, false, false, false, true, false},
		// An enabled-language set that excludes a label's language does not match it.
		{"en query against sl only", "missing*", sl, false, false, false, false, false},
		// Empty enabled languages fall back to every label language.
		{"fallback languages", "brez*", nil, false, true, false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := search.TestingMatchedSpecials(tt.query, tt.langs)
			assert.Equal(t, tt.missing, got.Missing, "missing")
			assert.Equal(t, tt.none, got.None, "none")
			assert.Equal(t, tt.unknown, got.Unknown, "unknown")
			assert.Equal(t, tt.hasProperty, got.HasProperty, "hasProperty")
			assert.Equal(t, tt.direct, got.Direct, "direct")
		})
	}
}
