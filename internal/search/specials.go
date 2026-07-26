package search

import (
	"io/fs"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

// SpecialSearchLanguages returns the languages the special-value filter search should match a client's
// query against: the client's (already resolved) language followed by the languages in its fallback
// chain, deduplicated. So a language with no configured fallback matches only its own labels, while one
// with a fallback also matches the fallback languages' labels, letting the site control whether a
// client-language search stays within that language or widens to others. The fallback argument is the
// per-language fallback chains from EnabledLanguagesFromLanguagePriority. The undetermined language
// is dropped from the fallback: the special-value labels are UI translations with no undetermined
// variant, so keeping it would only ever match nothing. It returns nil for an empty client language
// (the caller then matches every label language).
func SpecialSearchLanguages(clientLanguage string, fallback map[string][]string) []string {
	if clientLanguage == "" {
		return nil
	}
	// The client's own language always applies; its fallback chain widens the search, minus the
	// undetermined language, which carries no special-value labels.
	out := []string{clientLanguage}
	seen := map[string]bool{clientLanguage: true}
	for _, lang := range fallback[clientLanguage] {
		if seen[lang] || lang == document.UndeterminedLanguage {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	return out
}

// specialSearchLabels holds the frontend common.values translations, keyed by the common.values key and
// then by language. It is populated by LoadSpecialSearchLabels and is nil until then. The (external)
// search package reads it through SpecialSearchLabels to match a facet's special value rows (missing,
// none, unknown, has property, direct) by name in the filter-pane value search.
//
//nolint:gochecknoglobals
var specialSearchLabels map[string]map[string]string

// LoadSpecialSearchLabels reads the frontend locale files (one "<lang>.json" per language at the root
// of localeFS, the src/locales tree) and stores their common.values entries keyed by key and language,
// replacing any previously loaded set. The main package calls this on init from the embedded locales,
// so the labels the search package matches against are the exact strings the frontend renders and need
// not be duplicated in Go. Only common.values is read; the rest of the message catalog is ignored.
func LoadSpecialSearchLabels(localeFS fs.FS) errors.E {
	entries, err := fs.ReadDir(localeFS, ".")
	if err != nil {
		return errors.WithStack(err)
	}
	labels := map[string]map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		lang := strings.TrimSuffix(name, ".json")
		data, err := fs.ReadFile(localeFS, name)
		if err != nil {
			return errors.WithStack(err)
		}
		var parsed struct {
			Common struct {
				Values map[string]string `json:"values"`
			} `json:"common"`
		}
		errE := x.Unmarshal(data, &parsed)
		if errE != nil {
			errors.Details(errE)["locale"] = name
			return errE
		}
		for key, label := range parsed.Common.Values {
			if label == "" {
				continue
			}
			if labels[key] == nil {
				labels[key] = map[string]string{}
			}
			labels[key][lang] = label
		}
	}
	specialSearchLabels = labels
	return nil
}

// SpecialSearchLabels returns the loaded common.values labels keyed by key and then language, or nil
// when none have been loaded. The returned map must not be modified.
func SpecialSearchLabels() map[string]map[string]string {
	return specialSearchLabels
}
