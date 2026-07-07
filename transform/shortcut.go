package transform

import (
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/shortcut"
)

// shortcutMaxKeyParts caps the number of ":"-separated parts a key may have
// (1 for plain keys, 2 for nested "parent:prop" keys).
const shortcutMaxKeyParts = 2

// shortcutMaxValueParts caps the number of ":"-separated parts a value may have
// (1 for a plain target, "self", or "missing"; 2 for a "direct:<identifier>" pair).
const shortcutMaxValueParts = 2

// validateShortcut verifies that s is a well-formed search shortcut string
// (the value stored in a SEARCH_SHORTCUT or FIELD_VALUES claim). The string is
// parsed by the shortcut package into "&"-separated key=value entries, and this
// function applies the shortcut-specific rules: the key may have at most one ":"
// (and neither side of ":" may be "reverse" or "id"), the special "reverse" and "id"
// keys are allowed as whole single-segment keys, and the value is validated by
// validateShortcutValue.
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

		// identifierOnly reports that the entry's value must be a single identifier (or the magic
		// "self" value): true for the "reverse" and "id" keys, whose values are documents rather
		// than filter selections (so "missing" and "direct:" do not apply). isID additionally allows
		// the "languages" value, which the id key expands to the enabled languages.
		identifierOnly := false
		isID := false
		switch {
		case len(entry.Key) == 1 && (entry.Key[0].Literal == shortcut.ReverseKey || entry.Key[0].Literal == shortcut.IDKey):
			// Reverse and id keys are used as-is; only the value needs to be a valid identifier.
			identifierOnly = true
			isID = entry.Key[0].Literal == shortcut.IDKey
		case len(entry.Key) == shortcutMaxKeyParts:
			if entry.Key[0].Literal == shortcut.ReverseKey || entry.Key[1].Literal == shortcut.ReverseKey {
				return errors.New(`"reverse" is not allowed inside a nested key`)
			}
			if entry.Key[0].Literal == shortcut.IDKey || entry.Key[1].Literal == shortcut.IDKey {
				return errors.New(`"id" is not allowed inside a nested key`)
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

		errE := validateShortcutValue(entry.Value, identifierOnly, isID)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// validateShortcutValue validates the value side of a search shortcut entry. A reverse or id entry's value
// must be a single identifier or the magic "self" value (and an id entry may also use "languages", which
// expands to the enabled languages). A property entry's value may additionally be the magic "missing" value
// (selecting the missing bucket), or a "direct:<identifier>" pair (selecting the target as a most-specific
// match), where the identifier may itself be "self".
func validateShortcutValue(value []shortcut.Segment, identifierOnly, isID bool) errors.E {
	switch len(value) {
	case 1:
		segment := value[0]
		if segment.IsIdentifier() || segment.Literal == shortcut.SelfValue {
			return nil
		}
		if isID && segment.Literal == shortcut.LanguagesValue {
			return nil
		}
		if !identifierOnly && segment.Literal == shortcut.MissingValue {
			return nil
		}
		return errors.New("search shortcut value is not a valid identifier")
	case shortcutMaxValueParts:
		if identifierOnly {
			return errors.New("search shortcut value must be a single identifier")
		}
		if value[0].Literal != shortcut.DirectValue {
			return errors.New(`search shortcut multi-segment value must start with "direct"`)
		}
		if !value[1].IsIdentifier() && value[1].Literal != shortcut.SelfValue {
			return errors.New("search shortcut direct value is not a valid identifier")
		}
		return nil
	default:
		return errors.New(`search shortcut value must contain at most one ":"`)
	}
}
