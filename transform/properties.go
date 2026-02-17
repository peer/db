package transform

import (
	"context"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
)

// Mnemonics returns a map between property mnemonic and identifier.
//
// It takes a slice of any structs, including core.Property ones, and extracts
// the mnemonic to identifier mapping from Property documents.
//
// Returns an error if mnemonics are not unique.
func Mnemonics(ctx context.Context, documents []any) (map[string]identifier.Identifier, errors.E) {
	result := map[string]identifier.Identifier{}

	for _, doc := range documents {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		if prop, ok := doc.(*core.Property); ok {
			if prop.Mnemonic != "" && len(prop.ID) > 0 {
				if _, ok := result[prop.Mnemonic]; ok {
					errE := errors.Errorf("duplicate mnemonic")
					errors.Details(errE)["mnemonic"] = prop.Mnemonic
					return nil, errE
				}
				result[prop.Mnemonic] = identifier.From(prop.ID...)
			}
		}
	}

	return result, nil
}
