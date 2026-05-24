package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func TestSortedUniqueUsersEmpty(t *testing.T) {
	t.Parallel()

	assert.Nil(t, internalStore.SortedUniqueUsers(nil))
	assert.Nil(t, internalStore.SortedUniqueUsers([]*internalStore.User{}))
}

func TestSortedUniqueUsersSkipsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, internalStore.SortedUniqueUsers([]*internalStore.User{nil, nil}))
	got := internalStore.SortedUniqueUsers([]*internalStore.User{nil, {ID: "a"}, nil})
	assert.Equal(t, []internalStore.User{{ID: "a"}}, got)
}

func TestSortedUniqueUsersDedupesAndSorts(t *testing.T) {
	t.Parallel()

	got := internalStore.SortedUniqueUsers([]*internalStore.User{
		{ID: "c"}, {ID: "a"}, {ID: "b"}, {ID: "a"}, {ID: "c"},
	})
	assert.Equal(t, []internalStore.User{{ID: "a"}, {ID: "b"}, {ID: "c"}}, got)
}
