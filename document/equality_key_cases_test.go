package document_test

import (
	_ "embed"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
)

// equalityKeyCasesJSON is the shared corpus run by both this Go backend test and the TypeScript
// frontend test (src/document/equality-key-cases.test.ts). Each case is one claim of a claim type
// with the four keys it produces, so the two implementations are held to the same contract: what a
// claim says has to read the same on the backend and in the editor, or a claim which the one counts
// as a repeat of another the other counts as new (see Claim.EqualityKey).
//
// Regenerate the keys after changing what a claim says with:
//
//	WRITE_EQUALITY_KEY_CASES=1 go test ./document/ -run TestEqualityKeyCasesCorpus
//
//go:embed testdata/equality-key-cases.json
var equalityKeyCasesJSON []byte

const equalityKeyCasesPath = "testdata/equality-key-cases.json"

//nolint:tagliatelle // The corpus names the keys after the identity they carry, which we spell ID.
type equalityKeyCase struct {
	Name            string          `json:"name"`
	Claims          json.RawMessage `json:"claims"`
	Key             string          `json:"key"`
	KeyWithID       string          `json:"keyWithID"`
	KeyWithSub      string          `json:"keyWithSub"`
	KeyWithIDAndSub string          `json:"keyWithIDAndSub"`
}

// caseClaim returns the single claim a case's claims hold.
func caseClaim(t *testing.T, claims json.RawMessage) document.Claim { //nolint:ireturn
	t.Helper()

	var claimTypes document.ClaimTypes
	errE := x.UnmarshalWithoutUnknownFields(claims, &claimTypes)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Equal(t, 1, claimTypes.Size())

	for claim := range claimTypes.AllClaims() {
		return claim
	}
	require.FailNow(t, "no claim")
	return nil
}

// caseKeys are the four keys of a case's claim, in the order the case records them.
func caseKeys(t *testing.T, claim document.Claim) [4]string {
	t.Helper()

	var keys [4]string
	for i, ask := range [4][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		key, errE := claim.EqualityKey(ask[0], ask[1])
		require.NoError(t, errE, "% -+#.1v", errE)
		keys[i] = key
	}
	return keys
}

func TestEqualityKeyCasesCorpus(t *testing.T) {
	t.Parallel()

	var corpus struct {
		Cases []equalityKeyCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(equalityKeyCasesJSON, &corpus))
	require.NotEmpty(t, corpus.Cases)

	keys := make([][4]string, len(corpus.Cases))
	for i, c := range corpus.Cases {
		keys[i] = caseKeys(t, caseClaim(t, c.Claims))
	}

	// Regenerating writes the keys the current implementation produces; the frontend test then holds
	// the editor to them.
	if os.Getenv("WRITE_EQUALITY_KEY_CASES") != "" {
		for i := range corpus.Cases {
			corpus.Cases[i].Key = keys[i][0]
			corpus.Cases[i].KeyWithID = keys[i][1]
			corpus.Cases[i].KeyWithSub = keys[i][2]
			corpus.Cases[i].KeyWithIDAndSub = keys[i][3]
		}
		data, err := json.MarshalIndent(corpus, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(equalityKeyCasesPath, append(data, '\n'), 0o600))
		return
	}

	for i, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Key, keys[i][0], "EqualityKey(false, false)")
			assert.Equal(t, c.KeyWithID, keys[i][1], "EqualityKey(true, false)")
			assert.Equal(t, c.KeyWithSub, keys[i][2], "EqualityKey(false, true)")
			assert.Equal(t, c.KeyWithIDAndSub, keys[i][3], "EqualityKey(true, true)")

			// The ID is in the key only when asked for, and the sub-claims are added after it.
			assert.NotEqual(t, keys[i][0], keys[i][1])
			assert.Equal(t, keys[i][0], keys[i][2][:len(keys[i][0])])
		})
	}

	// The presence-only claims carry the same fields, so only their type tells them apart.
	has := caseByName(t, corpus.Cases, keys, "has")
	none := caseByName(t, corpus.Cases, keys, "none")
	unknown := caseByName(t, corpus.Cases, keys, "unknown")
	assert.NotEqual(t, has[0], none[0], "a has claim does not say what a none claim says")
	assert.NotEqual(t, has[0], unknown[0], "nor what an unknown claim says")
	assert.NotEqual(t, none[0], unknown[0], "and a none claim does not say what an unknown claim says")

	// The two cases holding the same sub-claims in the other order say the same thing, and are told
	// apart only by their identities.
	first := caseByName(t, corpus.Cases, keys, "claim with sub-claims")
	second := caseByName(t, corpus.Cases, keys, "claim with sub-claims in another order")
	assert.Equal(t, first[2], second[2], "sub-claim order does not decide the key")
	assert.NotEqual(t, first[3], second[3], "the identities do")
}

// caseByName returns the keys of the named case.
func caseByName(t *testing.T, cases []equalityKeyCase, keys [][4]string, name string) [4]string {
	t.Helper()

	for i, c := range cases {
		if c.Name == name {
			return keys[i]
		}
	}
	require.FailNow(t, "case not found", name)
	return [4]string{}
}
