package transform_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/transform"
)

func TestClassRegistry_CoreTypes(t *testing.T) {
	t.Parallel()

	// Verify that core init registered the expected class IDs.
	assert.Equal(t, reflect.TypeFor[core.Class](), transform.ClassRegistry[identifier.From(core.Namespace, "CLASS")])
	assert.Equal(t, reflect.TypeFor[core.Property](), transform.ClassRegistry[identifier.From(core.Namespace, "PROPERTY")])
	assert.Equal(t, reflect.TypeFor[core.Language](), transform.ClassRegistry[identifier.From(core.Namespace, "LANGUAGE")])
	assert.Equal(t, reflect.TypeFor[core.Unit](), transform.ClassRegistry[identifier.From(core.Namespace, "UNIT")])
	assert.Equal(t, reflect.TypeFor[core.ValueType](), transform.ClassRegistry[identifier.From(core.Namespace, "VALUE_TYPE")])

	// Verify that abstract classes are NOT registered.
	_, hasDocument := transform.ClassRegistry[identifier.From(core.Namespace, "DOCUMENT")]
	assert.False(t, hasDocument, "abstract class DOCUMENT should not be registered")
	_, hasVocabulary := transform.ClassRegistry[identifier.From(core.Namespace, "VOCABULARY")]
	assert.False(t, hasVocabulary, "abstract class VOCABULARY should not be registered")
}
