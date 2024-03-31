package store //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testInterface interface{}

func TestIsAnyType(t *testing.T) {
	t.Parallel()

	assert.True(t, isAnyType[any]())
	assert.True(t, isAnyType[interface{}]())
	assert.False(t, isAnyType[testInterface]())
	assert.False(t, isAnyType[interface{ Foo() }]())
}
