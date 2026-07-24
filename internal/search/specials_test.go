package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
)

func TestSpecialSearchLanguages(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name           string
		clientLanguage string
		fallback       map[string][]string
		expected       []string
	}{
		// An empty client language returns nil, so the caller matches every label language.
		{"empty client language", "", map[string][]string{"en": {"und"}}, nil},
		// A language with no fallback entry matches only its own labels.
		{"no fallback", "en", map[string][]string{}, []string{"en"}},
		// A language matches its own labels followed by its fallback chain, in order; the undetermined
		// language is dropped because no special-value labels exist for it.
		{"with fallback", "sl", map[string][]string{"sl": {"en", "und"}}, []string{"sl", "en"}},
		// The default site's synthetic fallback (undetermined only) leaves just the client language.
		{"undetermined only fallback", "en", map[string][]string{"en": {"und"}}, []string{"en"}},
		// The client language is not repeated when it also appears in its own fallback chain.
		{"client repeated in fallback", "en", map[string][]string{"en": {"en", "und"}}, []string{"en"}},
		// Duplicates within the fallback chain are collapsed.
		{"duplicate fallback", "sl", map[string][]string{"sl": {"en", "en", "und"}}, []string{"sl", "en"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := internalSearch.SpecialSearchLanguages(tt.clientLanguage, tt.fallback)
			assert.Equal(t, tt.expected, got)
		})
	}
}
