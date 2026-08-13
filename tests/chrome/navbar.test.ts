import type { Page } from "@playwright/test"

import { CLASS_IDS, LANGUAGES, PROPERTY_IDS } from "../peerdb_utils"
import {
  checkpoint,
  checkpointElement,
  expect,
  expectDocument,
  expectResults,
  goHome,
  mockUsername,
  openUserMenu,
  PEERDB_URL,
  resultIds,
  searchByProperty,
  settleFilters,
  signIn,
  signOut,
  switchLanguage,
  test,
  volatile,
} from "../utils"

// The class the navbar tests search by. Individuals are numerous enough for the results to fill the feed
// and for walking them with the previous and next buttons to have somewhere to go.
const SEARCH_CLASS = CLASS_IDS.INDIVIDUAL

// Clicks a navbar element which starts or updates a search session and waits until the new session has
// rendered. Every such click ends in a session of its own, so waiting for the location to change is what
// tells the old results from the new ones.
async function clickIntoSearch(page: Page, selector: string): Promise<void> {
  const element = page.locator(selector)
  await expect(element).toBeVisible()
  const before = page.url()
  await element.click()
  await page.waitForFunction((url) => window.location.href !== url, before)
  await expectResults(page)
}

// Opens a search of one class through the search shortcut route, which is the shortest way to a results
// page whose navbar the tests below then look at.
async function openClassSearch(page: Page): Promise<void> {
  await searchByProperty(page, PROPERTY_IDS.INSTANCE_OF, SEARCH_CLASS)
}

test.describe("PeerDB Navbar Flows", () => {
  for (const language of LANGUAGES) {
    test(`Test navbar on the home page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await checkpoint(page, `navbar-home-${language}`)

      // The home view has a navbar of its own: it carries only the trailing actions, because the home
      // view has its own search box and its own logo in the page body.
      await expect(page.locator(".pd-homenavbar"), "home navbar").toBeVisible()
      await expect(page.locator("#navbar"), "navbar").toBeVisible()
      await expect(page.locator("#navbar-link-home"), "the home navbar carries no link back home").toHaveCount(0)
      await expect(page.locator("#search-input-text"), "the home navbar carries no search box of its own").toHaveCount(0)
      await expect(page.locator("#home-input-search"), "search box of the home view").toBeVisible()
      await expect(page.locator("#home-button-search"), "search button of the home view").toBeVisible()
      await expect(page.locator("#home-link-logo"), "logo of the home view").toBeVisible()
      await expect(page.locator("#navbar-button-signin"), "sign in button").toBeVisible()
      await expect(page.locator(".pd-languageswitcher-button"), "language switcher").toBeVisible()
      // Creating is offered to a signed-in user with a role which grants it, so a visitor is offered none.
      await expect(page.locator(".pd-createbutton"), "the create button").toHaveCount(0)

      console.log(`Successfully verified the home page navbar in ${language}.`)
    })

    test(`Test navbar on a search page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      await openClassSearch(page)
      await settleFilters(page)
      await checkpoint(page, `navbar-search-${language}`, { mask: volatile(page) })

      // Away from home the navbar is the full one: the logo leads back home and the search box sits
      // next to it.
      await expect(page.locator(".pd-homenavbar"), "the home navbar is not used away from home").toHaveCount(0)
      await expect(page.locator("#navbar-link-home"), "link back to the home view").toBeVisible()
      await expect(page.locator(".pd-navbar-logo"), "logo of the navbar").toBeVisible()
      await expect(page.locator("#search-input-text"), "search box of the navbar").toBeVisible()
      await expect(page.locator(".pd-navbarsearch-button"), "search button of the navbar").toBeVisible()
      await expect(page.locator("#navbar-button-signin"), "sign in button").toBeVisible()

      // The logo is the way back to the home view, where the home navbar takes over again.
      const logo = page.locator("#navbar-link-home")
      await logo.click()
      await expect(page.locator("#home-input-search"), "the home view is reached again").toBeVisible()
      await expect(page.locator(".pd-homenavbar"), "the home navbar takes over again").toBeVisible()
      await checkpoint(page, `navbar-search-back-home-${language}`)

      console.log(`Successfully verified the search page navbar in ${language}.`)
    })

    test(`Test navbar on a document page in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await openClassSearch(page)

      const firstResult = page.locator(".pd-searchresult-link-title").first()
      await expect(firstResult).toBeVisible()
      await firstResult.click()
      await expectDocument(page)
      await checkpoint(page, `navbar-document-session-${language}`, { mask: volatile(page) })

      // Reached from a search, the document navbar replaces the search box with the session's query
      // (a link back to the results) and adds the buttons which walk the results.
      await expect(page.locator("#navbar-link-home"), "link back to the home view").toBeVisible()
      await expect(page.locator(".pd-inputtextlink"), "link back to the results").toBeVisible()
      // The compact button which stands in for the query link is rendered next to it, but it takes over
      // only once the navbar can no longer fit the link, which at this width it can.
      await expect(page.locator(".pd-navbarshortcut-button"), "the compact stand-in for the query link").toBeHidden()
      await expect(page.locator("#documentget-button-prev"), "previous result button").toBeVisible()
      await expect(page.locator("#documentget-button-next"), "next result button").toBeVisible()
      await expect(page.locator("#search-input-text"), "the search box gives way to the query link").toHaveCount(0)

      const documentID = new URL(page.url()).pathname.split("/")[2]

      const next = page.locator("#documentget-button-next")
      await next.click()
      await page.waitForFunction((id) => !window.location.pathname.includes(id), documentID)
      await expectDocument(page)
      await checkpoint(page, `navbar-document-session-next-${language}`, { mask: volatile(page) })
      await expect(page.locator("#documentget-button-prev"), "previous result button after walking forward").toBeVisible()

      // The query itself is a link back to the results the document was reached from.
      const backToSearch = page.locator(".pd-inputtextlink")
      await backToSearch.click()
      await expectResults(page)
      await settleFilters(page)
      await checkpoint(page, `navbar-document-back-to-search-${language}`, { mask: volatile(page) })

      // Opened on its own, without a search session, the document navbar carries the search box
      // instead, and there are no results to walk.
      await page.goto(`${PEERDB_URL}/d/${documentID}`)
      await expectDocument(page)
      await checkpoint(page, `navbar-document-direct-${language}`, { mask: volatile(page) })
      await expect(page.locator("#search-input-text"), "search box of the navbar").toBeVisible()
      await expect(page.locator(".pd-navbarsearch-button"), "search button of the navbar").toBeVisible()
      await expect(page.locator(".pd-inputtextlink"), "there is no session to link back to").toHaveCount(0)
      await expect(page.locator("#documentget-button-prev"), "there are no results to walk backwards").toHaveCount(0)
      await expect(page.locator("#documentget-button-next"), "there are no results to walk forwards").toHaveCount(0)

      console.log(`Successfully verified the document page navbar in ${language}.`)
    })

    test(`Test navbar search box in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)

      // A document opened on its own is the plainest view with the navbar search box: there is no
      // search session yet, so submitting the box has to start one. The link a result carries names the
      // session it was found in, which would open the document inside that session and put the query link
      // in the navbar in place of the box, so only the path of the link is opened.
      await openClassSearch(page)
      const firstResult = page.locator(".pd-searchresult-link-details").first()
      const href = await firstResult.getAttribute("href")
      await page.goto(`${PEERDB_URL}${new URL(href || "", PEERDB_URL).pathname}`)
      await expectDocument(page)
      await checkpoint(page, `navbar-searchbox-before-${language}`, { mask: volatile(page) })

      const searchInput = page.locator("#search-input-text")
      await expect(searchInput).toBeVisible()
      await searchInput.fill("weir")
      await checkpoint(page, `navbar-searchbox-filled-${language}`, { mask: volatile(page) })

      await clickIntoSearch(page, ".pd-navbarsearch-button")
      await checkpointElement(page, page.locator(".pd-navbar"), `navbar-searchbox-results-${language}`)
      await expect(searchInput, "the box keeps the query it was submitted with").toHaveValue("weir")
      const firstQueryResults = await resultIds(page)
      expect(firstQueryResults.length, "the first query finds documents").toBeGreaterThan(0)

      // On the results the same box edits the query of the running session rather than starting from
      // scratch, so submitting it again has to bring different results.
      await searchInput.fill("basin")
      await clickIntoSearch(page, ".pd-navbarsearch-button")
      await checkpointElement(page, page.locator(".pd-navbar"), `navbar-searchbox-results-updated-${language}`)
      await expect(searchInput, "the box keeps the second query").toHaveValue("basin")

      // What says the box edited the running session is that the results are of the second query and not
      // of the first, which is asserted on the documents themselves. The results of a query are ranked by
      // relevance, and relevance rests on term statistics which shift as the index merges its segments in
      // the background, so the results are compared as sets and are not screenshotted.
      const secondQueryResults = await resultIds(page)
      expect(secondQueryResults.length, "the second query finds documents").toBeGreaterThan(0)
      expect(secondQueryResults, "the second query brings other documents than the first").not.toEqual(firstQueryResults)

      console.log(`Successfully verified the navbar search box in ${language}, with ${firstQueryResults.length} and then ${secondQueryResults.length} results.`)
    })

    test(`Test navbar sign in and sign out in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      await goHome(page)
      await switchLanguage(page, language)
      await expect(page.locator("#navbar-button-signin")).toBeVisible()
      await expect(page.locator(".pd-createbutton")).toHaveCount(0)
      await checkpoint(page, `navbar-signed-out-${language}`)

      // Signing in adds the create button to the navbar and gives the user a menu of their own, named
      // after them, which everything of theirs moves into: the sign out button and the language
      // switcher among it, so neither is in the navbar itself any more.
      await signIn(page, ["curator"])
      await expect(page.locator(".pd-createbutton"), "the create button of a role which may create").toBeVisible()
      await expect(page.locator("#navbar-button-signin"), "the sign in button is gone once signed in").toHaveCount(0)
      await expect(page.locator(".pd-navbarmenu-button")).toHaveText(mockUsername(["curator"]))
      await expect(page.locator(".pd-languageswitcher-button"), "the language switcher moves into the menu").toHaveCount(0)
      await checkpoint(page, `navbar-signed-in-${language}`)

      // Opened, the menu holds the roles signed in with, the identity they make, the link to the user's
      // own editing sessions, and the sign out button. The language is kept, because it is stored in a
      // cookie and signing in reloads the application.
      await openUserMenu(page)
      await expect(page.locator(".pd-navbaruser-roles")).toHaveText("curator")
      await expect(page.locator(".pd-navbaruser-id span").first()).toHaveText(/^mock-user-curator@/)
      await expect(page.locator(".pd-navbaruser-sessions"), "link to the user's editing sessions").toBeVisible()
      await expect(page.locator("#navbar-button-signout")).toBeVisible()
      await expect(page.locator(".pd-languageswitcher-button")).toHaveText(new RegExp(language, "i"))
      // The open menu is checkpointed on its own rather than as a whole page: it hangs off the navbar
      // over the top of the view, so a full page adds everything below it without saying anything more
      // about the menu.
      await checkpointElement(page, page.locator(".pd-navbarmenu-panel"), `navbar-signed-in-menu-${language}`)

      // The full navbar shows the same buttons as the home one for a signed-in user.
      await openClassSearch(page)
      await expect(page.locator(".pd-createbutton")).toBeVisible()
      await expect(page.locator(".pd-navbarmenu-button")).toBeVisible()
      await settleFilters(page)
      await checkpoint(page, `navbar-signed-in-search-${language}`, { mask: volatile(page) })

      await signOut(page)
      await expectResults(page)
      await expect(page.locator(".pd-createbutton"), "the create button goes away with the role which granted it").toHaveCount(0)
      await settleFilters(page)
      await checkpoint(page, `navbar-signed-out-again-${language}`, { mask: volatile(page) })

      console.log(`Successfully verified signing in and out from the navbar in ${language}.`)
    })

    test(`Test navbar language switcher in ${language}`, async ({ context }) => {
      const page = await context.newPage()

      const other = language === "en" ? "sl" : "en"

      await goHome(page)
      await switchLanguage(page, language)
      await checkpoint(page, `navbar-languageswitcher-closed-${language}`)

      // The switcher is a menu which lists every enabled language, the current one included.
      const switcher = page.locator(".pd-languageswitcher-button")
      await expect(switcher).toBeVisible()
      await switcher.click()
      await expect(page.locator(".pd-languageswitcher-menu")).toBeVisible()
      for (const option of LANGUAGES) {
        await expect(page.locator(`.pd-languageswitcher-item-${option}`), `option for ${option}`).toBeVisible()
      }
      await expect(page.locator(".pd-languageswitcher-item"), "the switcher lists every enabled language").toHaveCount(LANGUAGES.length)
      await checkpoint(page, `navbar-languageswitcher-open-${language}`)

      // Choosing the other language closes the menu and relabels the interface. The label of the search
      // button of the home view is written in the interface language, so it is what says the labels
      // changed, and it is on the same page as the switcher.
      const searchButton = page.locator("#home-button-search")
      const before = ((await searchButton.textContent()) || "").trim()
      expect(before, "the search button is labelled").not.toBe("")
      const otherOption = page.locator(`.pd-languageswitcher-item-${other}`)
      await otherOption.click()
      await expect(page.locator(".pd-languageswitcher-menu")).toHaveCount(0)
      await expect(switcher).toHaveText(new RegExp(other, "i"))
      await expect(searchButton, "the interface is relabelled in the chosen language").not.toHaveText(before)
      await checkpoint(page, `navbar-languageswitcher-switched-${language}`)

      // Choosing the language back restores the labels, so the switch goes both ways.
      await switcher.click()
      await expect(page.locator(".pd-languageswitcher-menu")).toBeVisible()
      await page.locator(`.pd-languageswitcher-item-${language}`).click()
      await expect(switcher).toHaveText(new RegExp(language, "i"))
      await expect(searchButton, "the labels come back with the language").toHaveText(before)
      await checkpoint(page, `navbar-languageswitcher-restored-${language}`)

      console.log(`Successfully verified the navbar language switcher in ${language}.`)
    })
  }
})
