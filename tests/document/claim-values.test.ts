import type { Locator, Page } from "@playwright/test"

import { documentIdOf, LANGUAGES, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  fieldValues,
  goHome,
  openDocument,
  openDocumentTab,
  propertyRow,
  propertyValues,
  settle,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The documents of the test data this file looks at, addressed by their document identifier so that the same
// document is opened on every run. Each was picked because it carries the claims of the type the test around it
// asserts on, which the rest of the test data does not have together on one document.
const STAR_SYSTEM_ID = await documentIdOf("STAR_SYSTEM", "G1_KEPHRA")
const SITE_ID = await documentIdOf("SITE", "G2_OLD_SEAM")
const REGION_ID = await documentIdOf("REGION", "G1_WARM_POOL")
const GALAXY_ID = await documentIdOf("GALAXY", "G1_MILKY_WAY")
const RINGED_PLANET_ID = await documentIdOf("PLANET", "G1_SIXTEEN_HUNDRED_VIII")
const BARE_PLANET_ID = await documentIdOf("PLANET", "G1_LOW_ORDER")
const MOON_ID = await documentIdOf("MOON", "G1_UNDERCOUNT")
const PUBLICATION_ID = await documentIdOf("PUBLICATION", "PUB_COMPARATIVE_FERMENT")
const INSTITUTE_ID = await documentIdOf("INSTITUTE", "INST_ANCHOR")

// The time claims of the test data, one per precision the data records, each with the document carrying it and
// what the time display has to make of it. A timestamp is written out to the precision the claim records and the
// parts which the precision does not reach are rendered as zeros and marked as imprecise, so both the whole text
// and the imprecise tail of it are what says a precision came through.
const TIME_PRECISIONS = [
  { what: "a year", precision: "y", id: STAR_SYSTEM_ID, property: PROPERTY_IDS.FIRST_SURVEYED, text: /^\d{4}$/, imprecise: "" },
  { what: "a month", precision: "m", id: PUBLICATION_ID, property: PROPERTY_IDS.PUBLISHED_ON, text: /^\d{4}-\d{2}-00$/, imprecise: "-00" },
  {
    what: "a day",
    precision: "d",
    id: await documentIdOf("OBSERVATION", "OBSA_CORD_READING_RECORDER"),
    property: PROPERTY_IDS.OBSERVED_ON,
    text: /^\d{4}-\d{2}-\d{2}$/,
    imprecise: "",
  },
  { what: "a decade", precision: "10y", id: await documentIdOf("ARTIFACT", "G2_RIB_LATTICE"), property: PROPERTY_IDS.DATE_MADE, text: /^\d{4}$/, imprecise: "0" },
  { what: "a century", precision: "100y", id: await documentIdOf("ARTIFACT", "G1_NAME_STONE"), property: PROPERTY_IDS.DATE_MADE, text: /^\d{4}$/, imprecise: "00" },
] as const

// Opens a document and switches to its "all properties" tab, which lists every claim of the document,
// sub-claims included, each rendered by its own type. It is the only view which shows claims the document's
// class declares no field for and which does not narrow the claims to the current language, so it is where
// every claim type can be found.
//
// The whole page is compared against a screenshot under the given name. A document which a test opens only to
// look at one more claim of a type another document already covered is opened with a null name instead, so the
// suite does not carry a second screenshot of a page it already has one of.
async function openAllProperties(page: Page, id: string, name: string | null): Promise<void> {
  await openDocument(page, id)
  await settle(page)
  await openDocumentTab(page, "allproperties")
  await settle(page)
  await expect(page.locator(".pd-documentget-panel-allproperties .pd-propertiesview-row").first(), `rows of the all properties tab of ${name ?? id}`).toBeVisible()
  if (name !== null) {
    await checkpoint(page, name, { mask: volatile(page) })
  }
}

// The row of the "all properties" tab which holds the sub-claims of the claims of one property. Sub-claims
// render as a table of their own in a row below the claim they hang off, and that row carries the property of
// the claim they hang off in its class name. Defined here rather than in the shared helpers because this file
// is the only one which reaches into a claim's sub-claims by the property of its parent.
function subClaimRow(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-documentget-panel-allproperties .pd-propertiesview-row-sub-${propertyId}`)
}

// The table of the sub-claims of the claims of one property. A row of the properties table is laid out as
// contents rather than as a box of its own, so the table inside it is what a screenshot of the sub-claims can
// be clipped to.
function subClaimTable(page: Page, propertyId: string): Locator {
  return subClaimRow(page, propertyId).locator(".pd-propertiesview").first()
}

// The value cells of the sub-claims of one property which are themselves for another property, which is how the
// unit of an amount, the language of a text and the caption of a file are addressed.
function subClaimValues(page: Page, parentPropertyId: string, propertyId: string): Locator {
  return subClaimRow(page, parentPropertyId).locator(`.pd-propertiesview-row-${propertyId} .pd-propertiesview-value`)
}

test.describe("PeerDB Document Claim Values Flows", () => {
  test("Test an identifier claim value which its property gives a link template", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, STAR_SYSTEM_ID, "claim-values-starsystem-allproperties")

    // An identifier claim is shown as a link when the property it is for declares a link template, and the
    // identifier is put into the template, so the link has to point at the entry of the external register which
    // the identifier names and never at the template itself.
    const catalogueValues = propertyValues(page, PROPERTY_IDS.CATALOGUE_CODE)
    await expect(catalogueValues, "catalogue code claims").toHaveCount(1)
    const catalogueLink = catalogueValues.locator(".pd-claimvalueid")
    await expect(catalogueLink, "the catalogue code").toBeVisible()
    await expect(catalogueLink, "the catalogue code is rendered as a link").toHaveJSProperty("tagName", "A")
    const code = (await catalogueLink.textContent())!.trim()
    expect(code, "the catalogue code shows its value").not.toBe("")
    await checkpointElement(page, catalogueValues, "claim-values-id-cataloguecode")

    const catalogueHref = await catalogueLink.getAttribute("href")
    expect(catalogueHref, "the catalogue code links to the register the property names").toMatch(/^https:\/\/registry\.ccx\.example\/entry\//)
    expect(catalogueHref, "the catalogue code link carries the identifier and not the template parameter").toBe(`https://registry.ccx.example/entry/${code}`)

    // The same holds for a document identifier of a different register: the template belongs to the property,
    // so every property which declares one turns its identifiers into links of its own.
    await openAllProperties(page, PUBLICATION_ID, "claim-values-publication-allproperties")
    const doiValues = propertyValues(page, PROPERTY_IDS.DOI)
    await expect(doiValues, "DOI claims").toHaveCount(1)
    const doiLink = doiValues.locator(".pd-claimvalueid")
    await expect(doiLink, "the DOI").toBeVisible()
    const doi = (await doiLink.textContent())!.trim()
    await checkpointElement(page, doiValues, "claim-values-id-doi")
    expect(await doiLink.getAttribute("href"), "the DOI links to the resolver the property names").toBe(`https://doi.example/${doi}`)

    console.log(`Successfully verified that an identifier claim links through its property's link template, for the catalogue code ${code} and the DOI ${doi}.`)
  })

  test("Test an identifier claim value whose property gives no link template", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, SITE_ID, "claim-values-site-allproperties")

    // Without a link template on its property, and without being a web address itself, an identifier has
    // nothing to link to, so it is written out as text. It is still marked as an identifier rather than being
    // left as bare text, so it can be told apart from a string claim.
    const gridValues = propertyValues(page, PROPERTY_IDS.GRID_REFERENCE)
    await expect(gridValues, "grid reference claims").toHaveCount(1)
    const gridValue = gridValues.locator(".pd-claimvalueid")
    await expect(gridValue, "the grid reference").toBeVisible()
    await expect(gridValue, "the grid reference is not empty").not.toHaveText(/^\s*$/)
    await expect(gridValue, "the grid reference is not rendered as a link").toHaveJSProperty("tagName", "SPAN")
    await expect(gridValues.locator("a"), "the grid reference holds no link at all").toHaveCount(0)
    await checkpointElement(page, gridValues, "claim-values-id-gridreference")

    console.log(`Successfully verified that an identifier claim whose property declares no link template renders as text: ${(await gridValue.textContent())!.trim()}.`)
  })

  test("Test string, HTML and reference claim values", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, STAR_SYSTEM_ID, "claim-values-starsystem-text-allproperties")

    // A string claim is written out as it was recorded, with no markup of its own.
    const spectralValues = propertyValues(page, PROPERTY_IDS.SPECTRAL_CLASS)
    await expect(spectralValues, "spectral class claims").toHaveCount(1)
    const spectralString = spectralValues.locator(".pd-claimvaluestring")
    await expect(spectralString, "the spectral class").toBeVisible()
    await expect(spectralString, "the spectral class is not empty").not.toHaveText(/^\s*$/)
    expect(await spectralString.innerHTML(), "the spectral class carries no markup").not.toContain("<")
    await checkpointElement(page, spectralValues, "claim-values-string-spectralclass")

    // A string claim which was recorded in a language carries the language as a sub-claim, which renders as a
    // reference to the language document below the value rather than next to it.
    const nameValues = propertyValues(page, PROPERTY_IDS.NAME)
    await expect(nameValues, "name claims").toHaveCount(1)
    await expect(nameValues.locator(".pd-claimvaluestring"), "the name is a string claim").toHaveCount(1)
    await expect(subClaimValues(page, PROPERTY_IDS.NAME, PROPERTY_IDS.IN_LANGUAGE), "the language the name was recorded in").toHaveCount(1)

    // An HTML claim is rendered as markup and not as escaped text, so the element it renders into has to carry
    // the elements of the claim and not their source.
    const descriptionValues = propertyValues(page, PROPERTY_IDS.DESCRIPTION)
    await expect(descriptionValues, "description claims").toHaveCount(1)
    const descriptionHtml = descriptionValues.locator(".pd-claimvaluehtml")
    await expect(descriptionHtml, "the description is rendered as HTML").toHaveCount(1)
    await expect(descriptionHtml, "the description is not empty").not.toHaveText(/^\s*$/)
    expect((await descriptionHtml.innerHTML()).trim(), "the description renders as markup").toMatch(/^<p>/)
    await checkpointElement(page, descriptionValues, "claim-values-html-description")

    // A reference claim is shown as an inline reference to the document it points at, which links to that
    // document and shows its display label rather than its identifier.
    const containedValues = propertyValues(page, PROPERTY_IDS.CONTAINED_IN)
    await expect(containedValues, "contained in claims").toHaveCount(1)
    const containedRef = containedValues.locator(".pd-claimvalueref")
    await expect(containedRef, "the containing place").toBeVisible()
    await expect(containedRef, "the containing place shows a display label").not.toHaveText(/^\s*$/)
    await expect(containedRef, "the containing place links to the document it points at").toHaveAttribute("href", /^\/d\/[0-9A-Za-z]+$/)
    await checkpointElement(page, containedValues, "claim-values-ref-containedin")

    // Every reference of the document links, whatever property it is under, so a referenced document is always
    // one click away.
    const refs = page.locator(".pd-documentget-panel-allproperties .pd-claimvalueref")
    const refCount = await refs.count()
    expect(refCount, "reference claims of the document").toBeGreaterThan(1)
    for (let i = 0; i < refCount; i++) {
      await expect(refs.nth(i), `reference claim ${i} links to the document it points at`).toHaveAttribute("href", /^\/d\/[0-9A-Za-z]+$/)
    }

    console.log(`Successfully verified how string, HTML and reference claim values render, with ${refCount} references all linking to the document they point at.`)
  })

  test("Test amount claim values with and without a unit", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, STAR_SYSTEM_ID, "claim-values-starsystem-amounts-allproperties")

    // An amount claim which counts something has no unit, so it is the number and nothing else.
    const starValues = propertyValues(page, PROPERTY_IDS.STAR_COUNT)
    await expect(starValues, "number of stars claims").toHaveCount(1)
    const starAmount = starValues.locator(".pd-claimvalueamount")
    await expect(starAmount, "the number of stars").toBeVisible()
    await expect(starAmount, "the number of stars is a number").toHaveText(/^\d+(\.\d+)?$/)
    await expect(subClaimRow(page, PROPERTY_IDS.STAR_COUNT), "an amount which counts something carries no unit").toHaveCount(0)
    await checkpointElement(page, starValues, "claim-values-amount-starcount")

    // An amount claim which measures something carries the unit it was measured in as a sub-claim, which is a
    // reference to the unit document and renders below the amount rather than beside it.
    const distanceValues = propertyValues(page, PROPERTY_IDS.DISTANCE_FROM_SOL)
    await expect(distanceValues, "distance from Sol claims").toHaveCount(1)
    const distanceAmount = distanceValues.locator(".pd-claimvalueamount")
    await expect(distanceAmount, "the distance from Sol").toBeVisible()
    await expect(distanceAmount, "the distance from Sol is a number").toHaveText(/^\d+(\.\d+)?$/)
    const unitValues = subClaimValues(page, PROPERTY_IDS.DISTANCE_FROM_SOL, PROPERTY_IDS.IN_UNIT)
    await expect(unitValues, "the unit the distance was measured in").toHaveCount(1)
    await expect(unitValues.locator(".pd-claimvalueref"), "the unit is a reference to the unit document").toHaveAttribute("href", /^\/d\/[0-9A-Za-z]+$/)
    await checkpointElement(page, distanceValues, "claim-values-amount-distance")
    await checkpointElement(page, subClaimTable(page, PROPERTY_IDS.DISTANCE_FROM_SOL), "claim-values-amount-distance-unit")

    console.log("Successfully verified how amount claim values render, both the ones which count and the ones which carry a unit.")
  })

  test("Test an amount interval claim value including a negative bound", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, REGION_ID, "claim-values-region-allproperties")

    // An amount interval claim shows its two ends separated by a dash. This region reaches from the floor of a
    // sea to a shore above it, so its lower end is below the datum and is written with its sign rather than
    // being dropped or shown as a distance.
    const elevationValues = propertyValues(page, PROPERTY_IDS.ELEVATION_RANGE)
    await expect(elevationValues, "elevation range claims").toHaveCount(1)
    const from = elevationValues.locator(".pd-claimvalueamountinterval-from")
    const to = elevationValues.locator(".pd-claimvalueamountinterval-to")
    await expect(from, "the lower end of the elevation range").toBeVisible()
    await expect(from, "the lower end of the elevation range is negative").toHaveText(/^-\d+(\.\d+)?$/)
    await expect(to, "the upper end of the elevation range").toBeVisible()
    await expect(to, "the upper end of the elevation range is a number").toHaveText(/^-?\d+(\.\d+)?$/)
    await expect(elevationValues, "the two ends of the elevation range are separated by a dash").toHaveText(/-\d+(\.\d+)?\s*\u2013\s*-?\d+/)
    // An interval is measured in a unit like a single amount is, and records it the same way.
    await expect(subClaimValues(page, PROPERTY_IDS.ELEVATION_RANGE, PROPERTY_IDS.IN_UNIT), "the unit the elevation range was measured in").toHaveCount(1)
    await checkpointElement(page, elevationValues, "claim-values-amountinterval-elevation")

    const elevation = (await elevationValues.textContent())!.trim()

    // An interval whose ends are estimates rather than measurements renders the same way, so the type and not
    // the property is what decides how it looks. This moon is one of the worlds whose population was recorded
    // as a range instead of as a count.
    await openAllProperties(page, MOON_ID, null)
    const populationValues = propertyValues(page, PROPERTY_IDS.POPULATION_ESTIMATE)
    await expect(populationValues, "estimated population claims").toHaveCount(1)
    await expect(populationValues.locator(".pd-claimvalueamountinterval-from"), "the lower end of the estimated population").toHaveText(/^\d+(\.\d+)?$/)
    await expect(populationValues.locator(".pd-claimvalueamountinterval-to"), "the upper end of the estimated population").toHaveText(/^\d+(\.\d+)?$/)
    await expect(subClaimValues(page, PROPERTY_IDS.POPULATION_ESTIMATE, PROPERTY_IDS.IN_UNIT), "the unit the population was estimated in").toHaveCount(1)
    await checkpointElement(page, populationValues, "claim-values-amountinterval-population")

    console.log(`Successfully verified how an amount interval claim value renders, with the elevation range reading ${elevation}.`)
  })

  test("Test time claim values at every precision the data records", async ({ context }) => {
    const page = await context.newPage()

    for (const { what, precision, id, property, text, imprecise } of TIME_PRECISIONS) {
      await openAllProperties(page, id, `claim-values-time-${precision}-allproperties`)

      const values = propertyValues(page, property)
      await expect(values, `claims of the property recorded to ${what}`).toHaveCount(1)
      const time = values.locator(".pd-claimvaluetime")
      await expect(time, `the timestamp recorded to ${what}`).toBeVisible()
      await expect(time, `the timestamp recorded to ${what} is written out to that precision`).toHaveText(text)
      // The parts of the timestamp which the precision does not reach are rendered as zeros and marked, so that
      // a date known only to the decade is not read as a date known to the day.
      const impreciseParts = await time.locator(".pd-timedisplay-part-imprecise").allTextContents()
      expect(impreciseParts.join(""), `the imprecise tail of the timestamp recorded to ${what}`).toBe(imprecise)
      await checkpointElement(page, values, `claim-values-time-${precision}`)
    }

    console.log(`Successfully verified how a time claim value renders at each of the ${TIME_PRECISIONS.length} precisions the test data records.`)
  })

  test("Test time interval claim values which are closed, open ended and of unknown start", async ({ context }) => {
    const page = await context.newPage()

    // A closed interval shows both of its ends by the time display, each written out to the precision its own
    // end was recorded to.
    await openAllProperties(page, GALAXY_ID, "claim-values-galaxy-allproperties")
    const surveyValues = propertyValues(page, PROPERTY_IDS.SURVEY_PERIOD)
    await expect(surveyValues, "survey period claims of the galaxy").toHaveCount(1)
    await expect(surveyValues.locator(".pd-claimvaluetimeinterval-from"), "the start of a closed period").toHaveText(/^\d{4}$/)
    await expect(surveyValues.locator(".pd-claimvaluetimeinterval-to"), "the end of a closed period").toHaveText(/^\d{4}$/)
    await expect(surveyValues.locator(".pd-timedisplay"), "both ends of a closed period are shown by the time display").toHaveCount(2)
    await checkpointElement(page, surveyValues, "claim-values-timeinterval-closed")

    // An interval which is recorded as having no end at all names the end rather than leaving it blank, so that
    // a survey which is still running is not read as one whose end was never written down.
    await openAllProperties(page, RINGED_PLANET_ID, "claim-values-planet-allproperties")
    const openValues = propertyValues(page, PROPERTY_IDS.SURVEY_PERIOD)
    await expect(openValues, "survey period claims of the planet").toHaveCount(1)
    await expect(openValues.locator(".pd-claimvaluetimeinterval-from"), "the start of an open ended period").toHaveText(/^\d{4}$/)
    const openEnd = openValues.locator(".pd-claimvaluetimeinterval-to")
    await expect(openEnd, "the end of an open ended period is named").not.toHaveText(/^\s*$/)
    await expect(openEnd, "the end of an open ended period is not a timestamp").toHaveText(/^\D+$/)
    await expect(openValues.locator(".pd-timedisplay"), "only the start of an open ended period is shown by the time display").toHaveCount(1)
    await checkpointElement(page, openValues, "claim-values-timeinterval-open")

    // An interval whose start is recorded as unknown says so in the same place the start would be, which is a
    // statement about the record and not a gap in it.
    await openAllProperties(page, SITE_ID, "claim-values-site-interval-allproperties")
    const unknownStartValues = propertyValues(page, PROPERTY_IDS.OCCUPATION_PERIOD)
    await expect(unknownStartValues, "period of occupation claims").toHaveCount(1)
    const unknownStart = unknownStartValues.locator(".pd-claimvaluetimeinterval-from")
    await expect(unknownStart, "the start of a period whose start is unknown is named").not.toHaveText(/^\s*$/)
    await expect(unknownStart, "the start of a period whose start is unknown is not a timestamp").toHaveText(/^\D+$/)
    await expect(unknownStartValues.locator(".pd-claimvaluetimeinterval-to"), "the end of a period whose start is unknown").toHaveText(/^\d{4}$/)
    await expect(unknownStartValues.locator(".pd-timedisplay"), "only the end of a period whose start is unknown is shown by the time display").toHaveCount(1)
    await checkpointElement(page, unknownStartValues, "claim-values-timeinterval-unknownstart")

    console.log("Successfully verified how a time interval claim value renders when it is closed, when it has no end and when its start is unknown.")
  })

  test("Test link claim values for a web address and for a file", async ({ context }) => {
    const page = await context.newPage()

    // A link claim without a name to show instead is the address itself, so the reader sees where it goes.
    await openAllProperties(page, INSTITUTE_ID, "claim-values-institute-allproperties")
    const websiteValues = propertyValues(page, PROPERTY_IDS.WEBSITE)
    await expect(websiteValues, "website claims").toHaveCount(1)
    const websiteLink = websiteValues.locator(".pd-claimvaluelink")
    await expect(websiteLink, "the website link").toBeVisible()
    const websiteHref = await websiteLink.getAttribute("href")
    expect(websiteHref, "the website link points at a web address").toMatch(/^https:\/\//)
    await expect(websiteLink, "the website link shows its address").toHaveText(websiteHref!)
    // A link which leaves the site is marked as such, which is what the icon next to it is drawn from.
    await expect(websiteLink, "the website link is marked as leaving the site").toHaveClass(/pd-link-external/)
    await checkpointElement(page, websiteValues, "claim-values-link-website")

    // A link claim pointing at a file of this instance is marked as a file rather than as a page, and shows the
    // name the file was stored under instead of the address it is served from.
    await openAllProperties(page, RINGED_PLANET_ID, "claim-values-planet-link-allproperties")
    const imageValues = propertyValues(page, PROPERTY_IDS.IMAGE)
    await expect(imageValues, "image claims").toHaveCount(1)
    const imageLink = imageValues.locator(".pd-claimvaluelink")
    await expect(imageLink, "the image link").toBeVisible()
    await expect(imageLink, "the image link points at a file of this instance").toHaveAttribute("href", /^\/f\/[0-9A-Za-z]+$/)
    await expect(imageLink, "the image link is marked as a file").toHaveClass(/pd-link-file/)
    await expect(imageLink, "the image link shows a file name and not its address").toHaveText(/\.[a-z0-9]+$/)
    await expect(imageLink, "the image link does not show the address it is served from").not.toHaveText(/^\/f\//)
    await checkpointElement(page, imageValues, "claim-values-link-image")

    console.log("Successfully verified how a link claim value renders for a web address and for a file of this instance.")
  })

  test("Test has, none and unknown claim values", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, RINGED_PLANET_ID, "claim-values-planet-absences-allproperties")

    // A HAS claim carries no value at all: stating the property is the whole statement, so its row is the
    // property's label and nothing beside it. It is still a row of its own, so the property is listed.
    const ringRow = propertyRow(page, PROPERTY_IDS.HAS_RING_SYSTEM)
    await expect(ringRow, "the row of the ring system claim").toHaveCount(1)
    await expect(ringRow.locator(".pd-propertiesview-label-text"), "the ring system claim is labelled by its property").not.toHaveText(/^\s*$/)
    await expect(propertyValues(page, PROPERTY_IDS.HAS_RING_SYSTEM), "the ring system claim renders no value").toHaveCount(0)
    // A row of the properties table is laid out as contents rather than as a box of its own, so the label cell
    // is what a screenshot of the row can be clipped to.
    await checkpointElement(page, ringRow.locator(".pd-propertiesview-label"), "claim-values-has-ringsystem")

    // An unknown claim says that the document has a value for the property but that the value is not known,
    // which is shown by the word for it rather than by a blank.
    const unknownValues = propertyValues(page, PROPERTY_IDS.BIOSPHERE)
    await expect(unknownValues, "biosphere claims of the planet whose biosphere is unknown").toHaveCount(1)
    const unknownValue = unknownValues.locator(".pd-claimvalueunknown")
    await expect(unknownValue, "the unknown biosphere").toBeVisible()
    await expect(unknownValue, "the unknown biosphere is named and not left blank").not.toHaveText(/^\s*$/)
    await checkpointElement(page, unknownValues, "claim-values-unknown-biosphere")

    // A none claim says that the document has no value for the property, which is a statement about the world
    // and not a gap in the record, so it is shown by the word for it rather than by a blank. This planet is
    // recorded as having no biosphere at all, where the one above is recorded as having one nobody has seen.
    await openAllProperties(page, BARE_PLANET_ID, "claim-values-planet-none-allproperties")
    const noneValues = propertyValues(page, PROPERTY_IDS.BIOSPHERE)
    await expect(noneValues, "biosphere claims of the planet which has none").toHaveCount(1)
    const noneValue = noneValues.locator(".pd-claimvaluenone")
    await expect(noneValue, "the missing biosphere").toBeVisible()
    await expect(noneValue, "the missing biosphere is named and not left blank").not.toHaveText(/^\s*$/)
    await expect(noneValues.locator(".pd-claimvalueunknown"), "a biosphere recorded as absent is not recorded as unknown").toHaveCount(0)
    await checkpointElement(page, noneValues, "claim-values-none-biosphere")

    // The same property holds a description of the biosphere on the worlds which have one, so the absence has
    // to be told apart from a description as well as from the value being unknown.
    await expect(noneValues.locator(".pd-claimvaluehtml"), "a biosphere recorded as absent renders no description").toHaveCount(0)

    // A HAS claim of another property on another document renders the same way, so it is the type and not the
    // property which decides.
    await openAllProperties(page, MOON_ID, "claim-values-moon-allproperties")
    const lockedRow = propertyRow(page, PROPERTY_IDS.TIDALLY_LOCKED)
    await expect(lockedRow, "the row of the tidally locked claim").toHaveCount(1)
    await expect(propertyValues(page, PROPERTY_IDS.TIDALLY_LOCKED), "the tidally locked claim renders no value").toHaveCount(0)
    await checkpointElement(page, lockedRow.locator(".pd-propertiesview-label"), "claim-values-has-tidallylocked")

    console.log("Successfully verified how HAS, none and unknown claim values render.")
  })

  test("Test the time display switches every timestamp between absolute and relative", async ({ context }) => {
    const page = await context.newPage()

    await openAllProperties(page, RINGED_PLANET_ID, "claim-values-planet-timeformat-allproperties")

    // The choice between writing a timestamp out and saying how far away it is belongs to the reader and not to
    // the timestamp, so it is made once for the whole page: clicking any timestamp switches all of them.
    const timestamps = page.locator(".pd-documentget-panel-allproperties .pd-timedisplay")
    const count = await timestamps.count()
    expect(count, "timestamps of the planet").toBeGreaterThan(1)
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-timedisplay-relative"), "timestamps are written out to begin with").toHaveCount(0)

    const claimTime = page.locator(".pd-documentget-panel-allproperties .pd-claimvaluetime").first()
    const absolute = (await claimTime.textContent())!.trim()
    await claimTime.click()

    // The relative phrasing is worked out against the wall clock, so it is asserted to be there and to say
    // something rather than compared against a screenshot which would go stale.
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-timedisplay-relative"), "every timestamp switched to the relative form").toHaveCount(count)
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-timedisplay-part"), "no timestamp is still written out").toHaveCount(0)
    const relative = (await claimTime.textContent())!.trim()
    expect(relative, "the timestamp which was clicked says something in the relative form").not.toBe("")
    expect(relative, "the relative form differs from the written out one").not.toBe(absolute)

    // Only a plain click switches the format: a click with a modifier is how a reader opens what they clicked
    // somewhere else, so it has to reach whatever is under the timestamp instead of being taken by it.
    await claimTime.click({ modifiers: ["Control"] })
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-timedisplay-relative"), "a click with a modifier leaves the format alone").toHaveCount(count)

    // Clicking again plainly switches back, so the choice is a toggle and not a one way door.
    await claimTime.click()
    await expect(page.locator(".pd-documentget-panel-allproperties .pd-timedisplay-relative"), "clicking again writes the timestamps out").toHaveCount(0)
    await expect(claimTime, "the timestamp which was clicked is written out again").toHaveText(absolute)
    await checkpoint(page, "claim-values-planet-timeformat-back", { mask: volatile(page) })

    console.log(`Successfully switched all ${count} timestamps of a document between the written out and the relative form with one click.`)
  })

  test("Test only the claims of the current language render in the class tab", async ({ context }) => {
    const page = await context.newPage()

    // The class tab renders the claims which match the language the interface is in, falling back through the
    // chain the site declares, while the "all properties" tab renders every claim whatever language it was
    // recorded in. This institute is named in two languages and described in three, so the two tabs differ.
    const names = new Map<string, string>()
    const descriptions = new Map<string, string>()
    for (const language of LANGUAGES) {
      await goHome(page)
      await switchLanguage(page, language)
      await openDocument(page, INSTITUTE_ID)
      await settle(page)

      const nameValues = fieldValues(page, PROPERTY_IDS.NAME)
      await expect(nameValues, `name values shown in ${language}`).toHaveCount(1)
      names.set(language, (await nameValues.textContent())!.trim())
      const descriptionValues = fieldValues(page, PROPERTY_IDS.DESCRIPTION)
      await expect(descriptionValues, `description values shown in ${language}`).toHaveCount(1)
      descriptions.set(language, (await descriptionValues.textContent())!.trim())

      // A claim recorded in no language at all belongs to every one of them, so it is shown whatever the
      // interface is set to.
      const websiteValues = fieldValues(page, PROPERTY_IDS.WEBSITE)
      await expect(websiteValues, `website values shown in ${language}`).toHaveCount(1)
      await expect(websiteValues.locator(".pd-claimvaluelink"), `the website link in ${language}`).toHaveAttribute("href", /^https:\/\//)

      await checkpointElement(page, page.locator(".pd-documentget-panel-properties .pd-fieldsview").first(), `claim-values-language-fields-${language}`)

      // The tab which shows every claim shows both names and all three descriptions in every language, so what
      // the class tab leaves out is a choice of the view and not something missing from the document.
      await openDocumentTab(page, "allproperties")
      await settle(page)
      await expect(propertyValues(page, PROPERTY_IDS.NAME), `name claims of the document in ${language}`).toHaveCount(2)
      await expect(propertyValues(page, PROPERTY_IDS.DESCRIPTION), `description claims of the document in ${language}`).toHaveCount(3)
    }

    // The document is named in English and in Slovenian, so those two readings differ, while Portuguese has no
    // name of its own and falls back through English, which is what the site's language priority says.
    expect(names.get("sl"), "the name shown in Slovenian differs from the English one").not.toBe(names.get("en"))
    expect(names.get("pt"), "the name shown in Portuguese falls back to the English one").toBe(names.get("en"))
    // The description was recorded in all three, so no two readings of it are the same.
    expect(new Set(descriptions.values()).size, "the description differs in each of the three languages").toBe(LANGUAGES.length)

    console.log(
      `Successfully verified that the class tab renders one name per language, with ${new Set(names.values()).size} distinct readings across ${LANGUAGES.length} languages.`,
    )
  })
})
