import { Identifier } from "@tozd/identifier"

import { INSTANCE_OF, Namespace, SHORT_NAME, VARIANT } from "@/core"
import { testDocumentPage, testDocumentPageDirect } from "../peerdb_utils"
import { test } from "../utils"

const PROPERTY_CLASS = (await Identifier.from(Namespace, "PROPERTY")).toString()
const LITRE_UNIT = (await Identifier.from(Namespace, "UNIT", "l")).toString()
const SLOVENIAN_LANGUAGE = (await Identifier.from(Namespace, "LANGUAGE", "sl-SI")).toString()

const DOCUMENTS = [
  { id: SHORT_NAME, title: "short name" },
  { id: INSTANCE_OF, title: "instance of" },
  { id: VARIANT, title: "variant" },
  { id: PROPERTY_CLASS, title: "property" },
  { id: LITRE_UNIT, title: "litre" },
  { id: SLOVENIAN_LANGUAGE, title: "Slovenian" },
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
