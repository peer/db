package export_test

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/internal/export"
)

// TestExtractDiagramClassInfo_FromCoreClass verifies the reflective extraction
// of (id, mnemonic, subclass-of) from a real core.Class document. Language is
// a concrete vocabulary subclassed under VOCABULARY.
func TestExtractDiagramClassInfo_FromCoreClass(t *testing.T) {
	t.Parallel()

	doc := &core.Class{
		ClassFields: core.ClassFields{ //nolint:exhaustruct
			Mnemonic: "LANGUAGE",
			SubclassOf: []core.Ref{{
				ID: []string{core.Namespace, "VOCABULARY"},
			}},
		},
		DocumentFields: core.DocumentFields{ //nolint:exhaustruct
			ID: []string{core.Namespace, "LANGUAGE"},
		},
	}

	id, mnemonic, parents, ok := export.TestingExtractDiagramClassInfo(zerolog.Nop(), doc)
	assert.True(t, ok)
	assert.Equal(t, identifier.From(core.Namespace, "LANGUAGE"), id)
	assert.Equal(t, "LANGUAGE", mnemonic)
	assert.Equal(t, []identifier.Identifier{identifier.From(core.Namespace, "VOCABULARY")}, parents)
}

// TestExtractDiagramClassInfo_NoParents verifies that a class without
// SubclassOf returns an empty parents slice (nil).
func TestExtractDiagramClassInfo_NoParents(t *testing.T) {
	t.Parallel()

	doc := &core.Class{
		ClassFields: core.ClassFields{ //nolint:exhaustruct
			Mnemonic: "PROPERTY",
		},
		DocumentFields: core.DocumentFields{ //nolint:exhaustruct
			ID: []string{core.Namespace, "PROPERTY"},
		},
	}

	id, mnemonic, parents, ok := export.TestingExtractDiagramClassInfo(zerolog.Nop(), doc)
	assert.True(t, ok)
	assert.Equal(t, identifier.From(core.Namespace, "PROPERTY"), id)
	assert.Equal(t, "PROPERTY", mnemonic)
	assert.Empty(t, parents)
}

// TestExtractDiagramClassInfo_NilPointer rejects a nil pointer and logs a warning.
func TestExtractDiagramClassInfo_NilPointer(t *testing.T) {
	t.Parallel()
	var doc *core.Class
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, _, _, ok := export.TestingExtractDiagramClassInfo(logger, doc) //nolint:dogsled
	assert.False(t, ok)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","message":"class description is a nil pointer; skipping"}`+"\n",
		buf.String(),
	)
}

// TestExtractDiagramClassInfo_NonStruct rejects a non-struct value and logs a warning.
func TestExtractDiagramClassInfo_NonStruct(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, _, _, ok := export.TestingExtractDiagramClassInfo(logger, "just a string") //nolint:dogsled
	assert.False(t, ok)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","kind":"string","type":"string","message":"class description is not a struct; skipping"}`+"\n",
		buf.String(),
	)
}

// TestExtractDiagramClassInfo_MissingMnemonic rejects a class with an ID but
// no mnemonic and logs a warning.
func TestExtractDiagramClassInfo_MissingMnemonic(t *testing.T) {
	t.Parallel()

	doc := &core.Class{
		ClassFields: core.ClassFields{ //nolint:exhaustruct
			Mnemonic: "",
		},
		DocumentFields: core.DocumentFields{ //nolint:exhaustruct
			ID: []string{core.Namespace, "NAMELESS"},
		},
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, _, _, ok := export.TestingExtractDiagramClassInfo(logger, doc) //nolint:dogsled
	assert.False(t, ok)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","type":"core.Class","base":["core.peerdb.org","NAMELESS"],"message":"class description has no mnemonic; skipping"}`+"\n",
		buf.String(),
	)
}

// TestExtractDiagramClassInfo_MissingID rejects a class with a mnemonic but
// no documentid (empty Base) and logs a warning.
func TestExtractDiagramClassInfo_MissingID(t *testing.T) {
	t.Parallel()

	doc := &core.Class{
		ClassFields: core.ClassFields{ //nolint:exhaustruct
			Mnemonic: "ORPHAN",
		},
		DocumentFields: core.DocumentFields{ //nolint:exhaustruct
			ID: nil,
		},
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, _, _, ok := export.TestingExtractDiagramClassInfo(logger, doc) //nolint:dogsled
	assert.False(t, ok)
	assert.Equal( //nolint:testifylint
		t,
		`{"level":"warn","type":"core.Class","mnemonic":"ORPHAN","message":"class description has no documentid base; skipping"}`+"\n",
		buf.String(),
	)
}

// TestExtractDiagramClassInfo_MultipleParents verifies multi-inheritance:
// every SubclassOf Ref must surface as a parent.
func TestExtractDiagramClassInfo_MultipleParents(t *testing.T) {
	t.Parallel()

	doc := &core.Class{
		ClassFields: core.ClassFields{ //nolint:exhaustruct
			Mnemonic: "CHIMERA",
			SubclassOf: []core.Ref{
				{ID: []string{"ns", "A"}},
				{ID: []string{"ns", "B"}},
			},
		},
		DocumentFields: core.DocumentFields{ //nolint:exhaustruct
			ID: []string{"ns", "CHIMERA"},
		},
	}

	_, _, parents, ok := export.TestingExtractDiagramClassInfo(zerolog.Nop(), doc)
	assert.True(t, ok)
	assert.Equal(t, []identifier.Identifier{
		identifier.From("ns", "A"),
		identifier.From("ns", "B"),
	}, parents)
}
