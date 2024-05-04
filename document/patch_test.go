package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

func TestPatchJSON(t *testing.T) {
	t.Parallel()

	under := identifier.MustFromString("HpPn1Ra6SLdjWaDxaJJYx3")
	prop := identifier.MustFromString("XkbTJqwFCFkfoxMBXow4HU")
	value := "foobar"
	p := document.AddClaimPatch{
		Under: &under,
		Patch: document.IdentifierClaimPatch{
			Prop:       &prop,
			Identifier: &value,
		},
	}
	out, errE := x.MarshalWithoutEscapeHTML(p)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `{"under":"HpPn1Ra6SLdjWaDxaJJYx3","patch":{"prop":"XkbTJqwFCFkfoxMBXow4HU","id":"foobar"},"type":"id"}`, string(out))

	var p2 document.AddClaimPatch
	errE = x.UnmarshalWithoutUnknownFields(out, &p2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, p, p2)
}
