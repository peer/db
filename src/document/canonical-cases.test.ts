// @vitest-environment jsdom
// The shared claim-validation corpus, run against the frontend editor schema. The same file is
// run by the Go backend (document/canonical_cases_test.go), so this asserts that the frontend and
// the backend agree on HTML claim validation: the canonical form and validity of every input.
// The corpus is generated from the backend, which equals the TypeScript reference per the
// go-prosemirror conformance fixtures.

import { describe, expect, test } from "vitest"

// The corpus lives next to the Go backend test that also runs it; Vite resolves the JSON import
// from the project root.
import corpus from "@/../document/testdata/canonical-cases.json"

import { docToHtml, htmlToDoc, isCanonicalHTML } from "@/partials/input/InputHTML.schema"

interface CanonicalCase {
  name: string
  input: string
  canonical: string
  valid: boolean
  recanonical?: string
}

const cases: CanonicalCase[] = corpus.cases

// canonicalize mirrors the backend CanonicalizeHTML: parse the HTML into the editor schema and
// serialize it back.
function canonicalize(html: string): string {
  return docToHtml(htmlToDoc(html))
}

describe("shared canonical-cases corpus (matches the Go backend)", () => {
  test("corpus is not empty", () => {
    expect(cases.length).toBeGreaterThan(0)
  })

  for (const c of cases) {
    test(c.name, () => {
      expect(canonicalize(c.input)).toBe(c.canonical)
      expect(isCanonicalHTML(c.input)).toBe(c.valid)
      expect(c.input === c.canonical).toBe(c.valid)
      const expectedRe = c.recanonical ?? c.canonical
      expect(canonicalize(c.canonical)).toBe(expectedRe)
    })
  }
})
