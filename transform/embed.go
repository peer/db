package transform

import (
	"reflect"
	"strings"

	"gitlab.com/tozd/go/errors"

	internalCore "gitlab.com/peerdb/peerdb/internal/core"
	"gitlab.com/peerdb/peerdb/internal/shortcut"
)

// validateEmbedEntry verifies that entry is a well-formed embed entry "destination=source".
//
// The destination (left of "=") is a single identifier token naming the property under which the
// embedded claims are attached on the embedding document. The source (right of "=") is a
// ":"-separated path of identifier tokens navigating the referenced document's claims: the first
// token selects that document's claims with that property, and each further token selects sub-claims
// with that property under the previous match. Each token is either a comma-separated parts list or a
// 22-character base58 identifier, the same form a search shortcut uses. The destination does not
// support a path, embedding always lands directly under a single property.
func validateEmbedEntry(entry string) errors.E {
	parsed, errE := shortcut.ParseEntry(entry)
	if errE != nil {
		return errE
	}

	if len(parsed.Key) != 1 {
		errE := errors.New("embed entry destination must be a single segment")
		errors.Details(errE)["entry"] = entry
		return errE
	}
	if !parsed.Key[0].IsIdentifier() {
		errE := errors.New("embed entry destination is not a valid identifier")
		errors.Details(errE)["entry"] = entry
		return errE
	}

	for _, segment := range parsed.Value {
		if !segment.IsIdentifier() {
			errE := errors.New("embed entry source segment is not a valid identifier")
			errors.Details(errE)["entry"] = entry
			return errE
		}
	}

	return nil
}

// validateEmbed verifies that s is a well-formed embed tag value: one or more embed entries separated
// by "&", each a "destination=source" pair validated by validateEmbedEntry. The format is intentionally
// similar to the search shortcut format, but the left side is a single segment and the right side may be
// a ":"-separated path.
func validateEmbed(s string) errors.E {
	if s == "" {
		return errors.New("embed must not be empty")
	}
	for entry := range strings.SplitSeq(s, shortcut.EntrySeparator) {
		errE := validateEmbedEntry(entry)
		if errE != nil {
			return errE
		}
	}
	return nil
}

// parseEmbedTag parses the embed struct tag into the list of embed entries stored on a field's
// EMBED_PROPERTY setting. Entries are separated by "&"; each is validated and stored as-is, to be
// parsed and resolved to property IDs by the converter at index time. The embed tag can only be used
// with a core.Ref field type, since embedding pulls claims from the referenced document.
func parseEmbedTag(field reflect.StructField) ([]string, errors.E) {
	tag, ok := field.Tag.Lookup("embed")
	if !ok {
		return nil, nil
	}

	baseType := internalCore.UnwrapSliceAndPointer(field.Type)
	if baseType != internalCore.RefType {
		errE := errors.New("embed tag can only be used with core.Ref field type")
		errors.Details(errE)["type"] = field.Type.String()
		return nil, errE
	}

	errE := validateEmbed(tag)
	if errE != nil {
		return nil, errE
	}

	return strings.Split(tag, shortcut.EntrySeparator), nil
}
