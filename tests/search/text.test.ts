import type { Page } from "@playwright/test"

import { documentIdOf, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import { expect, loadAllResults, resultCount, resultIds, searchByQuery, test } from "../utils"

// The values which are deliberately kept out of the full text index. The test data marks the catalogue
// codes and the researcher codes as not text searchable (notTextSearchable in internal/xeno/properties.go),
// because they are values one filters by rather than words one searches for, and letting them into the text
// index would make every query which happens to look like a code match them.
const EXCLUDED_VALUES = [
  { property: "CATALOGUE_CODE", value: "MW-1104-KEP", document: ["STAR_SYSTEM", "G1_KEPHRA"], name: "Kephra" },
  { property: "RESEARCHER_CODE", value: "CCX-P-01023", document: ["RESEARCHER", "RES_HALVORSEN"], name: "Halvorsen" },
] as const

// A word of the catalogue and the same word in the plural. The index stems what it holds and what it is
// asked for, so the two are the same query as far as it is concerned.
const STEMMED = { singular: "practice", plural: "practices" }

// The name a class is also known by, which the schema declares next to the name itself
// (classAlternativeNames in internal/xeno/classes.go). A search for it has to find the documents of that
// class, which is what an alternative name is for: the visitor does not have to know the word the
// catalogue chose.
const ALTERNATIVE_CLASS_NAME = { word: "settlement", entityClass: "SITE" } as const

// A word of one of the other two languages the schema is written in. The schema documents carry their
// labels in all three, so a query in any of them finds them.
const SLOVENIAN_WORD = { word: "vsebovan", property: "CONTAINED_IN" } as const

// Runs a text query and returns the whole result set, so two queries can be compared as sets rather than by
// the ranking, which rests on term statistics and is not stable enough to compare.
async function search(page: Page, query: string): Promise<Array<string>> {
  await searchByQuery(page, query)
  await loadAllResults(page)
  return await resultIds(page)
}

test.describe("PeerDB Text Search Flows", () => {
  for (const excluded of EXCLUDED_VALUES) {
    test(`Test a ${excluded.property} value is not matched by a text query`, async ({ context }) => {
      const page = await context.newPage()

      // The document states the value, and the value is what the catalogue lists it under, but the property
      // is kept out of the text index, so searching for it finds nothing at all.
      await searchByQuery(page, excluded.value, { results: false })
      expect(await resultCount(page), `a query for the ${excluded.property} value`).toBe(0)

      // The same document is found by its name, which is what says the query was excluded rather than the
      // document being absent or the query being malformed.
      const id = await documentIdOf(...excluded.document)
      const byName = await search(page, excluded.name)
      expect(byName, `the document is found by its name instead`).toContain(id)

      // And the value is still on the document, so it was excluded from the text index rather than dropped.
      const stated = await page.evaluate(async (documentId) => JSON.stringify(await fetch(`/api/d/${documentId}`).then((r) => r.json())), id)
      expect(stated, `the ${excluded.property} value is still stated on the document`).toContain(excluded.value)

      console.log(`Successfully verified that the ${excluded.property} value ${excluded.value} is not matched by a text query.`)
    })
  }

  test("Test a text query matches the stem of a word rather than the word", async ({ context }) => {
    const page = await context.newPage()

    // What the index holds and what it is asked for are both reduced to their stems, so a plural finds what
    // the singular finds.
    const singular = await search(page, STEMMED.singular)
    const plural = await search(page, STEMMED.plural)

    expect(singular.length, `the query for "${STEMMED.singular}" finds documents`).toBeGreaterThan(0)
    expect([...plural].sort(), `the query for "${STEMMED.plural}" finds the same documents`).toEqual([...singular].sort())

    console.log(`Successfully verified that "${STEMMED.singular}" and "${STEMMED.plural}" find the same ${singular.length} documents.`)
  })

  test("Test a class is found by the name it is also known by", async ({ context }) => {
    const page = await context.newPage()

    // The class declares an alternative name next to its own, and the documents of the class are indexed
    // under it, so a visitor who knows only the other word still finds them.
    const found = await search(page, ALTERNATIVE_CLASS_NAME.word)
    expect(found.length, `the query for "${ALTERNATIVE_CLASS_NAME.word}" finds documents`).toBeGreaterThan(0)

    await searchByClass(page, ALTERNATIVE_CLASS_NAME.entityClass)
    await loadAllResults(page)
    const ofClass = await resultIds(page)
    expect(ofClass.length, `the class has documents`).toBeGreaterThan(0)

    const both = ofClass.filter((id) => found.includes(id))
    expect(both.length, `the documents of the class are found by the name it is also known by`).toBe(ofClass.length)

    console.log(`Successfully verified that all ${ofClass.length} documents of the class are found by "${ALTERNATIVE_CLASS_NAME.word}".`)
  })

  test("Test a query in another language of the site finds what it names", async ({ context }) => {
    const page = await context.newPage()

    // Every label of the schema is written in each of the three languages the site is served in, and all of
    // them are indexed, so a query in one of them finds the document whatever language the interface is in.
    const found = await search(page, SLOVENIAN_WORD.word)
    expect(found, `the Slovenian label finds the property it names`).toContain(PROPERTY_IDS[SLOVENIAN_WORD.property])

    console.log(`Successfully verified that the Slovenian query "${SLOVENIAN_WORD.word}" finds the property it names.`)
  })
})
