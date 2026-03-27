package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFallbackLanguages(t *testing.T) {
	t.Parallel()

	// With priority set.
	priority := map[string][]string{
		"en": {"sl", "und"},
		"sl": {"en"},
		"pt": {},
	}

	// Language with explicit fallbacks.
	assert.Equal(t, []string{"sl", "und"}, getFallbackLanguages("en", priority))
	assert.Equal(t, []string{"en"}, getFallbackLanguages("sl", priority))

	// Language with empty fallback list: no fallback at all.
	assert.Empty(t, getFallbackLanguages("pt", priority))

	// Language not in priority: fallback to "und".
	assert.Equal(t, []string{"und"}, getFallbackLanguages("fr", priority))

	// "und" not in priority: no fallback (it's already undetermined).
	assert.Nil(t, getFallbackLanguages("und", priority))

	// With nil priority.
	assert.Equal(t, []string{"und"}, getFallbackLanguages("en", nil))
	assert.Nil(t, getFallbackLanguages("und", nil))
}
