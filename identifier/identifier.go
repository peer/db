// Package provides functions to generate PeerDB identifiers.
package identifier

import (
	"crypto/rand"
	"io"
	"regexp"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"github.com/google/uuid"
)

const (
	idLength = 22
)

var idRegex = regexp.MustCompile(`^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]{22}$`)

// FromUUID returns an UUID encoded as a PeerDB identifier.
func FromUUID(data uuid.UUID) string {
	res := base58.Encode(data[:])
	if len(res) < idLength {
		return strings.Repeat("1", idLength-len(res)) + res
	}
	return res
}

// NewRandom returns a new random PeerDB identifier.
func NewRandom() string {
	return NewRandomFromReader(rand.Reader)
}

// NewRandom returns a new random PeerDB identifier using r as a source of randomness.
func NewRandomFromReader(r io.Reader) string {
	// We read one byte more than 128 bits, to always get full length.
	data := make([]byte, 17) //nolint:gomnd
	_, err := io.ReadFull(r, data)
	if err != nil {
		panic(err)
	}
	res := base58.Encode(data)
	return res[0:idLength]
}

// Valid returns true if id string looks like a valid PeerDB identifier.
func Valid(id string) bool {
	return idRegex.MatchString(id)
}
