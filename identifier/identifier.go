package identifier

import (
	"crypto/rand"
	"io"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"github.com/google/uuid"
)

func FromUUID(data uuid.UUID) string {
	res := base58.Encode(data[:])
	if len(res) < 22 {
		return strings.Repeat("1", 22-len(res)) + res
	}
	return res
}

func NewRandom() string {
	return NewRandomFromReader(rand.Reader)
}

func NewRandomFromReader(r io.Reader) string {
	// We read one byte more than 128 bits, to always get full length.
	data := make([]byte, 17)
	_, err := io.ReadFull(r, data)
	if err != nil {
		panic(err)
	}
	res := base58.Encode(data)
	return res[0:22]
}
