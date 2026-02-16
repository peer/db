//nolint:testpackage
package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
)

//nolint:exhaustruct
func TestMnemonics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		documents []any
		expected  map[string]identifier.Identifier
		wantError bool
		errorMsg  string
	}{
		{
			name:      "EmptySlice",
			documents: []any{},
			expected:  map[string]identifier.Identifier{},
			wantError: false,
		},
		{
			name: "SingleProperty",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop1"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"NAME": identifier.From("prop1"),
			},
			wantError: false,
		},
		{
			name: "MultipleProperties",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop1"},
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "AGE",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "EMAIL",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop3"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"NAME":  identifier.From("prop1"),
				"AGE":   identifier.From("prop2"),
				"EMAIL": identifier.From("prop3"),
			},
			wantError: false,
		},
		{
			name: "PropertyWithoutMnemonic",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop1"},
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"NAME": identifier.From("prop2"),
			},
			wantError: false,
		},
		{
			name: "PropertyWithoutID",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{},
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "AGE",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"AGE": identifier.From("prop2"),
			},
			wantError: false,
		},
		{
			name: "PropertyWithNilID",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: nil,
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "AGE",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"AGE": identifier.From("prop2"),
			},
			wantError: false,
		},
		{
			name: "DuplicateMnemonic",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop1"},
					},
				},
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
			},
			expected:  nil,
			wantError: true,
			errorMsg:  "duplicate mnemonic",
		},
		{
			name: "MixedDocumentTypes",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "NAME",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop1"},
					},
				},
				"not a property",
				123,
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "AGE",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"prop2"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"NAME": identifier.From("prop1"),
				"AGE":  identifier.From("prop2"),
			},
			wantError: false,
		},
		{
			name: "NonPropertyDocuments",
			documents: []any{
				"string",
				123,
				map[string]string{"key": "value"},
			},
			expected:  map[string]identifier.Identifier{},
			wantError: false,
		},
		{
			name: "PropertyWithMultiPartID",
			documents: []any{
				&core.Property{
					PropertyFields: core.PropertyFields{
						Mnemonic: "COMPLEX_ID",
					},
					DocumentFields: core.DocumentFields{
						ID: []string{"part1", "part2", "part3"},
					},
				},
			},
			expected: map[string]identifier.Identifier{
				"COMPLEX_ID": identifier.From("part1", "part2", "part3"),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, errE := Mnemonics(tt.documents)

			if tt.wantError {
				assert.EqualError(t, errE, tt.errorMsg)
				assert.Nil(t, result)
			} else {
				require.NoError(t, errE, "% -+#.1v", errE)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
