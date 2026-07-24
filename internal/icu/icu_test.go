package icu_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/internal/icu"
)

func TestFold(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ascii lowercased", "Missing", "missing"},
		{"case fold sharp s", "STRASSE", "strasse"},
		// Combining diacritics are removed via NFD decomposition.
		{"combining accents", "Máos", "maos"},
		{"precomposed accents", "Máos", "maos"},
		// Slovenian caron letters fold to their base letter.
		{"slovenian caron", "manjkajoče", "manjkajoce"},
		{"slovenian all carons", "čšž", "csz"},
		// Stroke/hook/descender letters have no NFD decomposition; the UTR#30 data folds them.
		{"latin stroke letters", "øđłħ", "odlh"},
		{"latin uppercase stroke", "Ø", "o"},
		{"ligature ae", "æ", "ae"},
		// NFKC compatibility folding.
		{"fullwidth", "ＡＢＣ", "abc"},
		{"ligature ff", "ﬀ", "ff"},
		{"superscript digit", "x²", "x2"},
		{"roman numeral", "Ⅸ", "ix"},
		// Native (non-ASCII) digits fold to ASCII digits.
		{"arabic-indic digits", "٣٤", "34"},
		// Greek final sigma folds together with the medial sigma via case folding.
		{"greek sigma", "Σς", "σσ"},
		// Default ignorables (a zero-width joiner) are removed.
		{"zero width joiner", "a\u200db", "ab"},
		// Various dashes fold to the ASCII hyphen-minus.
		{"em dash", "a—b", "a-b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, icu.Fold(tt.in))
		})
	}
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single word", "missing", []string{"missing"}},
		{"two words", "has property", []string{"has", "property"}},
		{"punctuation dropped", "  has,  property! ", []string{"has", "property"}},
		{"apostrophe kept", "don't", []string{"don't"}},
		{"period in number kept", "3.14", []string{"3.14"}},
		{"trailing period split", "U.S.A.", []string{"U.S.A"}},
		{"alphanumeric run", "abc123", []string{"abc123"}},
		{"quotes stripped as punctuation", "\"none\"", []string{"none"}},
		// Han ideographs are one token each.
		{"han per character", "中文", []string{"中", "文"}}, //nolint:gosmopolitan
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, icu.Tokenize(tt.in))
		})
	}
}

func TestTokens(t *testing.T) {
	t.Parallel()

	// Tokens tokenizes then folds each token, the und_text analyzer equivalent.
	assert.Equal(t, []string{"has", "property"}, icu.Tokens("Has Property"))
	assert.Equal(t, []string{"manjkajoce"}, icu.Tokens("manjkajoče"))
	assert.Empty(t, icu.Tokens("   ,.!  "))
}
