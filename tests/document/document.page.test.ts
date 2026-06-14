import { Identifier } from "@tozd/identifier"

import { CARDINALITY, CLASS, DESCRIPTION, INSTANCE_OF, Namespace } from "@/core"
import { testDocumentPage, testDocumentPageDirect } from "../peerdb_utils"
import { test } from "../utils"

const LITRE_UNIT = (await Identifier.from(Namespace, "UNIT", "l")).toString()
const ENGLISH_LANGUAGE = (await Identifier.from(Namespace, "LANGUAGE", "en-GB")).toString()

// testDocumentPage navigates via an empty search, whose results are ordered by display label and
// rendered only up to the initial page limit (50). These documents must therefore have labels that
// sort within the first page, so pick ones early in the alphabet while keeping a mix of types
// (property, class, unit, language).
const DOCUMENTS = [
  { id: CARDINALITY, title: "cardinality" },
  { id: CLASS, title: "class" },
  { id: DESCRIPTION, title: "description" },
  { id: ENGLISH_LANGUAGE, title: "English" },
  { id: INSTANCE_OF, title: "instance of" },
  { id: LITRE_UNIT, title: "litre" },
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
      console.log(`Successfully opened "${doc.title}" directly via URL, verified search bar (no prev/next), searched, and confirmed 80 results.`)
    })
  }
})
