package transform

import (
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/shortcut"
)

// shortcutSelfValue is the magic value token that the frontend resolves to the
// containing document's own ID at render time.
const shortcutSelfValue = "self"

// shortcutReverseKey is the special key that scopes the search to documents
// referencing the value via any property.
const shortcutReverseKey = "reverse"

// shortcutMaxKeyParts caps the number of ":"-separated parts a key may have
// (1 for plain keys, 2 for nested "parent:prop" keys).
const shortcutMaxKeyParts = 2

// validateShortcut verifies that s is a well-formed search shortcut string
// (the value stored in a SEARCH_SHORTCUT or FIELD_VALUES claim). The string is
// parsed by the shortcut package into "&"-separated key=value entries, and this
// function applies the shortcut-specific rules: the key may have at most one ":"
// (and neither side of ":" may be "reverse"), the special "reverse" key is allowed
// as a whole single-segment key, and the value is a single identifier or the magic
// "self" value (reserved by the frontend to denote the containing document's own ID).
func validateShortcut(s string) errors.E {
	if s == "" {
		return errors.New("search shortcut must not be empty")
	}

	entries, errE := shortcut.Parse(s)
	if errE != nil {
		return errE
	}

	for _, entry := range entries {
		if len(entry.Key) > shortcutMaxKeyParts {
			return errors.New("search shortcut key must contain at most one ':'")
		}

		switch {
		case len(entry.Key) == 1 && entry.Key[0].Literal == shortcutReverseKey:
			// Reverse key is used as-is; only the value needs to be a valid identifier.
		case len(entry.Key) == shortcutMaxKeyParts:
			if entry.Key[0].Literal == shortcutReverseKey || entry.Key[1].Literal == shortcutReverseKey {
				return errors.New(`"reverse" is not allowed inside a nested key`)
			}
			if !entry.Key[0].IsIdentifier() {
				return errors.New("search shortcut nested key parent is not a valid identifier")
			}
			if !entry.Key[1].IsIdentifier() {
				return errors.New("search shortcut nested key prop is not a valid identifier")
			}
		default:
			if !entry.Key[0].IsIdentifier() {
				return errors.New("search shortcut key is not a valid identifier")
			}
		}

		if len(entry.Value) != 1 || (entry.Value[0].Literal != shortcutSelfValue && !entry.Value[0].IsIdentifier()) {
			return errors.New("search shortcut value is not a valid identifier")
		}
	}

	return nil
}
