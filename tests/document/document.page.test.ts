import { Identifier } from "@tozd/identifier"

import { CARDINALITY, CLASS, DESCRIPTION, Namespace, VT_AMOUNT } from "@/core"
import { testDocumentPage, testDocumentPageDirect } from "../peerdb_utils"
import { test } from "../utils"

const BYTE_UNIT = (await Identifier.from(Namespace, "UNIT", "B")).toString()

// testDocumentPage navigates via an empty search, whose results are ordered by display label. The feed
// renders only an initial page and lazy-loads more as the page scrolls, so these documents must sort
// within that initially rendered page to be clickable without scrolling. Pick ones early in the alphabet
// while keeping a mix of types (property, class, value type, unit, language).
const DOCUMENTS = [
  { id: VT_AMOUNT, title: "amount" },
  { id: BYTE_UNIT, title: "byte" },
  { id: CARDINALITY, title: "cardinality" },
  { id: CLASS, title: "class" },
  { id: DESCRIPTION, title: "description" },
]

test.describe("PeerDB Core Documents", () => {
  // Navigating to documents via search shows them with prev/next buttons in NavBar.
  for (const doc of DOCUMENTS) {
    test(`Successful ${doc.title} page`, async ({ context }) => {
      const page = await context.newPage()
      await testDocumentPage(page, doc.id, doc.title)
      console.log(`Successfully loaded "${doc.title}" document page.`)
    })
  }

  // Direct navigation to documents shows them with search button in NavBar.
  for (const doc of DOCUMENTS) {
    test(`Successful ${doc.title} page direct URL`, async ({ context }) => {
      const page = await context.newPage()
      await testDocumentPageDirect(page, doc.id, doc.title)
      console.log(`Successfully opened "${doc.title}" directly via URL, verified search bar (no prev/next), searched, and confirmed results.`)
    })
  }
})
