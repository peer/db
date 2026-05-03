package store //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testInterface any

func TestIsNoneType(t *testing.T) {
	t.Parallel()

	assert.True(t, isNoneType[None]())
	assert.False(t, isNoneType[any]())
	assert.False(t, isNoneType[any]())
	assert.False(t, isNoneType[testInterface]())
	assert.False(t, isNoneType[interface{ Foo() }]())
}
