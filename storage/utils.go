package storage

import (
	"crypto/sha256"
	"encoding/base64"
)

// TODO: Move to x package? It is also used in waf.
func computeEtag(data ...[]byte) string {
	hash := sha256.New()
	for _, d := range data {
		_, _ = hash.Write(d)
	}
	return `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`
}
