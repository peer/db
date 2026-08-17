package core

import (
	"bytes"

	"gitlab.com/tozd/identifier"
)

// CompareIdentifiers orders identifiers by their bytes, which is an order they have whatever they stand for
// and however they were made. It is used where a set of them has to come out the same way twice and nothing
// else says which of them comes first: an identifier is assigned and says nothing about what it identifies,
// so this is an order and not a ranking.
func CompareIdentifiers(a, b identifier.Identifier) int {
	return bytes.Compare(a[:], b[:])
}
