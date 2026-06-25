package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/store"
)

type testInterface any

func TestIsNoneType(t *testing.T) {
	t.Parallel()

	assert.True(t, store.TestingIsNoneType[store.None]())
	assert.False(t, store.TestingIsNoneType[any]())
	assert.False(t, store.TestingIsNoneType[any]())
	assert.False(t, store.TestingIsNoneType[testInterface]())
	assert.False(t, store.TestingIsNoneType[interface{ Foo() }]())
}
