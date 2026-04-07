import { Identifier } from "@tozd/identifier"

import { INSTANCE_OF, NAMING, Namespace, SUBPROPERTY_OF } from "@/core"
import { navigateToSearchResults } from "../peerdb_utils"
import { checkpoint, expect, test } from "../utils"

test.describe("PeerDB Search Filters", () => {
  test("Filter panel shows filters and applying them narrows results", async ({ context }) => {
    const page = await context.newPage()

    // navigateToSearchResults confirms there is no query and there are 80 results.
    await navigateToSearchResults(page)
    await checkpoint(page, "search-results-before-filters")

    // There are 4 default filters.
    const filters = page.locator(".pd-filterresult")
    await expect(filters).toHaveCount(4)

    // Apply the "instance of = property" filter.
    // PROPERTY is a class, not a property, so it is not exported from @/core and is resolved using Identifier.
    const propertyClassId = (await Identifier.from(Namespace, "PROPERTY")).toString()
    const instanceOfPropertyCheckbox = page.locator(`#ref\\/${INSTANCE_OF}\\/${propertyClassId}`)
    await expect(instanceOfPropertyCheckbox).toBeVisible()
    await instanceOfPropertyCheckbox.click()

    // Confirm changes of "instance of = property" filter.
    const header = page.locator(".pd-searchresultsheader")
    await expect(header).toContainText("38 results found.")
    await checkpoint(page, "search-filtered-instance-of-property")

    // Apply the "subproperty of = naming" filter to further narrow results.
    const subpropertyOfNamingCheckbox = page.locator(`#ref\\/${SUBPROPERTY_OF}\\/${NAMING}`)
    await expect(subpropertyOfNamingCheckbox).toBeVisible()
    await subpropertyOfNamingCheckbox.click()

    // Confirm changes of "subproperty of = naming" filter.
    await expect(header).toContainText("6 results found.")
    await checkpoint(page, "search-filtered-instance-of-property-subproperty-of-naming")

    // Remove "subproperty of = naming" filter and return to previous state.
    await expect(subpropertyOfNamingCheckbox).toBeVisible()
    await subpropertyOfNamingCheckbox.click()
    await expect(header).toContainText("38 results found.")
    await checkpoint(page, "search-filtered-instance-of-property")

    // Remove "instance of = property" filter and return to previous state.
    await expect(instanceOfPropertyCheckbox).toBeVisible()
    await instanceOfPropertyCheckbox.click()
    await expect(header).toContainText("80 results found.")
    await checkpoint(page, "search-results-before-filters")

    console.log("Successfully applied two filters, one after the other, then removed them returning to original state.")
  })
})
