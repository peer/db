// Package icu provides Go approximations of Elasticsearch's icu_folding token filter and its
// icu_tokenizer, so search-time text can be normalized the same way the und_text analyzer normalizes
// indexed text.
//
// Fold applies the UTR#30 search-term foldings (NFKC normalization, Unicode case folding, accent and
// diacritic removal, dash/space/native-digit/symbol folding, and default-ignorable removal). Tokenize
// segments text into word tokens the way a UAX#29 word break iterator does, keeping runs that contain
// letters or numbers and dropping punctuation and whitespace. Tokens combines the two (tokenize, then
// fold each token), the equivalent of the und_text analyzer.
//
// The foldings are driven by the actual UTR#30 data files (data/*.txt, vendored from Lucene's ICU
// analysis module) combined with golang.org/x/text for NFKC and case folding. Fold is a close
// behavioural match: it reproduces every assertion in Lucene's TestICUFoldingFilter. Tokenize is a
// UAX#29 approximation that matches Lucene's TestICUTokenizer for Latin (including the Hebrew acronym
// rules), CJK (per-ideograph), and the space-separated alphabetic scripts, but does NOT implement ICU's
// dictionary break data, so scripts without word spaces (Thai, Lao, Khmer, Myanmar) and emoji sequences
// are not segmented the way ICU segments them.
package icu

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Fold applies the icu_folding foldings to s: NFKC normalization, Unicode case folding, accent and
// diacritic removal, the UTR#30 dash/space/native-digit/symbol foldings, and default-ignorable
// removal. The result is a normalized, case- and accent-insensitive form suitable for matching.
func Fold(s string) string {
	// NFKC folds width, ligatures, sub/superscripts, fractions and other compatibility forms.
	s = norm.NFKC.String(s)
	// Unicode case folding, a superset of lowercasing (handles final sigma, sharp s, and similar).
	s = cases.Fold().String(s)
	// Decompose so combining marks become separate runes for removal.
	s = norm.NFD.String(s)

	m := foldMap()
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := m[r]; ok {
			// A UTR#30 folding: a base letter for a stroke/hook/descender letter, an ASCII digit for a
			// native digit, a canonical dash or space, or the empty string for a removed diacritic.
			b.WriteString(repl)
			continue
		}
		// A combining mark not explicitly listed, or a default-ignorable code point, is dropped.
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	// Recompose the survivors.
	return norm.NFC.String(b.String())
}

// Tokenize splits s into word tokens the way icu_tokenizer (a UAX#29 word break iterator) does for
// search: maximal runs of letters, numbers, and the marks and joiners that bind them, with each
// ideographic (Han) character its own token, and punctuation and whitespace runs dropped. Tokens are
// returned in order; the result is empty when s has no word characters.
func Tokenize(s string) []string {
	runes := []rune(s)
	var tokens []string
	start := -1 // Start index of the current token, or -1 when between tokens.

	flush := func(end int) {
		if start >= 0 {
			// A word token must carry a letter or a number; a run of only marks or joiners is not a word.
			if seg := runes[start:end]; containsLetterOrNumber(seg) {
				tokens = append(tokens, string(seg))
			}
			start = -1
		}
	}
	for i, r := range runes {
		switch {
		case isIdeographic(r):
			// Each ideographic character is a word on its own (UAX#29 word rule status WORD_IDEO).
			flush(i)
			tokens = append(tokens, string(r))
		case isWordRune(r):
			if start < 0 {
				start = i
			}
		case start >= 0 && i+1 < len(runes) && joinsWord(runes[i-1], r, runes[i+1]):
			// A joiner that sits between two compatible word runes stays in the current token per UAX#29
			// WB6/WB7 (letters) and WB11/WB12 (numbers): "O'Reilly", "21.35", "don't". A comma binds only
			// numbers, so "dogs,chase" still breaks.
		default:
			flush(i)
		}
	}
	flush(len(runes))
	return tokens
}

// Tokens tokenizes s and folds each token, the equivalent of the und_text analyzer (icu_tokenizer then
// icu_folding). Tokens that fold to the empty string are dropped.
func Tokens(s string) []string {
	toks := Tokenize(s)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if f := Fold(t); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// isWordRune reports whether r is a letter, a number, or a mark (a combining mark stays with the letter
// it follows), the runes that make up the body of a word token.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.M, r)
}

// containsLetterOrNumber reports whether seg has at least one letter or number, the condition for a run
// of word runes to be emitted as a token.
func containsLetterOrNumber(seg []rune) bool {
	for _, r := range seg {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// isIdeographic reports whether r is a Han ideograph, which the tokenizer emits as its own token.
func isIdeographic(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// isHebrewLetter reports whether r is a Hebrew-script letter (UAX#29 Hebrew_Letter), approximated as a
// letter in the Hebrew script.
func isHebrewLetter(r rune) bool {
	return unicode.IsLetter(r) && unicode.Is(unicode.Hebrew, r)
}

// midClass is the UAX#29 word-break class of a within-word joiner rune.
type midClass int

const (
	notMid    midClass = iota // Not a joiner; it breaks a word.
	midLetter                 // Joins two letters (WB6/WB7): middle dot and similar.
	midNum                    // Joins two numbers (WB11/WB12): comma, semicolon and similar.
	midNumLet                 // Joins two letters or two numbers: period and apostrophes.
	midHebrew                 // Joins two Hebrew letters (WB7b/WB7c): the double quote in acronyms.
)

// classifyMid returns the UAX#29 joiner class of r.
func classifyMid(r rune) midClass {
	switch r {
	// Period, straight apostrophe, right and left curly apostrophes.
	case '.', '\'', '\u2019', '\u2018':
		return midNumLet
	// Middle dot, Greek ano teleia, hyphenation point.
	case '\u00b7', '\u0387', '\u2027':
		return midLetter
	// Comma, semicolon, Greek question mark, Armenian full stop, Arabic comma.
	case ',', ';', '\u037e', '\u0589', '\u060c':
		return midNum
	// Straight double quote and the Hebrew gershayim, the double quote of Hebrew acronyms.
	case '"', '\u05f4':
		return midHebrew
	default:
		return notMid
	}
}

// joinsWord reports whether the mid rune binds prev and next into one token per UAX#29 WB6/WB7 (two
// letters), WB11/WB12 (two numbers), and WB7b/WB7c (a double quote between two Hebrew letters).
func joinsWord(prev, mid, next rune) bool {
	letters := unicode.IsLetter(prev) && unicode.IsLetter(next)
	numbers := unicode.IsNumber(prev) && unicode.IsNumber(next)
	switch classifyMid(mid) {
	case midLetter:
		return letters
	case midNum:
		return numbers
	case midNumLet:
		return letters || numbers
	case midHebrew:
		return isHebrewLetter(prev) && isHebrewLetter(next)
	case notMid:
		return false
	}
	return false
}
