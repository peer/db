import { Identifier } from "@tozd/identifier"

import { INSTANCE_OF, Namespace, NAMING, SUBPROPERTY_OF } from "@/core"
import { searchWithQuery } from "../peerdb_utils"
import { checkpoint, expect, test } from "../utils"

test.describe("PeerDB Search Filters", () => {
  test("Filter panel shows filters and applying them narrows results", async ({ context }) => {
    const page = await context.newPage()

    await searchWithQuery(page, "")

    // Apply the "instance of = property" filter.
    // PROPERTY is a class, not a property, so it is not exported from @/core and is resolved using Identifier.
    const propertyClassId = (await Identifier.from(Namespace, "PROPERTY")).toString()
    const instanceOfPropertyCheckbox = page.locator(`#ref\\/${INSTANCE_OF}\\/${propertyClassId}`)
    await expect(instanceOfPropertyCheckbox).toBeVisible()
    await instanceOfPropertyCheckbox.click()
    await checkpoint(page, "search-filtered-instance-of-property")

    // Apply the "subproperty of = naming" filter to further narrow results.
    const subpropertyOfNamingCheckbox = page.locator(`#ref\\/${SUBPROPERTY_OF}\\/${NAMING}`)
    await expect(subpropertyOfNamingCheckbox).toBeVisible()
    await subpropertyOfNamingCheckbox.click()
    await checkpoint(page, "search-filtered-instance-of-property-subproperty-of-naming")

    // Remove "subproperty of = naming" filter and return to previous state.
    await expect(subpropertyOfNamingCheckbox).toBeVisible()
    await subpropertyOfNamingCheckbox.click()
    await checkpoint(page, "search-filtered-instance-of-property")

    // Remove "instance of = property" filter and return to previous state.
    await expect(instanceOfPropertyCheckbox).toBeVisible()
    await instanceOfPropertyCheckbox.click()
    await checkpoint(page, "search-default-results")

    console.log("Successfully applied two filters, one after the other, then removed them returning to original state.")
  })
})
