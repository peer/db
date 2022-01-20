package identifier_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/search/identifier"
)

// TODO: Convert to a fuzzing test and a benchmark.

func TestFromUUID(t *testing.T) {
	for i := 0; i < 100000; i++ {
		u := uuid.New()
		i := identifier.FromUUID(u)
		assert.Len(t, i, 22)
	}
}

func TestFromRandom(t *testing.T) {
	for i := 0; i < 100000; i++ {
		i := identifier.NewRandom()
		assert.Len(t, i, 22)
	}
}
