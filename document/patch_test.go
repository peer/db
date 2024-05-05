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
	changes := document.Changes{
		document.AddClaimChange{
			Under: &under,
			Patch: document.IdentifierClaimPatch{
				Prop:       &prop,
				Identifier: &value,
			},
		},
	}
	out, errE := x.MarshalWithoutEscapeHTML(changes)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, `[{"type":"add","under":"HpPn1Ra6SLdjWaDxaJJYx3","patch":{"type":"id","prop":"XkbTJqwFCFkfoxMBXow4HU","id":"foobar"}}]`, string(out))

	var changes2 document.Changes
	errE = x.UnmarshalWithoutUnknownFields(out, &changes2)
	assert.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changes, changes2)
}
