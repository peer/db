import type { BrowserContext, Locator, Page, Response } from "@playwright/test"
import type { Result } from "axe-core"
import type { PageScreenshotOptions } from "playwright-core"

import AxeBuilder from "@axe-core/playwright"
import { test as baseTest } from "@playwright/test"
import { createHtmlReport } from "axe-html-reporter"
import serialize from "canonicalize"
import { createHash } from "node:crypto"
import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, writeFileSync } from "node:fs"

// Allowed console message patterns.
const CONSOLE_ALLOWLIST = [/^Failed to load resource: the server responded with a status of 400 \(\)$/, /\[vite]/, /\[Vue/]

export const PEERDB_URL = process.env.PEERDB_URL || "https://localhost:8080"

// Extend BrowserContext to include console messages.
interface ExtendedBrowserContext extends BrowserContext {
  _consoleMessages: Array<Promise<string>>
}

export const test = baseTest.extend({
  context: async ({ browser }, use) => {
    const context = (await browser.newContext()) as ExtendedBrowserContext
    // Initialize console messages array for this specific context.
    context._consoleMessages = []

    await context.exposeFunction("collectIstanbulCoverage", (coverageJSON: string) => {
      if (!coverageJSON) {
        return
      }
      mkdirSync(".nyc_output", { recursive: true })
      const filename = `.nyc_output/${Math.random().toString(36).substring(2, 15)}.json`
      writeFileSync(filename, coverageJSON, { flag: "wx" })
      console.log(`Coverage snapshot written to ${filename}.`)
    })

    await context.addInitScript(() =>
      // Collect coverage before page unload.
      window.addEventListener("beforeunload", () =>
        (window as unknown as { collectIstanbulCoverage: (coverageJSON: string) => void }).collectIstanbulCoverage(
          JSON.stringify((globalThis as { __coverage__?: unknown }).__coverage__),
        ),
      ),
    )

    context.on("page", (page) => {
      // Hide carets in all input elements once the page loads.
      page.on("load", async () => {
        await page.addStyleTag({ content: "input,textarea,[contenteditable] { caret-color: transparent !important; }" }).catch(() => {
          // Ignore errors if page navigates before style is added.
        })
      })

      page.on("console", (msg) => {
        const url = page.url()
        const type = msg.type()
        const text = msg.text()
        // Running playwright in headed mode tries to fetch favicon, which returns 404, so we allow it.
        const favicon404 = /^Failed to load resource: the server responded with a status of 404 \(\)$/.test(text) && msg.location().url.endsWith("favicon.ico")
        // A document which the caller may not read is fetched and handled by the frontend, which shows
        // the user that access was denied. The browser logs the failed request anyway and there is no way
        // to tell it that the failure is expected, so we allow a 403 on a document fetch. The document ID can
        // be followed by a query string (a version, for example), so we match the end of the path and not the
        // end of the URL.
        const document403 =
          /^Failed to load resource: the server responded with a status of 403 \(\)$/.test(text) && /\/api\/d\/[0-9A-Za-z]+(?:[?#]|$)/.test(msg.location().url)
        if (!favicon404 && !document403 && !CONSOLE_ALLOWLIST.some((pattern) => pattern.test(text))) {
          const messagePromise = (async function () {
            let argsMsg
            try {
              const args = await Promise.all(msg.args().map((arg) => arg.jsonValue()))
              argsMsg = args.length ? "\nArgs\n" + args.join("\n") : ""
            } catch (error) {
              argsMsg = `\nError resolving arguments: ${String(error)}`
            }
            return `at ${url}: [${type}] ${text}${argsMsg}`
          })()
          context._consoleMessages.push(messagePromise)
        }
      })
    })

    try {
      await use(context)
    } finally {
      // Don't close the browser when interrupting the test (during development).
      if (baseTest.info().status !== "interrupted") {
        await context.close()
      }
    }
  },
})

export const expect = test.expect

// Generate accessibility report after all tests complete. The directory is created rather than read as it
// is: a run in which nothing was found never creates it, and a report of no violations is what such a run
// should produce, not a failure of whichever test happened to finish last.
test.afterAll(() => {
  mkdirSync("a11y-report", { recursive: true })
  createHtmlReport({
    results: {
      violations: readdirSync("a11y-report")
        .filter((f) => f.endsWith(".json"))
        .map((f) => JSON.parse(readFileSync(`a11y-report/${f}`, { encoding: "utf-8" })) as Result),
    },
    options: {
      projectKey: "Accessibility Report",
      outputDir: "a11y-report",
      reportFileName: "a11y-report.html",
    },
  })
})

// Clear the console errors array for this context.
export function clearConsoleErrors(page: Page): void {
  const context = page.context() as ExtendedBrowserContext
  context._consoleMessages.length = 0
}

// Fails the test if any console errors are present for this context.
export async function expectNoConsoleErrors(page: Page): Promise<void> {
  const context = page.context() as ExtendedBrowserContext
  const resolvedMessages = await Promise.all(context._consoleMessages)
  expect(resolvedMessages.length, `Console errors detected:\n${resolvedMessages.join("\n")}`).toBe(0)
}

interface CheckpointOptions {
  mask?: Array<Locator>
  fullPage?: boolean
  clip?: { x: number; y: number; width: number; height: number }
}

// How long one screenshot may take. The action timeout the rest of the suite runs under covers waiting for
// an element to appear, while capturing a full page is work whose cost grows with the page: a result page
// with every facet shown is several times the height of the viewport, and encoding it takes longer than
// waiting for anything on it ever should.
const SCREENSHOT_TIMEOUT = 60000

// Take up to 10 screenshots, wait until they stabilize. We had issues (and flakiness) because sometimes
// screenshots are not saved fully (just part of the page is visible, the rest is blank). Now we wait
// visually for screenshot to stabilize (instead of waiting just for DOM).
async function takeStableScreenshot(page: Page, options: PageScreenshotOptions): Promise<Buffer> {
  const screenshotOptions = { ...options, timeout: SCREENSHOT_TIMEOUT }
  let olderScreenshot = await page.screenshot(screenshotOptions)
  for (let i = 0; i < 10; i++) {
    await page.waitForTimeout(500)
    const newerScreenshot = await page.screenshot(screenshotOptions)
    if (olderScreenshot.equals(newerScreenshot)) {
      return newerScreenshot
    }
    olderScreenshot = newerScreenshot
  }
  throw new Error(`unable to take stable screenshot: ${screenshotOptions.path}`)
}

// Waits until the page has everything it asked for. The bar across the top is drawn while any request is in
// flight, so its absence says both that the page is done and that whatever the answers still have to change
// on it has been changed. What is passed as "what" names the moment this is waited for, so a failure says
// which step of a checkpoint was waiting.
async function expectNothingInFlight(page: Page, what: string): Promise<void> {
  await expect(page.locator(".pd-navbar-progress"), `the progress bar while ${what}`).toHaveCount(0, { timeout: LOADING_TIMEOUT })
}

export async function checkpoint(page: Page, name: string, { mask = [], fullPage = true, clip }: CheckpointOptions = {}) {
  // A screenshot which catches the progress bar is a screenshot of a page which is not done. It is waited out
  // here rather than at the call sites because it can be lit by anything the page is doing, and it sits over
  // the top of the navbar, which is inside the clip of an element screenshot of the navbar just as much as it
  // is on a whole page.
  await expectNothingInFlight(page, `taking ${name}`)
  // Anchor scroll to the top so position:fixed elements land at the top of fullPage screenshots.
  if (fullPage) {
    await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }))
  }
  // Move mouse to the same location so the same element gets focused every time.
  await page.mouse.move(0, 0)
  const screenshotPath = test.info().snapshotPath(`${name}.png`, { kind: "screenshot" })
  const screenshotOptions = {
    fullPage,
    mask,
    clip,
    ...(existsSync(screenshotPath) ? {} : { path: screenshotPath }),
  }

  const screenshotBuffer = await takeStableScreenshot(page, screenshotOptions)
  if (screenshotOptions.path) {
    // Only attach new screenshots to the report.
    await test.info().attach(name, {
      contentType: "image/png",
      path: screenshotPath,
    })
  } else {
    // Compare snapshot buffer with the existing one. The comparison is soft so that one screenshot which does not match does not hide the rest of the
    // test: the test runs on and is compared at every checkpoint it reaches, and fails at its end with one error per mismatch. It also keeps the checks
    // below running for a checkpoint whose screenshot differs, which a throwing comparison skips exactly where the interface changed.
    expect.soft(screenshotBuffer).toMatchSnapshot(`${name}.png`)
  }

  // Check for duplicate IDs.
  const duplicates = await page.evaluate(() => {
    const ids = Array.from(document.querySelectorAll("[id]"), (el) => (el as HTMLElement).id)
    return Array.from(ids.reduce((acc, v) => acc.set(v, (acc.get(v) || 0) + 1), new Map<string, number>()).entries())
      .filter(([_id, v]) => v > 1)
      .map(([id, _v]) => id)
  })
  if (duplicates.length > 0) {
    throw new Error(`Duplicate IDs found in checkpoint "${name}" at ${page.url()}: ${duplicates.join(", ")}`)
  }

  // Check for accessibility violations.
  const accessibilityScanResults = await new AxeBuilder({ page }).analyze()
  for (const violation of accessibilityScanResults.violations) {
    const serializedViolation: string = serialize(violation) as string
    const violationHash = createHash("sha256").update(serializedViolation).digest("hex")
    mkdirSync("a11y-report", { recursive: true })
    // The violation is written through a temporary file and renamed into place. The report is assembled by
    // reading the whole directory back, and workers run in parallel, so a reader must never come across a
    // file which is half written. A rename within a directory is atomic, so a reader sees either nothing or
    // the whole file. The temporary name does not end in .json, so a reader skips it while it is being written.
    const violationPath = `a11y-report/${violationHash}.json`
    const violationTempPath = `${violationPath}.${process.pid}.tmp`
    writeFileSync(violationTempPath, serializedViolation, { flag: "w" })
    renameSync(violationTempPath, violationPath)
  }

  // Check for any console logs.
  await expectNoConsoleErrors(page)
}

export async function takeScreenshotsOfEntries(
  page: Page,
  entrySelector: string,
  displayNameSelector: string,
  screenshotPrefix: string,
  { mask = [] }: Pick<CheckpointOptions, "mask"> = {},
): Promise<void> {
  // Get all entry elements.
  const entries = page.locator(entrySelector)
  const count = await entries.count()

  for (let i = 0; i < count; i++) {
    const entry = entries.nth(i)
    const box = await entry.boundingBox()
    if (!box) {
      continue
    }

    const displayNameElement = entry.locator(displayNameSelector)
    const displayName = (await displayNameElement.textContent())?.replace(/\s/g, "")

    await checkpoint(page, `${screenshotPrefix}-${displayName}`, { mask, fullPage: true, clip: { x: box.x, y: box.y, width: box.width, height: box.height } })
  }
}

// Takes a checkpoint of one element only, so a regression in the part of the view which was just driven
// is reported against a screenshot of that part rather than only against the whole page. The clip is
// measured with the page scrolled to the top, which is where checkpoint puts it before taking a full page
// screenshot, so the element's viewport box and its box in the full page screenshot are the same.
export async function checkpointElement(page: Page, locator: Locator, name: string, mask: Array<Locator> = []): Promise<void> {
  await expect(locator, `element for ${name}`).toBeVisible()
  await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }))

  // The box is measured on a page which has everything it asked for. A box read while an answer is still
  // on its way is a box of where the element is before the answer is rendered, and the element is then
  // somewhere else by the time the screenshot is taken, which the reads below cannot see: they only say
  // that the element is not moving right now, not that nothing is about to move it.
  await expectNothingInFlight(page, `measuring the element for ${name}`)

  // The clip is a box on the page and not the element itself, so anything above the element which is still
  // settling would move the element out from under the box and the screenshot would be of somewhere else.
  // The box is therefore read until it stops moving before it is used.
  let box = await locator.boundingBox()
  await expect
    .poll(
      async () => {
        const previous = box
        box = await locator.boundingBox()
        return previous !== null && box !== null && previous.x === box.x && previous.y === box.y && previous.width === box.width && previous.height === box.height
      },
      { message: `the box of the element for ${name} stops moving` },
    )
    .toBe(true)

  await checkpoint(page, name, { mask, clip: box! })

  // What was captured is a box of the page, so it is a screenshot of the element only while the element is
  // still in that box. Something above it which settles after the box was measured moves the element out
  // from under it and whatever took its place is captured instead, which reads as the element having
  // changed rather than as the page having moved. The box is read once more so that this is reported as
  // what it is. The comparison is soft for the same reason as the one of the screenshot itself.
  const after = await locator.boundingBox()
  expect.soft(after, `the element for ${name} stayed in the box its screenshot was taken of`).toEqual(box)
}

// Everything below drives the interface PeerDB itself provides, so it is the same for every application
// built on PeerDB. What differs between applications (the namespaces their documents live in, the classes
// and properties they declare, the roles their sites grant and the languages they are served in) is passed
// in by the caller rather than known here.

// How long to wait for the parts of a view which are fetched from the server. The default assertion
// timeout is enough while the site is idle, but tests run next to each other and the ones which write
// make the site answer noticeably slower, so waiting for a fetch is given more room than the default.
export const LOADING_TIMEOUT = 30000

// How many times settleFilters may reveal another batch of facets before it gives up on the filters panel
// ever listing them all.
const FILTER_PASSES = 50

// How many times settleSearch may find the results feed grown since the pass before it, before it gives up on
// the feed ever coming to rest. The feed reveals whole pages, so a handful of passes covers the two or three
// it fills its column with, while a feed which never stops is reported instead of screenshotted mid-reveal.
const RESULT_PASSES = 10

// The API route the results of a search are fetched from. A search which is changed from inside the result
// page fetches its results from it again, under the version the change produced.
const SEARCH_RESULTS_API = "/api/s/results/"

// The API route the facets of a search are fetched from. It is the only source the filters panel builds
// itself from, so every replacement of its list of facets is a response of this route.
const SEARCH_FILTERS_API = "/api/s/filters/"

// Serves the page a site context whose features are the site's own with the given ones changed, for a test
// about a feature the site under test is not served with. The context is fetched once when the application
// boots, so this has to be called before the page is opened, and it holds for every page load which
// follows.
//
// The site's own answer is what is changed, rather than one made up here, because the features are only
// part of what the context carries: everything else about the site stays what it is, and the headers stay
// with it, which is where the application reads the roles and the signed-in user from.
//
// That answer is fetched by the process running the tests rather than by the browser, so the process has to
// trust the certificate the site is served with. The image the tests run in does (see playwright.dockerfile,
// which puts the root of the test certificates into NODE_EXTRA_CA_CERTS). A checkout run against a
// development instance has to be told the same way (e.g., NODE_EXTRA_CA_CERTS=$(mkcert -CAROOT)/rootCA.pem),
// or the fetch never returns and the application waits for its context forever.
export async function overrideSiteFeatures(page: Page, features: Record<string, unknown>): Promise<void> {
  await page.route("**/context.json", async (route) => {
    const response = await route.fetch()
    const context = (await response.json()) as { features?: Record<string, unknown> }
    await route.fulfill({ response, json: { ...context, features: { ...context.features, ...features } } })
  })
}

// Elements whose content changes between runs and would otherwise make every screenshot differ. Pass them
// to checkpoint's mask option.
//
// Only the time of a history entry is masked, and not the whole entry: the time is when the instance under
// test happened to be populated, while the author next to it is the same on every run. Everything else a
// search or a document view shows follows from the test data, the number of results included, so masking
// more than this would hide the regressions the screenshots are taken for.
//
// The cell holding the time is masked rather than the link inside it. A mask covers exactly the box of what
// it masks, and the box of the link is as wide as the time it renders, which in a proportional font differs
// by a pixel between one time and another, so masking the link would leave its edge in the screenshot. The
// cell is half of the table whatever it holds.
export function volatile(page: Page): Array<Locator> {
  return [page.locator(".pd-documenthistory-text-time")]
}

// Opens the home page. Every test starts here so that the navbar is in its initial state.
export async function goHome(page: Page): Promise<void> {
  await page.goto(PEERDB_URL)
  await expect(page.locator("#home-input-search")).toBeVisible()
}

// The name the mock authenticator gives the user signed in with the given roles: the roles in
// alphabetical order, appended to its own name (mockUsername in auth/mock.go).
export function mockUsername(roles: ReadonlyArray<string>): string {
  return ["mock", ...[...roles].sort()].join("-")
}

// Signs in through the mock authenticator with exactly the given roles. Passing no role at all signs in
// as a user who holds none, which is what a test asserting that signing in alone grants nothing needs.
//
// The mock stands in for an identity provider: it takes the browser to a page of its own where the
// roles are chosen, and signing in there sends it back where it started. The roles are picked by the
// label they are listed under, which is the role name the site declares, so the order the page lists
// them in does not matter.
export async function signIn(page: Page, roles: ReadonlyArray<string>): Promise<void> {
  await goHome(page)
  // Signing in starts from the sign in button, which is there only while nobody is signed in, so a test
  // which takes on one identity after another is signed out first.
  if ((await page.locator(".pd-navbarmenu-button").count()) > 0) {
    await signOut(page)
  }
  const signInButton = page.locator("#navbar-button-signin")
  await expect(signInButton).toBeVisible()
  await signInButton.click()

  const mockSignIn = page.locator(".pd-authmocksignin")
  await expect(mockSignIn, "the mock sign-in page").toBeVisible()
  for (const role of roles) {
    // The label holds the checkbox and the name of the role, with the whitespace the markup is
    // written with around it, so the name is matched with that whitespace allowed rather than as the
    // whole text of the label.
    const checkbox = mockSignIn
      .locator("label")
      .filter({ hasText: new RegExp(`^\\s*${role}\\s*$`) })
      .getByRole("checkbox")
    await expect(checkbox, `checkbox of the ${role} role`).toBeVisible()
    await checkbox.check()
  }
  await page.locator("#authmocksignin-button-signin").click()

  // A signed-in user gets a menu of their own, named by them, which is where everything of theirs
  // (the sign-out button among it) moves, so the menu carrying their name is what says the sign-in
  // landed.
  await expect(page.locator(".pd-navbarmenu-button"), "the menu of the signed-in user").toHaveText(mockUsername(roles))
}

// Asserts that the given roles are exactly the ones the mock sign-in page offers, which is what says the
// site is configured with the roles the tests expect. The page lists them in the order the site declares
// them, so the labels are compared as a set.
export async function expectOfferedRoles(page: Page, roles: ReadonlyArray<string>): Promise<void> {
  const labels = page.locator(".pd-authmocksignin label")
  await expect(labels, "the mock sign-in page offers one checkbox per role").toHaveCount(roles.length)
  const offered = (await labels.allTextContents()).map((text) => text.trim()).sort()
  expect(offered, "the roles the mock sign-in page offers").toEqual([...roles].sort())
}

// Opens the menu of the signed-in user, which is where the sign-out button and the language switcher
// live once there is a user to name it after. The menu is left open, because everything in it is
// reached through it.
export async function openUserMenu(page: Page): Promise<void> {
  const menuButton = page.locator(".pd-navbarmenu-button")
  await expect(menuButton).toBeVisible()
  if (!(await page.locator(".pd-navbarmenu-panel").isVisible())) {
    await menuButton.click()
  }
  await expect(page.locator(".pd-navbarmenu-panel")).toBeVisible()
}

export async function signOut(page: Page): Promise<void> {
  await openUserMenu(page)
  const signOutButton = page.locator("#navbar-button-signout")
  await expect(signOutButton).toBeVisible()
  await signOutButton.click()
  await expect(page.locator("#navbar-button-signin")).toBeVisible()
}

// Switches the interface language and waits for the switch to take effect. The switcher sits in the
// navbar for a visitor who is not signed in and inside the user's own menu for one who is, so the
// menu is opened first when there is one.
export async function switchLanguage(page: Page, language: string): Promise<void> {
  const inMenu = (await page.locator(".pd-navbarmenu-button").count()) > 0
  if (inMenu) {
    await openUserMenu(page)
  }
  const switcher = page.locator(".pd-languageswitcher-button")
  await expect(switcher).toBeVisible()
  await switcher.click()
  const option = page.locator(`.pd-languageswitcher-item-${language}`)
  await expect(option).toBeVisible()
  await option.click()
  // Choosing a language relabels the whole interface, which closes the menu the switcher sits in when
  // there is one, so the menu is opened again to read back what the switcher says afterwards.
  if (inMenu) {
    await openUserMenu(page)
  }
  await expect(switcher).toHaveText(new RegExp(language, "i"))
}

// What searchWithQuery may be asked to do beyond running the query.
export interface SearchWithQueryOptions {
  // Whether the home page before the search and the results after it are checkpointed. Without it no
  // checkpoint is taken at all, which is what a test about what a search finds rather than about how the
  // page looks asks for.
  checkpoints?: boolean
  // Whether the search is expected to have found something. A search which found nothing renders no result
  // at all and is waited for differently, see expectNoResults. Defaults to true.
  results?: boolean
}

// Runs a search from the home page and waits for the results to render.
export async function searchWithQuery(page: Page, query: string, options: SearchWithQueryOptions = {}): Promise<void> {
  await goHome(page)

  const searchInput = page.locator("#home-input-search")
  await expect(searchInput).toBeVisible()
  const searchButton = page.locator("#home-button-search")
  await expect(searchButton).toBeVisible()
  if (options.checkpoints) {
    await checkpoint(page, "home-page-before-search")
  }

  await searchInput.fill(query)
  await searchButton.click()

  if (options.results ?? true) {
    await expectResults(page)
  } else {
    await expectNoResults(page)
  }
  if (options.checkpoints) {
    await checkpoint(page, query === "" ? "search-default-results" : `search-query-${query}`)
  }
}

// Waits until a search has finished and the page has settled, whatever the search found. The header
// renders its count only once the search has resolved, and it renders it for a search which found nothing
// just as much as for one which found something, so it is what says the page reached its final state.
// Waiting for a result instead would leave a search which found nothing waiting until it times out.
export async function settleSearch(page: Page): Promise<void> {
  await expect(page.locator(".pd-searchresultsheader")).toBeVisible()
  await expect(page.locator(".pd-searchresultsheader-count-results")).toBeVisible()
  await page.waitForLoadState("networkidle")
  // The feed reveals results by itself until its column reaches about two viewports (fillColumn in
  // SearchResultsFeed.vue), and every batch it reveals fetches the documents of its results, so the feed can
  // still be growing when the network first goes quiet. What is waited for here is that it stopped: a pass
  // which counts as many results as the pass before it, with the page settled in between. A screenshot taken
  // while the feed is still revealing would hold however much of the result set the run happened to reach.
  const results = page.locator(".pd-searchresult")
  let previous = -1
  for (let pass = 0; pass < RESULT_PASSES; pass++) {
    const shown = await results.count()
    if (shown === previous) {
      return
    }
    previous = shown
    await settle(page)
  }
  throw new Error("the results feed kept revealing results")
}

// Waits for a settled search which is expected to have found something, so that a screenshot taken
// afterwards is of the results rather than of a partially loaded page.
export async function expectResults(page: Page): Promise<void> {
  await settleSearch(page)
  await expect(page.locator(".pd-searchresult").first()).toBeVisible()
}

// Waits for a settled search which is expected to have found nothing. The count the header renders is
// then the message saying so, and the feed renders no result at all: it drops its whole result block,
// pagers and load more button included, rather than showing an empty one.
export async function expectNoResults(page: Page): Promise<void> {
  await settleSearch(page)
  await expect(page.locator(".pd-searchresult")).toHaveCount(0)
  await expect(page.locator("#searchresultsfeed-button-loadmore")).toHaveCount(0)
  await expect(page.locator(".pd-searchresultspager")).toHaveCount(0)
}

// The number of results the header reports, which is the size of the whole result set and not the number
// of results the feed currently shows. The number is rendered into a sentence and grouped for the locale,
// so only its digits are read.
export async function resultCount(page: Page): Promise<number> {
  const count = page.locator(".pd-searchresultsheader-count-results")
  await expect(count).toBeVisible()
  const text = (await count.textContent()) || ""
  expect(text.trim(), "the results header reports what the search found").not.toBe("")
  // A search which found nothing is reported as the message saying so, in place of a count, so a report
  // carrying no digits at all is a report of no results.
  const digits = text.replace(/\D/g, "")
  if (digits === "") {
    return 0
  }
  return Number(digits)
}

// The identifiers of the documents the results feed currently shows, in the order it shows them. Every
// result links to its document through its details link, which is there whether or not the document has a
// display label to link by, so this also works for the classes whose documents have no title.
//
// A result which has just been added to the feed is counted as soon as its card is in the document, which
// under load can be before the card has rendered the link the identifier is read from, so this waits until
// every card yields one rather than reporting the ones which are not there yet as missing.
export async function resultIds(page: Page): Promise<Array<string>> {
  const read = async (): Promise<Array<string>> =>
    await page.locator(".pd-searchresult").evaluateAll((cards) =>
      cards.map((card) => {
        const link = card.querySelector("a.pd-searchresult-link-details")
        const match = /\/d\/([0-9A-Za-z]+)/.exec(link?.getAttribute("href") || "")
        return match ? match[1] : ""
      }),
    )

  await expect.poll(async () => (await read()).every((id) => id !== ""), { message: "every result shows the document it links to" }).toBe(true)
  return await read()
}

// The titles of the results the feed currently shows, in the order it shows them, so that a test can assert
// what a change to the sort order or to the grouping did to them. A document without a display label renders
// no title at all, so this reports one title per result which has one and not one entry per result.
export async function resultTitles(page: Page): Promise<Array<string>> {
  const titles = page.locator(".pd-searchresult-link-title")
  await expect(titles.first(), "the results feed shows a title").toBeVisible({ timeout: LOADING_TIMEOUT })
  return await titles.allTextContents()
}

// Loads every result of the current search. The feed fetches the whole result set at once but renders
// only the first page of it, so this asks it for the rest until there is nothing left to add, which is
// what a test comparing whole result sets needs.
//
// The rest is asked for through the load more button, which is the one affordance a visitor has for it
// whatever the site does with scrolling. The press is dispatched rather than clicked, because a click
// first scrolls the button into view, and on a site which loads while scrolling that scrolling loads a
// page of its own and takes the button out from under the press.
export async function loadAllResults(page: Page): Promise<void> {
  const total = await resultCount(page)
  const results = page.locator(".pd-searchresult")
  const loadMore = page.locator("#searchresultsfeed-button-loadmore")

  // What is waited for is the count the header reports, and not that every press adds something. The feed
  // drops the button with the last page, and by the time the results of that page are counted the button may
  // already be gone, so a wait for the button to go away would leave a last, empty attempt to press a button
  // which is not there. A press which lands on a button being taken away is swallowed and the next pass
  // presses again, so the poll is what carries this rather than one press per page. The timeout covers
  // loading every page of a class rather than a single fetch.
  await expect
    .poll(
      async () => {
        await loadMore.dispatchEvent("click").catch(() => null)
        return await results.count()
      },
      { message: "the feed shows every result the search found", timeout: 2 * LOADING_TIMEOUT },
    )
    .toBe(total)

  await expect(loadMore, "the feed offers no more results once all of them are shown").toBeHidden()
}

// Adds every facet the filters panel still has to add, so that a checkpoint of a search is of a panel which
// cannot grow any further.
//
// The panel shows a first batch of facets behind a button, and the handler watching the page for scrolling
// and resizing presses that button whenever the end of the page comes near (onScrollOrResize in
// SearchResultsFeed.vue). Taking a full page screenshot moves the page, so it presses the button too, and a
// checkpoint of a panel which still had facets to add captures however many the capture itself got around to
// loading. How many that is follows how quickly the site answers rather than anything the test does, which
// makes such a screenshot differ for reasons of its own. Once every facet is added there is nothing left for
// the capture to trigger.
//
// The button is pressed rather than the end of the page scrolled to, even though the handler reacts to the
// latter: scrolling to the end of a page which is already at its end fires no scroll event at all, so a panel
// which stopped growing would never be asked for the next batch and the wait below could only time out. The
// press is dispatched rather than clicked, because the panel replaces the button while it re-renders and a
// click waits for an element which is being taken away from under it. This is also what the handler does with
// the button, so nothing is reached in a way the site does not reach it itself.
// A panel which has every facet added is not yet a panel which stays that way: the batch it shows starts over
// from the first one whenever a new list of facets arrives, which brings the button back with only the first
// batch behind it. Waiting for the button to be away therefore has to be a wait for a list which is not being
// replaced any more, and the list has exactly one source, the filters route (useFilters in search.ts). Its URL
// carries the version of the search session and the text typed into the panel's own box, and nothing else asks
// for it, so a response arriving late belongs to something the test itself did and can be waited out.
export async function settleFilters(page: Page): Promise<void> {
  const moreFilters = page.locator(".pd-searchresultsfeed-button-morefilters")
  let filtersFetched = 0
  let lastFiltersURL = ""
  const onResponse = (response: Response) => {
    if (response.url().includes(SEARCH_FILTERS_API)) {
      filtersFetched += 1
      lastFiltersURL = response.url()
    }
  }
  page.on("response", onResponse)
  try {
    // Every pass adds a batch, so the cap is far above what any search of the test data needs, and reaching it
    // means the panel never came to rest. The URL of the list it last fetched is reported with it: its version
    // and its query name what kept asking for a new one.
    for (let pass = 0; pass < FILTER_PASSES; pass++) {
      await settle(page)
      if (await moreFilters.isVisible().catch(() => false)) {
        // The press is dispatched rather than clicked, because the panel replaces the button while it re-renders
        // and a click waits for an element which is being taken away from under it.
        await moreFilters.dispatchEvent("click").catch(() => null)
        continue
      }
      // The button is away while a list is being fetched as well, because a panel with no facets has none to
      // add, and it comes back with the response. So nothing is concluded from one look: the panel is settled
      // once it still offers nothing after another wait and no list arrived in the meantime.
      const fetched = filtersFetched
      await settle(page)
      if (filtersFetched === fetched && !(await moreFilters.isVisible().catch(() => false))) {
        return
      }
    }
    throw new Error(`the filters panel never came to rest, its facets were last fetched from ${lastFiltersURL}`)
  } finally {
    page.off("response", onResponse)
  }
}

// Waits until nothing on the page stands in for something still being fetched: the document itself,
// every display label, every inline reference and every referenced document have all resolved. A
// screenshot taken before that catches the loading placeholders instead of the page they stand in for.
export async function expectNothingLoading(page: Page, timeout: number = LOADING_TIMEOUT): Promise<void> {
  await expect(page.locator(".pd-displaylabel-loading"), "display labels").toHaveCount(0, { timeout })
  await expect(page.locator(".pd-documentrefinline-loading"), "inline references").toHaveCount(0, { timeout })
  await expect(page.locator(".pd-withdocument-loading"), "referenced documents").toHaveCount(0, { timeout })
  await expect(page.locator(".pd-documentget-loading"), "document").toHaveCount(0, { timeout })
}

// Waits until a view which does not poll the server has settled, both on the network and in what it
// renders. Not usable on the edit view, which never goes network-idle, see settleEdit.
export async function settle(page: Page): Promise<void> {
  await page.waitForLoadState("networkidle")
  await expectNothingLoading(page)
}

// What the document view the helpers below wait for is expected to show.
export interface DocumentViewOptions {
  // Whether the document is headed by a title. A class whose documents record a relation between other
  // documents rather than a thing with a name of its own declares neither a display label template nor a
  // field to derive one from, and the view then renders no title at all. Defaults to true.
  titled?: boolean
}

// Waits until the document view has rendered and everything it shows has resolved.
export async function settleDocument(page: Page, options: DocumentViewOptions = {}): Promise<void> {
  await expectDocument(page, options)
  await expectNothingLoading(page)
}

// Waits until the edit view has rendered its form and everything the form shows has resolved.
//
// Waiting for the network to go idle instead, the way the other views are waited for, cannot work here:
// the edit view polls its editing session for changes every 100 milliseconds (pollInterval in
// DocumentEdit.vue) for as long as it is open, so its network is never quiet and the wait can only run
// into the test's timeout. What the view renders is waited for instead.
export async function settleEdit(page: Page): Promise<void> {
  await expect(page.locator(".pd-documentedit"), "edit view").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator(".pd-fieldsform"), "fields form").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentedit-button-save"), "save button").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expectNothingLoading(page)
}

// Waits until the form has taken focus into a field of its own. The form focuses a field as soon as it has
// loaded, which scrolls that field into view, so waiting for it once before anything is driven keeps it
// from taking the focus away from the input a test is working in and from moving the page under a
// screenshot which is being taken.
export async function settleFormFocus(page: Page): Promise<void> {
  await expect
    .poll(async () => await page.evaluate(() => document.activeElement?.tagName ?? ""), { message: "the form takes focus into a field of its own" })
    .not.toBe("BODY")
}

// Hides the panel of potential duplicates of the document being edited. The panel searches the index for
// documents which structurally match the one on the form, so from the second run on it lists the document
// the previous run created, which moves everything below it and makes a screenshot of the view differ
// between runs. Hiding it keeps the comparison on the form.
export async function hideDuplicates(page: Page): Promise<void> {
  await page.addStyleTag({ content: ".pd-documentduplicates { display: none !important; }" })
}

// What a search opened by its address may be told beyond what it searches for.
export interface SearchOptions {
  // Whether the search is expected to have found something, which says how the page is waited for: a search
  // which found nothing renders no result at all (see expectNoResults). Defaults to true.
  results?: boolean
}

// Searches for all documents which carry the given value for the given property. The search shortcut route
// takes a property identifier as the name of a query parameter and a value identifier as its value, and
// redirects to a session prefiltered to them, which puts the result page in the same scope as ticking the
// value in that property's filter but reaches it in one navigation. The property may be a path of them
// joined by colons, which is how a value held by a sub-claim is prefiltered on.
export async function searchByProperty(page: Page, propertyId: string, valueId: string, { results = true }: SearchOptions = {}): Promise<void> {
  await page.goto(`${PEERDB_URL}/s?${propertyId}=${valueId}`)
  if (results) {
    await expectResults(page)
  } else {
    await expectNoResults(page)
  }
}

// Searches for the given text, by opening the result page for it rather than by typing into a search box
// (searchWithQuery does that). An empty query is the search which finds everything, which is where a test
// about the result page itself rather than about what a query matches starts from.
export async function searchByQuery(page: Page, query = "", { results = true }: SearchOptions = {}): Promise<void> {
  await page.goto(`${PEERDB_URL}/s?q=${encodeURIComponent(query)}`)
  if (results) {
    await expectResults(page)
  } else {
    await expectNoResults(page)
  }
}

// The row of the "all properties" tab which is for the given property, addressed by the class the row
// carries, so a row is found without depending on the property's label.
export function propertyRow(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-documentget-panel-allproperties .pd-propertiesview-row-${propertyId}`)
}

// The value cells of the rows of the "all properties" tab which are for one property. A label is rendered
// once per property and a value once per claim, so a property stated more than once has one label and
// several of these.
export function propertyValues(page: Page, propertyId: string): Locator {
  return propertyRow(page, propertyId).locator(".pd-propertiesview-value")
}

// Opens the first search result and waits for the document view.
export async function openFirstResult(page: Page): Promise<void> {
  const first = page.locator(".pd-searchresult-link-title").first()
  await expect(first).toBeVisible()
  await first.click()
  await expectDocument(page)
}

// Waits until a document view has rendered.
export async function expectDocument(page: Page, options: DocumentViewOptions = {}): Promise<void> {
  await expect(page.locator(".pd-documentget")).toBeVisible({ timeout: LOADING_TIMEOUT })
  if (options.titled ?? true) {
    await expect(page.locator("#documentget-title")).toBeVisible()
  } else {
    // Without a title there is nothing at the head of the view to wait for, so the tabs below it are what
    // says the view is rendered, and the title is asserted to be the one thing which is not there.
    await expect(page.locator(".pd-documentget-tabs"), "tabs of the document").toBeVisible()
    await expect(page.locator("#documentget-title"), "the document is headed by no title").toBeHidden()
  }
  await page.waitForLoadState("networkidle")
}

// Opens a document directly by its identifier.
export async function openDocument(page: Page, id: string): Promise<void> {
  await page.goto(`${PEERDB_URL}/d/${id}`)
  await expectDocument(page)
}

// The identifier of the document the browser is on, read out of the address of the document view. A document
// a test creates gets a fresh identifier on every run, so a test which creates one reads it back rather than
// knowing it in advance. The identifier has to end the path, so an address which carries something else after
// "/d/", an editing session and a delete confirmation among them, is reported as not being a document view
// instead of yielding that word as the identifier.
export function documentId(page: Page): string {
  const match = /\/d\/([0-9A-Za-z]+)(?:[?#]|$)/.exec(page.url())
  expect(match, `the browser is on a document: ${page.url()}`).not.toBeNull()
  return match![1]
}

// Switches to one of the tabs of the document view. The tabs are rendered by headless-ui, which generates
// their ids, so they are addressed by the class we add instead. Which tabs there are depends on the
// document and on what the application registers, so the tab is named by the suffix of its hook.
export async function openDocumentTab(page: Page, tab: string): Promise<void> {
  const tabButton = page.locator(`.pd-documentget-tab-${tab}`)
  await expect(tabButton).toBeVisible()
  await tabButton.click()
  await expect(page.locator(`.pd-documentget-panel-${tab}`)).toBeVisible()
}

// Starts an edit session for the document currently shown and waits for the form.
export async function startEdit(page: Page): Promise<void> {
  const editButton = page.locator("#documentget-button-edit")
  await expect(editButton).toBeVisible()
  await editButton.click()
  await settleEdit(page)
}

// The form block of the field holding the given property.
export function field(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-fieldsformfield-${propertyId}`)
}

// The form block of the given property which holds its values together with the instruction written for it.
// A flag annotating a field is a claim on the same property as the values it annotates, so one property can
// have several blocks on the form, and only the one holding the values renders the hints block this picks it
// out by.
export function valueField(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-fieldsformfield-${propertyId}:has(.pd-claimcardinality-text-hints)`)
}

// The input of the first value slot of the field holding the given property, addressed by the class of the
// input, which says which kind of value the field holds. A field matches more inputs of a kind than the one
// its own value is typed into: a field which may hold several values grows an empty slot as soon as its
// first slot is filled, and a field with sub-fields renders their inputs below its own. The field's own
// first value slot comes before both, so the first match is the one.
export function fieldInput(page: Page, propertyId: string, inputClass: string): Locator {
  return field(page, propertyId).locator(inputClass).first()
}

// The slots (the repeated entries) of one field of the edit form. A repeated field lays its slots out inside
// a grid wrapper and a single-value field renders them directly under the field's claim cardinality, so both
// shapes are matched. Scoping to the claim cardinality of the field's own property keeps the slots of its
// sub-fields out of the result.
export function fieldSlots(page: Page, propertyId: string): Locator {
  const cardinality = `.pd-fieldsformfield-${propertyId} .pd-claimcardinality-${propertyId}`
  return page.locator(`${cardinality} > .pd-claimcardinality-item, ${cardinality} > div > .pd-claimcardinality-item`)
}

// The value input of one slot of a field. The input class says which kind of value the field holds.
export function slotInput(page: Page, propertyId: string, slot: number, input: string): Locator {
  return fieldSlots(page, propertyId).nth(slot).locator(input)
}

// The API a slot of the edit form posts a change to.
const SAVE_CHANGE_API = "/api/d/saveChange/"

// The promise of the next change the edit form posts into the editing session. A slot posts what was typed
// into it when the focus leaves it, and the post travels through a queue, so a step which depends on the
// session holding the value (a save, above all, which acts on the claims the session holds and not on what
// the inputs show) has to wait for the post itself. The form settling around it says nothing about it. Ask
// for the promise before the action which commits the slot, and await it after.
//
// Only call this for an action which changes a value: a slot which is filled with what it already holds
// posts nothing, and the wait would then run into its timeout.
export function changePosted(page: Page): Promise<Response> {
  return page.waitForResponse((response) => response.url().includes(SAVE_CHANGE_API) && response.request().method() === "POST", { timeout: LOADING_TIMEOUT })
}

// Fills one slot of a string or identifier field and waits until the form has settled on the number of slots
// the field is expected to show afterwards, and until the change has reached the session. Filling the
// trailing empty slot of a field grows it by a fresh empty one, while overwriting a slot which already holds
// a value leaves the count alone. What is passed as "what" names the slot in every assertion, so a failure
// says which field of the form it is about.
export async function fillSlot(page: Page, propertyId: string, slot: number, input: string, value: string, slots: number, what: string): Promise<void> {
  const filled = slotInput(page, propertyId, slot, input)
  await expect(filled, what).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await filled.fill(value)
  await filled.blur()
  await expect(filled, `${what} after typing`).toHaveValue(value)
  await expect(fieldSlots(page, propertyId), `slots of ${what} after typing`).toHaveCount(slots)
  await posted
}

// Clears the console errors which asking for an address the server refuses provokes on purpose, so that a
// checkpoint taken afterwards is about the view it is of. A document which was deleted and one the caller
// may not read are both answered with a status the browser reports as a failed resource whatever the
// application then does with it, and the application logs the failed fetch on top of that. A checkpoint
// fails a test on any console error, so both have to be cleared. Call this only right after such a step, so
// that an error logged anywhere else still fails the test.
export function clearRefusedRequestErrors(page: Page): void {
  clearConsoleErrors(page)
}

// Saves the open edit session, waits for the document view to come back and returns the identifier the
// document was saved under.
//
// Focus is moved out of whatever was edited last, onto the discard button next to save: an input writes
// its value into the editing session when it loses focus, and the save button stays disabled until the
// session has something to save, so clicking save straight after typing would wait on a disabled button
// instead of committing the value.
export async function saveEdit(page: Page, options: DocumentViewOptions = {}): Promise<string> {
  const discardButton = page.locator("#documentedit-button-discard")
  await expect(discardButton).toBeVisible()
  await discardButton.focus()
  const saveButton = page.locator("#documentedit-button-save")
  await expect(saveButton).toBeEnabled()
  await saveButton.click()
  await settleDocument(page, options)
  return documentId(page)
}

// Discards the open edit session without saving.
export async function discardEdit(page: Page): Promise<void> {
  const discardButton = page.locator("#documentedit-button-discard")
  await expect(discardButton).toBeVisible()
  await discardButton.click()
  await settleDocument(page)
}

// Starts creating a document of the given class. The create view lists the classes as a tree and a
// class is chosen by clicking it, so the class is addressed by the identifier in its class name.
//
// A class which is a subclass of more than one class is listed once under each of them, so its button
// matches more than once and the first match is taken.
export async function startCreate(page: Page, classId: string): Promise<void> {
  const createButton = page.locator(".pd-createbutton")
  await expect(createButton).toBeVisible()
  await createButton.click()
  await expect(page.locator(".pd-documentcreate")).toBeVisible({ timeout: LOADING_TIMEOUT })
  const classButton = page.locator(`.pd-classtreelabel-button-${classId}`).first()
  await expect(classButton, `class button of ${classId}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  await classButton.click()
  await settleEdit(page)
}

// Drives a reference input the way a user does: type a query, wait for the results, pick one. Used
// both for picking the class of a new document and for filling any reference field.
export async function selectRef(page: Page, inputSelector: string, query: string): Promise<void> {
  const input = page.locator(inputSelector)
  await expect(input).toBeVisible()
  await input.fill(query)
  const firstResult = page.locator(".pd-inputref-item").first()
  await expect(firstResult).toBeVisible()
  await firstResult.click()
}

// How long one search of a reference input is given to offer the wanted document, and how many times the
// query is typed again before the search is given up on. The two together are the time a document a test
// has just saved is given to reach the search index, which tests writing next to each other make slower.
const REFERENCE_ATTEMPT_TIMEOUT = 5000
const REFERENCE_ATTEMPTS = 24

// Whether the wanted candidate of a reference search turned up within the time one search is given.
async function offered(candidate: Locator): Promise<boolean> {
  try {
    await candidate.waitFor({ state: "visible", timeout: REFERENCE_ATTEMPT_TIMEOUT })
    return true
  } catch {
    return false
  }
}

// What pickReference may be asked to do beyond picking the document.
export interface PickReferenceOptions {
  // Whether the typed query, the offered results and the picked reference are checkpointed. What is passed
  // as "what" names those checkpoints. Without it no checkpoint is taken at all, which is what a test using
  // a reference field to get somewhere rather than to look at it asks for.
  checkpoints?: boolean
}

// Drives a reference input the way a user does: type a query, wait for the wanted document among the
// results, pick it. The wanted document is named by its identifier rather than taken by rank, because the
// ranking rests on term statistics and is not the same from one run to the next. What is passed as "what"
// names the input in every assertion, so a failure says which field of the form it is about.
//
// A reference input searches once per query it is given and never repeats the search on its own, so a
// document which the search index has not caught up with yet would never turn up however long a single
// search is waited for. The query is therefore typed again on every attempt, which is what a test picking a
// document it has just created needs. The first attempt only types, so a test which checkpoints the query
// sees the input the way a user leaves it.
export async function pickReference(page: Page, scope: Locator, query: string, documentId: string, what: string, options: PickReferenceOptions = {}): Promise<void> {
  const input = scope.locator(".pd-inputref-input").first()
  await expect(input, `reference input for ${what}`).toBeVisible()
  const wanted = scope.locator(`.pd-inputref-item-${documentId}`)

  let found = false
  for (let attempt = 0; attempt < REFERENCE_ATTEMPTS && !found; attempt++) {
    if (attempt > 0) {
      // The query is emptied before it is typed again, because filling an input with what it already holds
      // is no change of its value and would run no search at all.
      await input.click()
      await input.fill("")
    }
    await input.fill(query)
    await expect(input, `reference input for ${what} holds the typed query`).toHaveValue(query)
    if (attempt === 0 && options.checkpoints) {
      await checkpoint(page, `${what}-query`)
    }
    found = await offered(wanted)
  }
  expect(found, `wanted result for ${what}`).toBe(true)
  // Every result also links to the document it stands for, so the user can check it before picking it.
  await expect(scope.locator(".pd-inputref-link-result").first(), `link of the first result for ${what}`).toBeVisible()
  if (options.checkpoints) {
    await checkpoint(page, `${what}-results`)
  }

  // The pick is no change of its own: the input writes what was picked into the slot's local state, which
  // the slot posts into the session when the focus leaves it (onSlotFocusOut in ClaimInput.vue), the same
  // way a typed value is posted. A step which needs the session to hold the pick has therefore to take the
  // focus out of the slot first, which pressing Save does by itself.
  await wanted.click()
  // The picked reference replaces the search input with the document's label and a clear button, so the two
  // of them appearing is what says the pick landed.
  await expect(scope.locator(".pd-inputref-value").first(), `picked reference for ${what}`).toBeVisible()
  await expect(scope.locator(".pd-inputref-button-clear").first(), `clear button after picking ${what}`).toBeVisible()
  // What is waited for is that the picked document has resolved, and not that the edit form has settled: a
  // reference input is also offered by the claim editor of the all properties tab, which renders no form at
  // all, so waiting for one there would wait until the test times out.
  await expectNothingLoading(page)
  if (options.checkpoints) {
    await checkpoint(page, `${what}-picked`)
  }
}

// What createNamed may be asked to do beyond filling in a name.
export interface CreateNamedOptions {
  // The prefix of the names of the checkpoints taken while the document is created. Without it no checkpoint
  // is taken at all, which is what a test wanting nothing but the document itself asks for.
  checkpointPrefix?: string
  // Whether the empty create form is checkpointed as a whole page before anything is filled in, next to the
  // checkpoint of the name field which the prefix already asks for.
  checkpointEmptyForm?: boolean
}

// Creates a document of the given class with nothing but the given naming property filled in, which is all
// a class whose other fields are optional needs in order to be saved, and returns the identifier it was
// saved under. The browser is left on the document view of the created document.
//
// The panel of potential duplicates is hidden for as long as the form is open: it searches the index for
// documents which structurally match the one being created, so from the second run on it lists the document
// an earlier run created, which moves everything below it and makes a checkpoint differ between runs.
export async function createNamed(page: Page, classId: string, propertyId: string, name: string, options: CreateNamedOptions = {}): Promise<string> {
  await startCreate(page, classId)
  await hideDuplicates(page)
  if (options.checkpointPrefix !== undefined && options.checkpointEmptyForm) {
    await checkpoint(page, `${options.checkpointPrefix}-create-form`)
  }

  const nameField = field(page, propertyId)
  const nameInput = nameField.locator(".pd-inputstring").first()
  await expect(nameInput, `name input of the new ${classId}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  const posted = changePosted(page)
  await nameInput.fill(name)
  await nameInput.blur()
  await expect(nameInput, `name input of the new ${classId} holds the entered name`).toHaveValue(name)
  // The save below acts on the claims the session holds, so the name has to have reached it: a save which
  // goes ahead of the post creates a document without a name.
  await posted
  if (options.checkpointPrefix !== undefined) {
    await checkpointElement(page, nameField, `${options.checkpointPrefix}-name`)
  }

  const id = await saveEdit(page)
  await expect(page.locator("#documentget-title"), `title of the created ${classId}`).toHaveText(name, { timeout: LOADING_TIMEOUT })
  return id
}

// Opens the search filters panel, which is collapsed at narrow widths and on some views.
export async function openFilters(page: Page): Promise<void> {
  const panel = page.locator(".pd-searchresultsfeed-panel-filters")
  if (!(await panel.isVisible())) {
    const toggle = page.locator(".pd-searchresultsfeed-button-filters")
    await expect(toggle).toBeVisible()
    await toggle.click()
  }
  await expect(panel).toBeVisible()
}

// Expands a filter block which shows only its first few values, so the rest can be asserted on.
export async function expandFilter(page: Page, block: Locator): Promise<void> {
  const more = block.locator(".pd-filtersresult-more")
  if (await more.isVisible()) {
    await more.click()
  }
}

// One facet of the filters panel, addressed by the hook it carries: its kind followed by the identifiers of
// the property path it filters on. A nested facet passes both properties of its path.
export function filter(page: Page, kind: "ref" | "has" | "time" | "amount", ...props: Array<string>): Locator {
  return page.locator([".pd-filtersresult", kind, ...props].join("-"))
}

// The checkbox of one value of a facet, addressed by the id the facet gives it, which is the kind, the
// property path and the identifier of the value document joined by slashes.
export function filterValue(page: Page, kind: "ref" | "has", props: Array<string>, value: string): Locator {
  return page.locator(`[id="${[kind, ...props, value].join("/")}"]`)
}

// Runs an action which changes the search from inside the result page, and waits until what the page shows
// is the result of the changed search.
//
// Nothing the page renders can be waited for instead. The results header goes on reporting the count of the
// previous search for as long as the new one is in flight, because useSearchResults replaces its total only
// once the new results are in and does not clear it while it fetches them, and the feed goes on showing the
// results that count belongs to. A control which was just used says nothing either: it renders state of the
// search session rather than of the search, so it takes the change the moment it is clicked. Waiting for the
// network to go idle does not bridge the two: the fetch is issued a few ticks after the click, and until it
// is the page is idle, so that wait can be over before the search has even started. The response of the
// search itself is therefore what is waited for.
export async function searchAgain(page: Page, action: () => Promise<void>): Promise<void> {
  const results = page.waitForResponse((response) => response.url().includes(SEARCH_RESULTS_API), { timeout: LOADING_TIMEOUT })
  await action()
  await results
  await expectResults(page)
}

// Selects a value of a facet and waits until the search it started has come back. A facet grows a clear
// button only once its filter is active, so that is what tells a click which took effect apart from one
// which has not been applied yet.
export async function applyFilterValue(page: Page, facet: Locator, value: Locator): Promise<void> {
  // The values of a facet are fetched per facet rather than coming with the panel, so a facet card is on
  // the page before the values it offers are, and waiting for one of them is waiting for an answer.
  await expect(value, "value of the facet to select").toBeVisible({ timeout: LOADING_TIMEOUT })
  // Every control of the view is disabled while a request is in flight (useLocked in PeerDB), and the filters
  // panel keeps fetching after the results have arrived, so a value which is on the page is not yet a value
  // which can be ticked.
  await expect(value, "value of the facet to select is not locked").toBeEnabled({ timeout: LOADING_TIMEOUT })
  await searchAgain(page, async () => {
    await value.click()
    const cleared = facet.locator(".pd-filtersresult-button-clear")
    await expect(cleared, "the facet whose value was selected offers to be cleared").toBeVisible({ timeout: LOADING_TIMEOUT })
  })
}

// Opens the sort and grouping dialog.
export async function openSortDialog(page: Page): Promise<void> {
  const sortButton = page.locator(".pd-searchresultsheader-button-sort")
  await expect(sortButton).toBeVisible()
  await sortButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel")).toBeVisible()
}

// What a request made from inside the page reports back about the response it got.
export interface FetchedResponse {
  status: number
  headers: Record<string, string>
  body: string
  length: number
}

// Requests a path of this instance from inside the page and reports what the server answered, whatever the
// status is, so that a test can tell an address which is refused from one which is served.
//
// The request is made by the page and not by the test's own request context, which shares neither the
// browser's view of the certificate the site is served with, nor its origin, nor its cookies, the last of
// which is what makes the request carry the session of the view next to it. The page has to be on the site
// already for a request to a path to be same origin.
//
// The body is reported both as text and as the number of bytes it carried, so that an answer meant to be
// read as text can be read and a file can be checked against its size on disk, which decoding into text
// would change.
export async function fetchFromPage(page: Page, path: string): Promise<FetchedResponse> {
  return await page.evaluate(async (requested) => {
    const response = await fetch(requested)
    const buffer = await response.arrayBuffer()
    return {
      status: response.status,
      headers: Object.fromEntries(response.headers.entries()),
      body: new TextDecoder().decode(buffer),
      length: buffer.byteLength,
    }
  }, path)
}
