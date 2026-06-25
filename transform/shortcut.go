package transform

import (
	"gitlab.com/tozd/go/errors"

	internalShortcut "gitlab.com/peerdb/peerdb/internal/shortcut"
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
// (and neither side of ":" may be "reverse"), the special "reverse" key is allowed
// as a whole single-segment key, and the value is validated by validateShortcutValue.
func validateShortcut(s string) errors.E {
	if s == "" {
		return errors.New("search shortcut must not be empty")
	}

	entries, errE := internalShortcut.Parse(s)
	if errE != nil {
		return errE
	}

	for _, entry := range entries {
		if len(entry.Key) > shortcutMaxKeyParts {
			return errors.New("search shortcut key must contain at most one ':'")
		}

		reverse := false
		switch {
		case len(entry.Key) == 1 && entry.Key[0].Literal == internalShortcut.ReverseKey:
			// Reverse key is used as-is; only the value needs to be a valid identifier.
			reverse = true
		case len(entry.Key) == shortcutMaxKeyParts:
			if entry.Key[0].Literal == internalShortcut.ReverseKey || entry.Key[1].Literal == internalShortcut.ReverseKey {
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

		errE := validateShortcutValue(entry.Value, reverse)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// validateShortcutValue validates the value side of a search shortcut entry. A reverse entry's value must
// be a single identifier or the magic "self" value. A property entry's value may additionally be the magic
// "missing" value (selecting the missing bucket), or a "direct:<identifier>" pair (selecting the target as
// a most-specific match), where the identifier may itself be "self".
func validateShortcutValue(value []internalShortcut.Segment, reverse bool) errors.E {
	switch len(value) {
	case 1:
		segment := value[0]
		if segment.IsIdentifier() || segment.Literal == internalShortcut.SelfValue {
			return nil
		}
		if !reverse && segment.Literal == internalShortcut.MissingValue {
			return nil
		}
		return errors.New("search shortcut value is not a valid identifier")
	case shortcutMaxValueParts:
		if reverse {
			return errors.New("search shortcut reverse value is not a valid identifier")
		}
		if value[0].Literal != internalShortcut.DirectValue {
			return errors.New(`search shortcut multi-segment value must start with "direct"`)
		}
		if !value[1].IsIdentifier() && value[1].Literal != internalShortcut.SelfValue {
			return errors.New("search shortcut direct value is not a valid identifier")
		}
		return nil
	default:
		return errors.New(`search shortcut value must contain at most one ":"`)
	}
}
