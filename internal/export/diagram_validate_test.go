package export_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

// TestEmbedsStruct exercises the embedding check against real core types:
// core.Class embeds both core.ClassFields and core.DocumentFields, while
// core.ClassFields embeds neither.
func TestEmbedsStruct(t *testing.T) {
	t.Parallel()

	classType := reflect.TypeFor[core.Class]()
	classFields := reflect.TypeFor[core.ClassFields]()
	documentFields := reflect.TypeFor[core.DocumentFields]()

	assert.True(t, export.TestingEmbedsStruct(documentFields, documentFields), "a type embeds itself")
	assert.True(t, export.TestingEmbedsStruct(classType, documentFields), "core.Class embeds DocumentFields")
	assert.True(t, export.TestingEmbedsStruct(classType, classFields), "core.Class embeds ClassFields")
	assert.False(t, export.TestingEmbedsStruct(classFields, documentFields), "core.ClassFields does not embed DocumentFields")
	assert.False(t, export.TestingEmbedsStruct(documentFields, classFields), "unrelated types do not embed each other")
}

// TestValidateDiagramTypes verifies that a class type missing the shared
// DocumentFields or missing its registered own-fields struct each produce a
// warning, while a well-formed class produces none.
func TestValidateDiagramTypes(t *testing.T) {
	t.Parallel()

	documentFields := reflect.TypeFor[core.DocumentFields]()

	goodID := identifier.From("test", "GOOD")
	missingSharedID := identifier.From("test", "MISSING_SHARED")
	missingOwnID := identifier.From("test", "MISSING_OWN")

	classRegistry := map[identifier.Identifier]reflect.Type{
		// Embeds both ClassFields and DocumentFields.
		goodID: reflect.TypeFor[core.Class](),
		// ClassFields embeds neither DocumentFields nor itself's parent.
		missingSharedID: reflect.TypeFor[core.ClassFields](),
		// Embeds DocumentFields but not the PropertyFields registered below.
		missingOwnID: reflect.TypeFor[core.Class](),
	}
	classFieldsRegistry := map[identifier.Identifier]reflect.Type{
		goodID:          reflect.TypeFor[core.ClassFields](),
		missingSharedID: reflect.TypeFor[core.ClassFields](),
		missingOwnID:    reflect.TypeFor[core.PropertyFields](),
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	export.TestingValidateDiagramTypes(logger, classRegistry, classFieldsRegistry, documentFields)

	out := buf.String()

	// Exactly the two misconfigured classes are flagged, one warning each.
	assert.Equal(t, 2, strings.Count(out, `"level":"warn"`), "expected one warning per misconfigured class")

	// The well-formed class embeds both, so it is never mentioned.
	assert.NotContains(t, out, goodID.String(), "well-formed class should not be flagged")

	// The class missing DocumentFields is flagged for the shared fields.
	assert.Contains(t, out, missingSharedID.String())
	assert.Contains(t, out, "does not embed the shared DocumentFields the diagram assumes")

	// The class missing its registered own-fields struct is flagged for that.
	assert.Contains(t, out, missingOwnID.String())
	assert.Contains(t, out, "does not embed its ClassFieldsRegistry fields struct")
}
