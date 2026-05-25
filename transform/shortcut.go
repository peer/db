package transform

import (
	"slices"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
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

// validateShortcutIdentifier checks that token is either a 22-character base58
// identifier or a comma-separated list of non-empty base parts (each part is
// then hashed via [identifier.From] on the frontend).
func validateShortcutIdentifier(token string) errors.E {
	if strings.Contains(token, ",") {
		if slices.Contains(strings.Split(token, ","), "") {
			errE := errors.New("empty identifier part")
			errors.Details(errE)["token"] = token
			return errE
		}
		return nil
	}
	_, errE := identifier.MaybeString(token)
	return errE
}

// validateShortcut verifies that s is a well-formed search shortcut string
// (the value stored in a SEARCH_SHORTCUT or FIELD_VALUES claim). The string is
// split on "&" into key=value parts, the first "=" separates key from value,
// the key may contain at most one ":" (and neither side of ":" may be "reverse"),
// and each identifier token is either a comma-separated parts list or a 22-char
// base58 identifier. The value "self" is reserved by the frontend to denote
// the containing document's own ID.
func validateShortcut(s string) errors.E {
	if s == "" {
		return errors.New("search shortcut must not be empty")
	}

	for part := range strings.SplitSeq(s, "&") {
		eq := strings.IndexByte(part, '=')
		if eq <= 0 || eq == len(part)-1 {
			errE := errors.New("search shortcut part must have a non-empty key and value separated by '='")
			errors.Details(errE)["part"] = part
			return errE
		}
		key := part[:eq]
		value := part[eq+1:]

		keyParts := strings.Split(key, ":")
		if len(keyParts) > shortcutMaxKeyParts {
			errE := errors.New("search shortcut key must contain at most one ':'")
			errors.Details(errE)["key"] = key
			return errE
		}

		switch {
		case key == shortcutReverseKey:
			// Reverse key is used as-is; only the value needs to be a valid identifier.
		case len(keyParts) == shortcutMaxKeyParts:
			if keyParts[0] == shortcutReverseKey || keyParts[1] == shortcutReverseKey {
				errE := errors.New(`"reverse" is not allowed inside a nested key`)
				errors.Details(errE)["key"] = key
				return errE
			}
			errE := validateShortcutIdentifier(keyParts[0])
			if errE != nil {
				return errors.WithMessage(errE, "search shortcut nested key parent is not a valid identifier")
			}
			errE = validateShortcutIdentifier(keyParts[1])
			if errE != nil {
				return errors.WithMessage(errE, "search shortcut nested key prop is not a valid identifier")
			}
		default:
			errE := validateShortcutIdentifier(key)
			if errE != nil {
				return errors.WithMessage(errE, "search shortcut key is not a valid identifier")
			}
		}

		if value == shortcutSelfValue {
			continue
		}
		errE := validateShortcutIdentifier(value)
		if errE != nil {
			return errors.WithMessage(errE, "search shortcut value is not a valid identifier")
		}
	}

	return nil
}
