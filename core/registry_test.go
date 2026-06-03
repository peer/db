package core_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
)

func TestClassRegistry_CoreTypes(t *testing.T) {
	t.Parallel()

	// Verify that core init registered the expected class IDs.
	assert.Equal(t, reflect.TypeFor[core.Class](), core.ClassRegistry[identifier.From(core.Namespace, "CLASS")])
	assert.Equal(t, reflect.TypeFor[core.Property](), core.ClassRegistry[identifier.From(core.Namespace, "PROPERTY")])
	assert.Equal(t, reflect.TypeFor[core.Language](), core.ClassRegistry[identifier.From(core.Namespace, "LANGUAGE")])
	assert.Equal(t, reflect.TypeFor[core.Unit](), core.ClassRegistry[identifier.From(core.Namespace, "UNIT")])
	assert.Equal(t, reflect.TypeFor[core.ValueType](), core.ClassRegistry[identifier.From(core.Namespace, "VALUE_TYPE")])

	// Verify that the abstract VOCABULARY class is NOT registered.
	_, hasVocabulary := core.ClassRegistry[identifier.From(core.Namespace, "VOCABULARY")]
	assert.False(t, hasVocabulary, "abstract class VOCABULARY should not be registered")
}
