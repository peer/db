package icu

import (
	"bufio"
	"bytes"
	"embed"
	"strconv"
	"strings"
	"sync"
)

// foldingData holds the vendored UTR#30 character-folding data files. They carry the foldings NFKC and
// case folding do not: base letters for stroke/hook/descender letters, ASCII digits for native digits,
// canonical dashes and spaces, and diacritic removals. nfc, nfkc and nfkc_cf are intentionally not
// vendored, since golang.org/x/text supplies NFKC and case folding.
//
//go:embed data/*.txt
var foldingData embed.FS

// foldMap returns the parsed UTR#30 folding map: each source rune maps to its folded replacement
// string (the empty string means the rune is removed). It is built once from the vendored data files.
//
//nolint:gochecknoglobals
var foldMap = sync.OnceValue(buildFoldMap)

func buildFoldMap() map[rune]string {
	m := map[rune]string{}
	entries, err := foldingData.ReadDir("data")
	if err != nil {
		// The data is embedded, so this cannot fail at run time; an empty map degrades Fold to NFKC and
		// case folding rather than panicking.
		return m
	}
	for _, entry := range entries {
		data, err := foldingData.ReadFile("data/" + entry.Name())
		if err != nil {
			continue
		}
		parseFoldingFile(data, m)
	}
	return m
}

// parseFoldingFile parses one UTR#30 folding file into m. A "#" starts a comment. Each remaining line
// is "SRC>DST", where SRC is a hex code point or a "lo..hi" hex range and DST is space-separated hex
// code points (an empty DST removes the source runes).
func parseFoldingFile(data []byte, m map[rune]string) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if before, _, found := strings.Cut(line, "#"); found {
			line = before
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		srcStr, dstStr, found := strings.Cut(line, ">")
		if !found {
			continue
		}
		lo, hi, ok := parseSourceRange(strings.TrimSpace(srcStr))
		if !ok {
			continue
		}
		repl := decodeReplacement(strings.TrimSpace(dstStr))
		for r := lo; r <= hi; r++ {
			m[r] = repl
		}
	}
}

// parseSourceRange parses a folding source: a single hex code point ("00F8") or a hex range
// ("2010..2015"). It returns the inclusive rune bounds and whether parsing succeeded.
func parseSourceRange(src string) (rune, rune, bool) {
	if loStr, hiStr, found := strings.Cut(src, ".."); found {
		lo, err1 := strconv.ParseInt(loStr, 16, 32)
		hi, err2 := strconv.ParseInt(hiStr, 16, 32)
		if err1 != nil || err2 != nil || hi < lo {
			return 0, 0, false
		}
		return rune(lo), rune(hi), true
	}
	cp, err := strconv.ParseInt(src, 16, 32)
	if err != nil {
		return 0, 0, false
	}
	return rune(cp), rune(cp), true
}

// decodeReplacement decodes a folding replacement (space-separated hex code points) into a string. An
// empty input yields the empty string (the source runes are removed).
func decodeReplacement(dst string) string {
	if dst == "" {
		return ""
	}
	var b strings.Builder
	for h := range strings.FieldsSeq(dst) {
		cp, err := strconv.ParseInt(h, 16, 32)
		if err != nil {
			continue
		}
		b.WriteRune(rune(cp))
	}
	return b.String()
}
