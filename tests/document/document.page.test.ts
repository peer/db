import { Identifier } from "@tozd/identifier"

import { INSTANCE_OF, Namespace, SHORT_NAME, VARIANT } from "@/core"
import { testDocumentPage } from "../peerdb_utils"
import { test } from "../utils"

const PROPERTY_CLASS = (await Identifier.from(Namespace, "PROPERTY")).toString()
const LITRE_UNIT = (await Identifier.from(Namespace, "UNIT", "l")).toString()
const SLOVENIAN_LANGUAGE = (await Identifier.from(Namespace, "LANGUAGE", "sl-SI")).toString()

const DOCUMENTS = [
  { id: SHORT_NAME, title: "short name", checkpoint: "property-short-name" },
  { id: INSTANCE_OF, title: "instance of", checkpoint: "property-instance-of" },
  { id: VARIANT, title: "variant", checkpoint: "property-variant" },
  { id: PROPERTY_CLASS, title: "property", checkpoint: "property" },
  { id: LITRE_UNIT, title: "litre", checkpoint: "litre-unit" },
  { id: SLOVENIAN_LANGUAGE, title: "Slovenian", checkpoint: "slovenian-language" },
]

test.describe("PeerDB Core Documents", () => {
  for (const doc of DOCUMENTS) {
    test(`Successful ${doc.title} page`, async ({ context }) => {
      const page = await context.newPage()
      await testDocumentPage(page, doc.id, doc.title, doc.checkpoint)
      console.log(`Successfully loaded "${doc.title}" document page.`)
    })
  }
})
