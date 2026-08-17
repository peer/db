import type { Page } from "@playwright/test"

import type { EntityClass } from "../peerdb_utils"

import { CLASS_IDS, openClassSearch, PROPERTY_IDS, searchByClass } from "../peerdb_utils"
import { checkpointElement, expect, expectResults, filter, loadAllResults, openFilters, PEERDB_URL, resultCount, resultIds, test } from "../utils"

// The site is configured to index the ancestors of a property as well as the property itself
// (indexAncestorProperties in config.yml), so a claim of a subproperty is found by a search for the
// property above it. The test data declares three such trees on purpose (properties.go in internal/xeno):
//
//   - RELATED_LOCATION gathers every way an entry is tied to a place: where somebody was born, where an
//     artifact was found, where an observation was made, where an expedition went.
//   - CLASSIFIED_AS gathers every controlled vocabulary an entry is filed under.
//   - PERIOD gathers every stretch of time an entry spans.
//
// A search narrowed by the property at the top of a tree therefore has to find the documents which carry
// only one of the properties below it, and never fewer of them than a search for that one property alone.
// Each tree is exercised over one class, the one whose documents carry the property below the top of it.
const TREES = [
  { what: "the related place property", ancestor: "RELATED_LOCATION", descendant: "BIRTHPLACE", entityClass: "INDIVIDUAL" },
  { what: "the classification property", ancestor: "CLASSIFIED_AS", descendant: "HAS_SITE_TYPE", entityClass: "SITE" },
  { what: "the classification property", ancestor: "CLASSIFIED_AS", descendant: "HAS_NARRATIVE_GENRE", entityClass: "NARRATIVE" },
] as const

// The identifiers of the values one reference facet offers, read out of the id each checkbox carries, which
// is the kind, the property path and the value joined by slashes. The special rows (a missing value, and the
// row which counts only the documents pointing directly at a value rather than at anything below it) are
// left out: they stand for something other than a document to filter by.
async function facetValues(page: Page, propertyId: string): Promise<Array<string>> {
  const ids = await filter(page, "ref", propertyId)
    .locator(".pd-reffiltertreerow-checkbox")
    .evaluateAll((boxes) => boxes.map((box) => box.id))
  return ids.map((id) => id.split("/").pop() ?? "").filter((value) => value !== "" && !value.startsWith("__"))
}

// The documents a search narrowed to one value of one property finds, as a set, through the search shortcut
// route so that each search is reached in one navigation. The search is narrowed to one class as well, which
// is what keeps the two sets small enough to load whole: the feed shows only its first page, and a set which
// is compared against another has to be the whole one.
async function foundBy(page: Page, entityClass: EntityClass, propertyId: string, valueId: string): Promise<Array<string>> {
  await page.goto(`${PEERDB_URL}/s?${PROPERTY_IDS.INSTANCE_OF}=${CLASS_IDS[entityClass]}&${propertyId}=${valueId}`)
  await expectResults(page)
  await loadAllResults(page)
  return await resultIds(page)
}

test.describe("PeerDB Property Hierarchy Flows", () => {
  for (const tree of TREES) {
    test(`Test a search by ${tree.what} finds what ${tree.descendant} carries`, async ({ context }) => {
      const page = await context.newPage()

      // A value is taken out of the facet of the property below the one at the top of the tree, and the same
      // value is then searched for twice: once through the property which records it, and once through the
      // property above it. The facets themselves cannot be compared, because a facet lists only its first
      // values and two facets over different properties rank them differently. Both searches are narrowed to
      // the class which carries the property, which keeps them small enough to load whole.
      await openClassSearch(page, tree.entityClass)
      const offered = await facetValues(page, PROPERTY_IDS[tree.descendant])
      expect(offered.length, `the ${tree.descendant} facet offers values`).toBeGreaterThan(0)
      const value = offered[0]

      const byDescendant = await foundBy(page, tree.entityClass, PROPERTY_IDS[tree.descendant], value)
      const byAncestor = await foundBy(page, tree.entityClass, PROPERTY_IDS[tree.ancestor], value)
      expect(byDescendant.length, `the ${tree.descendant} search finds documents`).toBeGreaterThan(0)
      for (const id of byDescendant) {
        expect(byAncestor, `a document found by its ${tree.descendant} is also found by the property above it`).toContain(id)
      }

      console.log(
        `Successfully verified that ${tree.what} finds all ${byDescendant.length} documents which carry the value under ${tree.descendant}, among the ${byAncestor.length} it finds.`,
      )
    })
  }

  test("Test a place facet nests along the containment chain", async ({ context }) => {
    const page = await context.newPage()

    // Places record what contains them through a property declared under the core property for entity
    // hierarchies, which is what makes a facet over places a tree rather than a flat list: a value which
    // contains other values shows them under it. The facet is looked at over one class rather than over
    // everything, so the panel stays small enough to screenshot on its own.
    await searchByClass(page, "INDIVIDUAL")
    await openFilters(page)

    const birthplace = filter(page, "ref", PROPERTY_IDS.BIRTHPLACE)
    await expect(birthplace, "the birthplace facet").toBeVisible()
    const nested = birthplace.locator(".pd-reffiltertreerow-list")
    const nestedCount = await nested.count()
    expect(nestedCount, "the birthplace facet nests its values").toBeGreaterThan(0)

    // A nested row is a value which has values under it, so the values below it are rows of their own inside
    // its own list rather than rows of the facet. What the rows below a nested one count, and the row a facet
    // offers for counting only what points at a value directly, are asserted in the tests of the special
    // rows of a facet.
    await expect(nested.first().locator(".pd-reffiltertreerow-checkbox").first(), "a nested list holds values of its own").toBeVisible()

    await checkpointElement(page, birthplace, "hierarchy-nested-place-facet")

    console.log(`Successfully verified that a place facet nests, with ${nestedCount} nested lists.`)
  })

  test("Test a time facet is offered for the property gathering the periods below it", async ({ context }) => {
    const page = await context.newPage()

    // The period tree is the same idea over time values: a survey period, an active period and an occupation
    // period are all recorded under properties below one, so a facet for that one is offered over documents
    // which carry none of them directly.
    await searchByClass(page, "SITE")
    await openFilters(page)

    const period = filter(page, "time", PROPERTY_IDS.PERIOD)
    await expect(period, "the period facet").toBeVisible()
    await expect(period.locator(".pd-timefiltersresult-row-histogram"), "the period facet draws its values").toBeVisible()

    const found = await resultCount(page)
    expect(found, "the search finds documents to filter").toBeGreaterThan(0)
    await checkpointElement(page, period, "hierarchy-time-period-facet")

    console.log(`Successfully verified that the period facet is offered over ${found} documents.`)
  })
})
