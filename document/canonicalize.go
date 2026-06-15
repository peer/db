package document

import (
	_ "embed"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/prosemirror/model"
)

// SchemaJSON is the editor schema in the shared ProseMirror schema dialect. It is the single
// source of truth for which HTML the editor and the backend accept: the backend compiles it
// into htmlSchema below, and the frontend fetches the same bytes from /schema.json and builds
// its editor schema from them, so the two cannot drift. The shared canonical-cases corpus, run
// by both the Go and TypeScript sides, enforces that the frontend and the backend agree.
//
//go:embed schema.json
var SchemaJSON []byte

// htmlSchema is the compiled editor schema used to parse and canonically serialize HTML on the
// backend. Parsing into this schema sanitizes by construction: only schema content survives and
// URL attribute values are validated by the named validators below.
//
//nolint:gochecknoglobals
var htmlSchema = mustHTMLSchema()

func mustHTMLSchema() *model.Schema {
	schema, errE := model.NewSchema(SchemaJSON, model.SchemaCallbacks{
		Validators: map[string]model.AttrValidator{
			"linkURL":     urlAttrValidator(true),
			"resourceURL": urlAttrValidator(false),
		},
	})
	if errE != nil {
		panic(errE)
	}
	return schema
}

// urlAttrValidator adapts validateURL to the schema attribute validator signature: it requires a
// string value and validates it with validateURL, allowing the contact schemes (mailto and tel) only
// when allowContact is set. The schema's linkURL (<a href>) uses allowContact true; resourceURL
// (<blockquote cite>) uses false. So link attribute values go through the same URL validation as
// LinkClaim IRIs.
func urlAttrValidator(allowContact bool) model.AttrValidator {
	return func(value any) errors.E {
		s, ok := value.(string)
		if !ok {
			return errors.New("URL value is not a string")
		}
		return validateURL(s, allowContact)
	}
}

// canonicalEmptyHTML is the canonical serialization of an empty document (an editor document
// with no content). Parsing empty, whitespace-only, or all-disallowed input yields this, so it
// serves as the sentinel for "this HTML carries no content" (see IsEmptyHTML).
//
//nolint:gochecknoglobals
var canonicalEmptyHTML = mustCanonicalEmptyHTML()

func mustCanonicalEmptyHTML() string {
	canonical, errE := CanonicalizeHTML("")
	if errE != nil {
		panic(errE)
	}
	return canonical
}

// htmlParseOptions are the parse options used for canonicalizing HTML claim values. We preserve
// whitespace (PreserveWhitespaceTrue): runs of spaces are kept (the editor stores HTML faithfully,
// so a user's spacing survives a round trip and stays canonical), while newlines are normalized to
// spaces (no linebreak replacement node is set). The frontend's htmlToDoc passes the matching
// preserveWhitespace option, and the editor's paste path uses the collapsing default so formatting
// whitespace from imported HTML is not pulled in as content.
//
// With these options canonicalization is always idempotent for this schema (one pass reaches a
// fixed point), which is what makes IsCanonicalHTML well-defined (canonical iff input equals its
// canonicalization). Preserved spaces and pre content round-trip unchanged, and the only
// whitespace transform, newline-to-space, leaves no convertible newlines for a second pass to
// change.
//
//nolint:gochecknoglobals,exhaustruct
var htmlParseOptions = model.ParseOptions{PreserveWhitespace: model.PreserveWhitespaceTrue}

// CanonicalizeHTML parses the input HTML into an editor document and serializes that document
// back to its canonical HTML form, sanitizing the input in the process. Canonical HTML is the
// fixed point of this function.
func CanonicalizeHTML(input string) (string, errors.E) {
	return model.CanonicalizeHTML(htmlSchema, input, htmlParseOptions)
}

// IsCanonicalHTML reports whether the input HTML is already in canonical form: parsing it into
// the editor schema and serializing it back is the identity. This is the claim-validity check
// for HTML claim values, matching the frontend's isCanonicalHTML.
func IsCanonicalHTML(input string) (bool, errors.E) {
	return model.IsCanonicalHTML(htmlSchema, input, htmlParseOptions)
}

// IsEmptyHTML reports whether canonical HTML (as returned by CanonicalizeHTML) carries no
// content. Empty, whitespace-only, and all-disallowed input all canonicalize to the same empty
// document, so callers that convert a field into an HTML claim use this to decide that no claim
// should be made.
func IsEmptyHTML(canonical string) bool {
	return canonical == canonicalEmptyHTML
}

// ParseHTML parses HTML into an editor document using the editor schema (and the same parse options
// as CanonicalizeHTML). It lets callers walk the structured document, for example to extract its
// text with Node.TextBetween, instead of re-tokenizing the HTML string. Disallowed content is
// dropped by the schema, so the result is sanitized.
func ParseHTML(input string) (*model.Node, errors.E) {
	return model.ParseHTML(htmlSchema, input, htmlParseOptions)
}
