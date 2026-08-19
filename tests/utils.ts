import type { BrowserContext, Locator, Page, Response } from "@playwright/test"
import type { Result } from "axe-core"

import AxeBuilder from "@axe-core/playwright"
import { test as baseTest } from "@playwright/test"
import { Identifier } from "@tozd/identifier"
import { createHtmlReport } from "axe-html-reporter"
import serialize from "canonicalize"
import { createHash } from "node:crypto"
import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, statSync, writeFileSync } from "node:fs"
import { basename } from "node:path"

// Allowed console message patterns.
const CONSOLE_ALLOWLIST = [
  /^Failed to load resource: the server responded with a status of 400 \(\)$/,
  /\[vite]/,
  /\[Vue/,
  /was preloaded using link preload in Early Hints but not used/,
]

// How long a menu is given to open, and a button inside one to answer, before the press is made again.
// It is short because what it waits for is a render and not an answer from the site.
const MENU_TIMEOUT = 2000

export const PEERDB_URL = process.env.PEERDB_URL || "https://localhost:8080"

// The identifier a document is stored under, derived the way the application derives it: from the namespace
// the document lives in and the parts of its base which say which document of that namespace it is. A test
// names a document by those parts instead of by the opaque string they hash to, and each application wraps
// this with the namespaces of its own (see documentIdOf in its utils).
export async function identifierOf(namespace: string, ...base: Array<string>): Promise<string> {
  return (await Identifier.from(namespace, ...base)).toString()
}

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
  // The element to capture instead of the page. What is captured is the element itself and not a region of
  // the page measured from it, so a page which moves between the moment the capture is asked for and the
  // moment it is taken cannot leave the capture framed on somewhere else.
  of?: Locator
  // Whether the checks which read the whole page are run: the duplicate identifiers and the accessibility
  // violations. They are about the page and not about the screenshot, and both walk the whole document, so a
  // test which captures the same page many times over (one screenshot per step of a movement, say) runs them
  // on its first capture and turns them off for the rest instead of deriving the same answer every time. The
  // console is read whatever this says, because the messages are collected as they arrive and reading them
  // costs nothing.
  checks?: boolean
  // Whether the capture is of a view with an operation still running. A capture is normally of a view at
  // rest, so the default waits for what is in flight to land before anything is captured. A caller which
  // drives an operation and captures the view while it runs says so here, because the answer which has not
  // arrived is exactly what it is capturing, and waiting for it would capture the view after the operation
  // or, where the answer takes the captured element off the page, not at all.
  running?: boolean
}

// How long one screenshot may take. The action timeout the rest of the suite runs under covers waiting for
// an element to appear, while capturing a full page is work whose cost grows with the page: a result page
// with every facet shown is several times the height of the viewport, and encoding it takes longer than
// waiting for anything on it ever should.
const SCREENSHOT_TIMEOUT = 60000

// Take up to 10 screenshots, wait until they stabilize. We had issues (and flakiness) because sometimes
// screenshots are not saved fully (just part of the page is visible, the rest is blank). Now we wait
// visually for screenshot to stabilize (instead of waiting just for DOM). What one attempt captures is
// decided by the caller through shoot, so the same waiting covers a page and an element alike.
async function takeStableScreenshot(page: Page, name: string, shoot: () => Promise<Buffer>): Promise<Buffer> {
  let olderScreenshot = await shoot()
  for (let i = 0; i < 10; i++) {
    await page.waitForTimeout(500)
    const newerScreenshot = await shoot()
    if (olderScreenshot.equals(newerScreenshot)) {
      return newerScreenshot
    }
    olderScreenshot = newerScreenshot
  }
  throw new Error(`unable to take stable screenshot: ${name}`)
}

// Waits until the page has everything it asked for. The bar across the top is drawn while any request is in
// flight, so its absence says both that the page is done and that whatever the answers still have to change
// on it has been changed. What is passed as "what" names the moment this is waited for, so a failure says
// which step of a checkpoint was waiting.
async function expectNothingInFlight(page: Page, what: string): Promise<void> {
  await expect(page.locator(".pd-navbar-progress"), `the progress bar while ${what}`).toHaveCount(0, { timeout: LOADING_TIMEOUT })
}

// Takes a screenshot of the page (or of the given element of it) and compares it with the one stored under
// the given name, then checks the page for duplicate identifiers, accessibility violations and console errors.
export async function checkpoint(page: Page, name: string, { mask = [], fullPage = true, of, checks = true, running = false }: CheckpointOptions = {}): Promise<void> {
  // A screenshot which catches the progress bar is a screenshot of a page which is not done. It is waited out
  // here rather than at the call sites because it can be lit by anything the page is doing, and it sits over
  // the top of the navbar, which is inside an element screenshot of the navbar just as much as it is on a
  // whole page. A capture of a view with an operation running is the one case where it is not waited out.
  if (!running) {
    await expectNothingInFlight(page, `taking ${name}`)
  }
  // Anchor scroll to the top so position:fixed elements land at the top of fullPage screenshots. An element
  // capture starts from there as well: Playwright scrolls the element into view itself, and starting every
  // capture from the top of the page is what makes it scroll the same way twice.
  if (of !== undefined || fullPage) {
    await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }))
  }
  // Move mouse to the same location so the same element gets focused every time.
  await page.mouse.move(0, 0)
  const screenshotPath = test.info().snapshotPath(`${name}.png`, { kind: "screenshot" })
  // A screenshot which has nothing stored under its name yet is written to where the comparison would have
  // read it from, so that the run which adds a checkpoint is also the one which gives it its stored copy.
  const isNew = !existsSync(screenshotPath)
  const screenshotOptions = {
    mask,
    timeout: SCREENSHOT_TIMEOUT,
    ...(isNew ? { path: screenshotPath } : {}),
  }

  const screenshotBuffer = await takeStableScreenshot(page, name, () =>
    of !== undefined ? of.screenshot(screenshotOptions) : page.screenshot({ ...screenshotOptions, fullPage }),
  )

  if (isNew) {
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

  if (!checks) {
    await expectNoConsoleErrors(page)
    return
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

  // The first capture is the one which reads the whole page: what the duplicate identifiers and the
  // accessibility scan look at is the same page for every entry below, so the rest ask for the screenshot
  // alone. The console is read for every one of them, so an error raised while a later entry is captured
  // is still reported.
  let firstEntry = true

  for (let i = 0; i < count; i++) {
    const entry = entries.nth(i)
    // An entry which is not rendered is passed over rather than waited for: what is captured here is the
    // entries the page shows.
    if ((await entry.boundingBox()) === null) {
      continue
    }

    const displayNameElement = entry.locator(displayNameSelector)
    const displayName = (await displayNameElement.textContent())?.replace(/\s/g, "")

    await checkpointElement(page, entry, `${screenshotPrefix}-${displayName}`, { mask, checks: firstEntry })
    firstEntry = false
  }
}

interface CheckpointElementOptions extends Pick<CheckpointOptions, "mask" | "checks"> {
  // Whether the element is captured with something in it locked. A capture is normally of an element at
  // rest, so the default is to wait for the locked controls to be released, and a caller which drives an
  // operation and captures the element while it runs says so here, which waits for the lock instead.
  locked?: boolean
}

// The controls which an operation of the view they are in has made inactive. The class is carried by every
// control which renders that state, and by the control itself rather than by a wrapper of it, so an element
// which is a control on its own is asked about as well as the controls inside it.
const LOCKED_CONTROLS = ":scope.pd-locked, .pd-locked"

// Takes a checkpoint of one element only, so a regression in the part of the view which was just driven
// is reported against a screenshot of that part rather than only against the whole page. What it adds to
// a checkpoint of the element is the waiting: the element has to be there, the page has to have everything
// it asked for, and what is in the element has to be in the state the caller names.
export async function checkpointElement(
  page: Page,
  locator: Locator,
  name: string,
  { mask = [], locked = false, checks = true }: CheckpointElementOptions = {},
): Promise<void> {
  await expect(locator, `element for ${name}`).toBeVisible()

  // An element captured with something in it locked is an element captured while an operation runs, so what
  // is in flight is not waited out for it: it is the operation being captured. Everything else is captured at
  // rest, because an element captured while an answer is on its way is captured before what the answer
  // changes in it has been changed, which is a state the name does not describe.
  if (!locked) {
    await expectNothingInFlight(page, `taking the element for ${name}`)
  }

  // A locked control is drawn greyed and does not accept anything, so which of the two states it is in
  // decides what the screenshot shows. The progress bar does not answer this on its own: a lock can be held
  // by work which fetches nothing, a validation pass of the form being one, so it can be raised and released
  // without the bar ever lighting. Waiting for the state the caller says it wants makes both reproducible.
  const lockedControls = locator.locator(LOCKED_CONTROLS)
  if (locked) {
    await expect(lockedControls, `a locked control in the element for ${name}`).not.toHaveCount(0, { timeout: LOADING_TIMEOUT })
  } else {
    await expect(lockedControls, `the locked controls in the element for ${name}`).toHaveCount(0, { timeout: LOADING_TIMEOUT })
  }

  await checkpoint(page, name, { mask, checks, of: locator, running: locked })
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

// The API route a change to the search session is posted to. Its answer says which version the session is at
// after the change, and the results of that version are fetched under that version, which is how a change is
// followed from the click to the results it produced.
const SEARCH_UPDATE_API = "/api/s/update/"

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

// The panel of the document view which renders the fields the document's class declares, which is the tab the
// view opens on.
export function fieldsPanel(page: Page): Locator {
  return page.locator(".pd-documentget-panel-properties")
}

// Screenshots the part of the form which the shortcut filled in, with the page scrolled so that the part
// sits at the top of the window.
//
// The window is captured rather than the whole page. The editor of a class whose fields are grouped into
// sections opens on an address ending in an anchor to the first section, and capturing a whole page makes
// the browser lay the page out again, which sends it to that anchor a second time. That happens between one
// capture and the next, so a whole page capture of such a form catches it either before or after the jump
// depending on how quickly the capture runs. A window capture lays nothing out again.
export async function checkpointFormAt(page: Page, locator: Locator, name: string): Promise<void> {
  await expect(locator, `the part of the form for ${name}`).toBeVisible()
  await locator.evaluate((element) => element.scrollIntoView({ block: "start", behavior: "instant" }))
  await checkpoint(page, name, { fullPage: false })
}

// The parts of a view which do not look the same on every run, and which every checkpoint therefore
// masks. Next to the times the shared helper masks, a reference field whose candidates all fit is
// rendered as a list of options which comes from a search of the index, and the order in which a search
// returns the documents a filter alone matches is not stable while the suite is writing documents, so
// such a list is masked rather than compared.
export function volatileSelect(page: Page): Array<Locator> {
  return [...volatile(page), page.locator(".pd-claimrefselect-list")]
}

// Where the address a stored file is served under is rendered: the value box of the image file field on the
// form, and the value cell holding the link on the saved document. A stored file gets a fresh identifier
// every time one is uploaded, so a screenshot showing that address would differ between runs. Pass these to
// the mask option of a checkpoint.
//
// The two boxes are masked rather than the link itself, because a link is only as wide as the address it
// holds, so the mask would move with the address it is there to hide, while both boxes are as wide as the
// layout makes them. A file attached to the notes is linked by its name and needs no masking.
export function volatileFileLinks(page: Page): Array<Locator> {
  return [page.locator(".pd-inputfile-value"), page.locator(".pd-fieldsview-value:has(.pd-claimvaluelink)")]
}

// Takes a checkpoint of the fields form only. While a document is being created the view also shows
// the panel of its potential duplicates, which from the second run on lists the document an earlier
// run created, so a full page screenshot of the create view would differ between runs. The form
// itself sits above that panel and is not moved by it, so a screenshot of the form alone is the same
// on every run.
export async function checkpointFields(page: Page, name: string): Promise<void> {
  await checkpointElement(page, page.locator(".pd-fieldsform"), name)
}

// The values the class tab of the document view shows, in the order the class gives its fields, which for
// an art discipline is the name and then the code.
export function documentValues(page: Page): Locator {
  return page.locator(".pd-documentget-panel-properties .pd-fieldsview-value")
}

// The row of the class tab of the document view which is for the given property, addressed by the property
// the row carries, so a row is found without depending on its label. The all properties tab has a row per
// property as well, which propertyRow addresses.
export function fieldRow(page: Page, propertyId: string): Locator {
  return page.locator(`.pd-documentget-panel-properties .pd-fieldsview-row-${propertyId}`)
}

// The value cells of the row of the class tab which is for one property. A label is rendered once per field
// and a value once per claim, so a property stated more than once has one label and several of these.
export function fieldValues(page: Page, propertyId: string): Locator {
  return fieldRow(page, propertyId).locator(".pd-fieldsview-value")
}

// The print view shows a timestamp which ticks every second, so it never looks the same twice and has to be
// masked together with the rest of the volatile content.
export function printVolatile(page: Page): Array<Locator> {
  return [...volatile(page), page.locator(".pd-searchresultsfeed-timestamp")]
}

// The controls of the home view: the box a query is typed into and the button which sends it. Landing on
// the address of a site which serves that view is the box being there.
const HOME_SEARCH_INPUT = "#home-input-search"
const HOME_SEARCH_BUTTON = "#home-button-search"

// Where a site is searched from, for a site which does not serve the home view at its address. A site whose
// front page is a search over its catalog has no home view at all, so it says which element says its address
// has landed and which controls it is searched through, instead of the home view's.
export interface SiteSearchOptions {
  // What says the address of the site has landed. Defaults to the search box of the home view.
  landed?: string
  // Where a query is typed and what sends it. Both default to the controls of the home view.
  input?: string
  button?: string
}

// Opens the address of the site and waits until what it leads to has rendered. Every test starts here so
// that the navbar is in its initial state.
export async function goHome(page: Page, { landed = HOME_SEARCH_INPUT }: SiteSearchOptions = {}): Promise<void> {
  await page.goto(PEERDB_URL)
  await expect(page.locator(landed), "what the address of the site leads to").toBeVisible()
}

// The name the mock authenticator gives the user signed in with the given roles: the roles in
// alphabetical order, appended to its own name (mockUsername in auth/mock.go).
export function mockUsername(roles: ReadonlyArray<string>): string {
  return ["mock", ...[...roles].sort()].join("-")
}

// How much of the text of an unexpected page a sign-in which did not land reports, which is plenty for the status text an error page carries.
const SIGN_IN_PAGE_TEXT_LENGTH = 200

// What stands where the menu of the signed-in user was expected, so that a sign-in which did not land says what happened instead of only that an
// element is missing. The application is served for both of the pages a sign-in passes through, so those two are told apart by what they render,
// and anything else is the server answering the callback with a refusal, whose body is the plain text of the status it was refused with.
async function signInPageState(page: Page): Promise<string> {
  const menu = page.locator(".pd-navbarmenu-button")
  if ((await menu.count()) > 0) {
    return `the menu names ${JSON.stringify(((await menu.first().textContent()) ?? "").trim())}`
  }
  if ((await page.locator("#navbar-button-signin").count()) > 0) {
    return "the application is served, to a caller who is not signed in"
  }
  if ((await page.locator(".pd-authmocksignin").count()) > 0) {
    return "the page choosing the roles is still up, so the callback was never made"
  }
  let body = ""
  try {
    body = await page.locator("body").innerText()
  } catch {
    // A page which went away while it was being read says nothing about itself, and it is a failure
    // which is being described here, so reading it must not fail on its own.
  }
  const text = body.replace(/\s+/g, " ").trim()
  if (text === "") {
    return "the page is empty"
  }
  return `the page says ${JSON.stringify(text.slice(0, SIGN_IN_PAGE_TEXT_LENGTH))}`
}

// Signs in through the mock authenticator with exactly the given roles. Passing no role at all signs in
// as a user who holds none, which is what a test asserting that signing in alone grants nothing needs.
//
// The mock stands in for an identity provider: it takes the browser to a page of its own where the
// roles are chosen, and signing in there sends it back where it started. The roles are picked by the
// label they are listed under, which is the role name the site declares, so the order the page lists
// them in does not matter.
export async function signIn(page: Page, roles: ReadonlyArray<string>, options: SiteSearchOptions = {}): Promise<void> {
  await goHome(page, options)
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
  // landed. The sign-in goes through two server-side redirects, so where the browser ended up and what
  // stands there is what a sign-in which did not land is diagnosed from.
  try {
    await expect(page.locator(".pd-navbarmenu-button"), "the menu of the signed-in user").toHaveText(mockUsername(roles))
  } catch {
    throw new Error(`the sign-in as ${mockUsername(roles)} did not land, on ${page.url()}: ${await signInPageState(page)}`)
  }
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
//
// The page is waited out first and the press is repeated until the menu stays open. A navbar renders
// again whenever what it names changes (the answer to what the page asked for lands, the user signs in
// or out), and rendering it again closes the menu, so a press which lands in that moment opens a menu
// which is gone by the time anything in it is reached for.
export async function openUserMenu(page: Page): Promise<void> {
  const menuButton = page.locator(".pd-navbarmenu-button")
  await expect(menuButton).toBeVisible()
  const panel = page.locator(".pd-navbarmenu-panel")
  if (await panel.isVisible()) {
    return
  }
  await settle(page)
  await expect
    .poll(
      async () => {
        if (await panel.isVisible()) {
          return true
        }
        await menuButton.click()
        // The press is given time to land before it is judged: a menu which took a moment to open is not
        // a menu which did not open, and pressing again would close the one which just did.
        return await panel.isVisible({ timeout: MENU_TIMEOUT })
      },
      { message: "the menu of the signed-in user is open", timeout: LOADING_TIMEOUT },
    )
    .toBe(true)
}

// Signs the user out, through the button inside their own menu.
//
// The menu is opened again for every attempt, because it closes whenever the navbar renders again, which
// it does while the page the sign-out starts from is still being answered. What says the sign-out landed
// is the button which signs in being back.
export async function signOut(page: Page): Promise<void> {
  const signInButton = page.locator("#navbar-button-signin")
  await expect
    .poll(
      async () => {
        if (await signInButton.isVisible()) {
          return true
        }
        await openUserMenu(page)
        const signOutButton = page.locator("#navbar-button-signout")
        try {
          await signOutButton.click({ timeout: MENU_TIMEOUT })
        } catch {
          // The menu closed under the press, which the next attempt opens again.
          return false
        }
        return await signInButton.isVisible({ timeout: MENU_TIMEOUT })
      },
      { message: "the user is signed out", timeout: LOADING_TIMEOUT },
    )
    .toBe(true)
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
export interface SearchWithQueryOptions extends SiteSearchOptions {
  // Whether the home page before the search and the results after it are checkpointed. Without it no
  // checkpoint is taken at all, which is what a test about what a search finds rather than about how the
  // page looks asks for.
  checkpoints?: boolean
  // Whether the search is expected to have found something. A search which found nothing renders no result
  // at all and is waited for differently, see expectNoResults. Defaults to true.
  results?: boolean
}

// Runs a search from the front page of the site and waits for the results to render.
export async function searchWithQuery(page: Page, query: string, options: SearchWithQueryOptions = {}): Promise<void> {
  await goHome(page, options)

  const searchInput = page.locator(options.input ?? HOME_SEARCH_INPUT)
  await expect(searchInput, "the search box the query is typed into").toBeVisible()
  const searchButton = page.locator(options.button ?? HOME_SEARCH_BUTTON)
  await expect(searchButton, "the button which sends the query").toBeVisible()
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

export function searchResults(page: Page): Locator {
  return page.locator(".pd-searchresult")
}

export function loadMoreButton(page: Page): Locator {
  return page.locator("#searchresultsfeed-button-loadmore")
}

// Clicks a navbar element which starts or updates a search session and waits until the new session
// has rendered. Every such click ends in a session of its own, so waiting for the location to change
// is what tells the old results from the new ones.
export async function clickIntoSearch(page: Page, selector: string): Promise<void> {
  const element = page.locator(selector)
  await expect(element).toBeVisible()
  const before = page.url()
  await element.click()
  await page.waitForFunction((url) => window.location.href !== url, before)
  await expectResults(page)
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
  const ids = await read()

  // Every card records the address it read its document from, and it has to be the document the card links
  // to: a card which links to one document while rendering another is a card a reader cannot trust, and the
  // two come from different places in the view, so nothing else holds them together.
  const fetched = await page.locator(".pd-searchresult").evaluateAll((cards) => cards.map((card) => card.getAttribute("data-url")))
  expect(fetched, "every result names the address it read its document from").toEqual(ids.map((id) => `/api/d/${id}`))

  return ids
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

// The panel of the feed which lists the facets. It carries the address its list came from, which names the
// version of the search the list is for.
const FILTERS_PANEL = ".pd-searchresultsfeed-panel-filters"

// The version of the search the given address was asked for, or null when there is no address or it names no
// version.
function versionOfURL(url: string | null): string | null {
  return url === null ? null : new URL(url, PEERDB_URL).searchParams.get("version")
}

// The version of the search the given element last asked for, or null when the element is not on the page (a
// view which renders no such element) or carries no address yet.
async function versionOfElement(page: Page, selector: string): Promise<string | null> {
  const element = page.locator(selector).first()
  if ((await element.count()) === 0) {
    return null
  }
  return versionOfURL(await element.getAttribute("data-url"))
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
  // One header per facet, whatever kind of facet it is, so this counts what the panel is showing.
  const facets = page.locator(".pd-filtersresult-header")
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
      //
      // How many facets it is showing is confirmed along with the button, because a list which arrives takes
      // the panel back to the facets it shows first, and it does so while re-rendering rather than over the
      // network: the button is briefly away in the middle of that, which on its own reads the same as a panel
      // with nothing left to add. A panel taken back to its first facets shows fewer of them than the one
      // which was settled, so the count is what tells the two apart.
      //
      // Which version the list is for is confirmed as well, because a panel showing every facet of the
      // version before is a settled panel by every other measure. The panel builds its list from the search
      // session it has been handed, which is the session as it was last read back, so it goes on showing the
      // list of the version before while the results are already those of the new one, with nothing left to
      // add and nothing to say that a list is on its way. The list which then arrives takes the panel back
      // to the facets it shows first, which is what a capture taken in the meantime shows. Both the panel
      // and the results publish the address their contents came from rather than the one last asked for
      // (see useFilters and useSearchResults in search.ts), so the versions in them agree exactly when what
      // is on the screen belongs together.
      const fetched = filtersFetched
      const shown = await facets.count()
      await settle(page)
      const resultsVersion = await versionOfElement(page, RESULTS_ELEMENT)
      const filtersVersion = await versionOfElement(page, FILTERS_PANEL)
      const current = resultsVersion === null || filtersVersion === null || resultsVersion === filtersVersion
      if (filtersFetched === fetched && (await facets.count()) === shown && !(await moreFilters.isVisible().catch(() => false)) && current) {
        return
      }
    }
    throw new Error(`the filters panel never came to rest, its facets were last fetched from ${lastFiltersURL}`)
  } finally {
    page.off("response", onResponse)
  }
}

// Waits until a facet is back on the page after the search it belongs to has changed. A search which changed
// takes the panel back to the facets it shows first, so a facet beyond them is off the page until the rest of
// them are asked for again. Without this an assertion which passes on a facet not being there (that it offers
// nothing to clear, for one) would pass while the facet is merely missing, and one about what the facet says
// would fail for the same reason. The panel is asked for the rest of its facets until the facet turns up,
// because the collapse happens once the changed search comes back and can therefore be later than the press.
export async function expectFacetBack(page: Page, facet: Locator): Promise<void> {
  await expect
    .poll(
      async () => {
        await settleFilters(page)
        return await facet.count()
      },
      { message: "the facet is on the page again after the search changed", timeout: LOADING_TIMEOUT },
    )
    .toBeGreaterThan(0)
  await expect(facet, "the facet is on the page again after the search changed").toBeVisible()
}

// Clears one filter through its own clear button, the way a user drops a single filter while keeping the
// others.
export async function clearFilter(page: Page, applied: Locator): Promise<void> {
  const clearButton = applied.locator(".pd-filtersresult-button-clear")
  await expect(clearButton, "the clear button of the applied filter").toBeVisible()
  await searchAgain(page, async () => {
    await clearButton.click()
    await expect(clearButton, "the cleared facet no longer offers to be cleared").toHaveCount(0)
  })
}

// Shows a facet which sits below the ones the panel shows at first: the panel is settled, which adds every
// facet it has and waits for its list to stop being replaced, and the facet is then asserted to be among
// them.
//
// Asking for batches until the facet appears is not enough on its own. The panel starts its batches over
// whenever a new list of facets arrives, so a facet revealed just before one lands is taken away again, and
// what is then waited for is a value of a facet which is no longer on the page.
export async function showFilter(page: Page, facet: Locator, what: string): Promise<void> {
  await settleFilters(page)
  await expect(facet, what).toBeVisible({ timeout: LOADING_TIMEOUT })
}

// Screenshots one facet instead of the whole page, so that a change in the facet is reported as such rather
// than as a change somewhere in a very tall page. The panel is settled first, because a facet still waiting
// for the ones above it to load would be framed at the wrong place.
export async function checkpointFacet(page: Page, name: string, facet: Locator): Promise<void> {
  await settleFilters(page)
  await checkpointElement(page, facet, name)
}

// The digits of a rendered count, so that a count can be compared without depending on how the locale
// groups thousands. Counts are rendered both in the results header and next to a facet or one of its rows.
export async function countDigits(locator: Locator): Promise<string> {
  await expect(locator, "the element rendering a count").toBeVisible()
  return ((await locator.textContent()) || "").replace(/\D/g, "")
}

// Waits until the results header reports the given number of results. The header updates only once the
// filtered search comes back, so this is polled rather than read once.
export async function expectResultsCount(page: Page, digits: string): Promise<void> {
  const count = page.locator(".pd-searchresultsheader-count-results")
  await expect(count, "the results header").toBeVisible()
  await expect
    .poll(async () => ((await count.textContent()) || "").replace(/\D/g, ""), { message: `results header reports ${digits} results`, timeout: LOADING_TIMEOUT })
    .toBe(digits)
}

// Waits until the results header reports fewer results than the given count, and returns the new count.
// Narrowing a filter which is already active leaves the panel looking the same while the filtered search is
// in flight, so the header is polled rather than read once: read too early, it still holds the count from
// before the narrowing.
export async function expectFewerResults(page: Page, than: string): Promise<string> {
  const count = page.locator(".pd-searchresultsheader-count-results")
  await expect(count, "the results header").toBeVisible()
  await expect
    .poll(async () => Number(((await count.textContent()) || "").replace(/\D/g, "")), {
      message: `results header reports fewer than ${than} results`,
      timeout: LOADING_TIMEOUT,
    })
    .toBeLessThan(Number(than))
  return await countDigits(count)
}

// Waits until the panel shows the facet's filter as active or as inactive. The clear button is rendered
// exactly when a filter on the facet is part of the search session, so it tells apart a filter which has
// been applied from one whose update is still in flight.
export async function expectFilterActive(page: Page, facet: Locator, active: boolean): Promise<void> {
  const clear = facet.locator(".pd-filtersresult-button-clear")
  if (active) {
    await expect(clear, "the facet whose filter is applied offers to be cleared").toBeVisible({ timeout: LOADING_TIMEOUT })
  } else {
    await expect(clear, "the facet without a filter offers nothing to clear").toHaveCount(0)
  }
  await page.waitForLoadState("networkidle")
}

// Runs the search over all documents from the front page of the site and opens the filters panel. A test
// takes its own checkpoints, so that every screenshot name stays unique even when tests run next to each
// other.
export async function openAllDocumentsSearch(page: Page, options: SiteSearchOptions = {}): Promise<void> {
  await goHome(page, options)

  const searchInput = page.locator(options.input ?? HOME_SEARCH_INPUT)
  await expect(searchInput, "the search box the query is typed into").toBeVisible()
  const searchButton = page.locator(options.button ?? HOME_SEARCH_BUTTON)
  await expect(searchButton, "the button which sends the query").toBeVisible()

  await searchInput.fill("")
  await searchButton.click()

  await expectResults(page)
  await openFilters(page)
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

// Waits until the edit form owes the server nothing: every change it has queued and every change it has in
// flight has settled. The form publishes the count of them on its root element, because the changes go
// through a queue of its own which never lights the page's progress bar, so neither the bar nor the network
// answers this. A form which owes nothing is also a form which shows what it holds: a slot whose change has
// not landed is read-only, which greys the controls of its sub-fields.
//
// This says nothing about a change which is about to be posted, only about the ones the form has taken on,
// so it is what a step which has waited for its own change (see changePosted) adds rather than what it
// replaces.
export async function expectNothingPending(page: Page, timeout: number = LOADING_TIMEOUT): Promise<void> {
  await expect(page.locator(".pd-documentedit"), "changes the form has queued or in flight").toHaveAttribute("data-pending-changes", "0", { timeout })
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

// Waits until the edit view has rendered its form, owes the server nothing, and everything the form shows
// has resolved.
//
// Waiting for the network to go idle instead, the way the other views are waited for, cannot work here:
// the edit view polls its editing session for changes every 100 milliseconds (pollInterval in
// DocumentEdit.vue) for as long as it is open, so its network is never quiet and the wait can only run
// into the test's timeout. What the view renders is waited for instead.
//
// The changes the form has queued or in flight are waited for before what it shows, because a change is
// what makes the view fetch again: a slot whose change has not landed is read-only, which greys the
// controls of its sub-fields, and the document is read back once it has, which sends every label derived
// from it through its loading state again. The page's progress bar answers neither: the form posts its
// changes through a queue of its own which never lights it.
export async function settleEdit(page: Page): Promise<void> {
  await expect(page.locator(".pd-documentedit"), "edit view").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator(".pd-fieldsform"), "fields form").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentedit-button-save"), "save button").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expectNothingPending(page)
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

// How long the index is given to catch up with a write. Indexing runs after the write has been committed,
// so a document becomes searchable, and stops being searchable, some time after the view which wrote it
// has already moved on.
export const INDEXING_TIMEOUT = 60000

// The identifiers a full text search for the given query finds, without asserting that it found anything:
// what a test which waits for the index to catch up with a write is asking is exactly whether the document
// is there yet. Every attempt starts a fresh search session, because a session which has already run keeps
// the result set it found when it ran.
async function searchIdsForQuery(page: Page, query: string): Promise<Array<string>> {
  await page.goto(`${PEERDB_URL}/s?q=${encodeURIComponent(query)}`)
  await settleSearch(page)
  return await resultIds(page)
}

// The document as the server stores it, fetched from the API as text. An editing session keeps its changes
// to itself until the save goes through, so this is how a test tells what a refused save left behind, and
// what one which went through actually wrote. The body is searched as text rather than dug through as a
// claim tree, because that would make the test know the shape of every kind of claim it looks at.
export async function storedDocument(page: Page, id: string): Promise<string> {
  const response = await fetchFromPage(page, `/api/d/${id}`)
  expect(response.status, `status of the stored document ${id}`).toBe(200)
  return response.body
}

// Waits until a search for the given query does or does not find the given document, which is how a test
// waits for the index to catch up with what it wrote.
export async function expectSearchFinds(page: Page, query: string, id: string, found: boolean, what: string): Promise<void> {
  await expect.poll(async () => (await searchIdsForQuery(page, query)).includes(id), { message: what, timeout: INDEXING_TIMEOUT, intervals: [1000] }).toBe(found)
}

// The block of one sub-field inside one slot of a repeated field, which is where the values which hang
// off that slot's own value are edited.
export function subField(slot: Locator, propertyId: string): Locator {
  return slot.locator(`.pd-claimcardinality-${propertyId}`)
}

// The rows of one kind inside one field of the form, for a field which renders a row per value of a claim.
export function slotRows(scope: Locator, property: string, kind: string): Locator {
  return scope.locator(`.pd-claiminput-${property} > div > .pd-fieldsformrow-${kind}`)
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
  // The view records the address it read the document from, which is what says the document on the screen is
  // the one which was asked for rather than one the view was left holding. It is asserted here because every
  // test which opens a document by its identifier comes through here knowing which one it asked for.
  await expect(page.locator(".pd-documentget"), "the document view names the address it read the document from").toHaveAttribute("data-url", `/api/d/${id}`)
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
// document and on what the application registers, so the tab is named by the suffix of its CSS class.
export async function openDocumentTab(page: Page, tab: string): Promise<void> {
  const tabButton = page.locator(`.pd-documentget-tab-${tab}`)
  await expect(tabButton).toBeVisible()
  await tabButton.click()
  await expect(page.locator(`.pd-documentget-panel-${tab}`)).toBeVisible()
  // The tab brings rows of its own, and each of them asks for the labels it names things with, so the panel
  // being there is not the same as the panel being readable. What stands in for a label while it is fetched
  // is drawn without motion, so two captures of it in a row are identical and a capture concludes it is of a
  // page at rest, which is how such a stand-in ends up stored as what the tab looks like.
  await expectNothingLoading(page)
}

// Starts an edit session for the document currently shown and waits for the form.
export async function startEdit(page: Page): Promise<void> {
  const editButton = page.locator("#documentget-button-edit")
  await expect(editButton).toBeVisible()
  await editButton.click()
  await settleEdit(page)
}

// The identifier of the document an editing session is about. A create session allocates the identifier up
// front, so this is how a test names the document of a session which has not been saved yet.
export function editingDocumentId(page: Page): string {
  const match = /\/d\/edit\/([0-9A-Za-z]+)\/[0-9A-Za-z]+(?:[?#]|$)/.exec(page.url())
  expect(match, `the browser is on an editing session: ${page.url()}`).not.toBeNull()
  return match![1]
}

// Discards a create session and waits for the class tree it goes back to. A document which is being created
// exists only inside its session, so a discarded create session has no document view to land on.
//
// Focus is moved onto the button before it is pressed, the same way saveEdit does it: the form focuses its
// first input as soon as it opens, and the blur which the press itself causes commits that input and grows
// the form by whatever the committed value asks for, which moves the button out from under the press.
export async function discardCreate(page: Page): Promise<void> {
  const discardButton = page.locator("#documentedit-button-discard")
  await expect(discardButton, "discard button of the create form").toBeVisible()
  await discardButton.focus()
  await discardButton.click()
  await expect(page.locator(".pd-documentcreate"), "the create page a discarded create session goes back to").toBeVisible({ timeout: LOADING_TIMEOUT })
  await expect(page.locator("#documentcreate-title"), "title of the create page").toBeVisible()
}

// The count a shortcut shows in parentheses after its label. The count is fetched after the document
// renders, so this waits for it to appear before reading it.
export async function shortcutCount(row: Locator): Promise<number> {
  const link = row.locator(".pd-searchshortcutlink-link")
  await expect(link, "the shortcut link shows a count").toHaveText(/\(\d+\)/, { timeout: LOADING_TIMEOUT })
  const text = (await link.textContent()) || ""
  const match = /\((\d+)\)/.exec(text)
  expect(match, `the shortcut link "${text.trim()}" shows a count`).not.toBeNull()
  return Number(match![1])
}

// Presses the "+" of a create shortcut whose limit resolves to a single creatable class, and asserts that
// the press lands in an editing session without the class picker ever being shown.
export async function pressCreateShortcut(page: Page, button: Locator): Promise<void> {
  await expect(button, "the create button of the shortcut").toBeVisible()
  await button.click()
  await settleEdit(page)
  await hideDuplicates(page)
  await expect(page.locator(".pd-classtreelist"), "the class picker of the create view").toHaveCount(0)
  await expect(page.locator("#documentcreate-title"), "the title of the create view").toHaveCount(0)
  await expect(page, "the press landed in an editing session").toHaveURL(/\/d\/edit\/[0-9A-Za-z]+\/[0-9A-Za-z]+/)
}

// The "+" of the shortcut which creates a document of the given class and points the given property at
// the document currently open. The button is picked out by what its link does rather than by where the
// sidebar renders it or by the label it carries, which differs between the two interface languages. Both
// parts of the query are matched separately because the create view sorts its query parameters, so which
// of the two comes first depends on the identifiers.
export function createShortcutButton(page: Page, classId: string, propertyId: string, selfId: string): Locator {
  return page.locator(`.pd-searchshortcutlink-button-create[href*="limit=${classId}"][href*="${propertyId}=${selfId}"]`)
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

// The complaints the form shows on one field. Every value slot renders the error of the input inside it,
// so a field which nothing is wrong with matches no element at all.
export function fieldErrors(page: Page, propertyId: string): Locator {
  return field(page, propertyId).locator(".pd-inputfield-error")
}

// Presses save on the open editing session without waiting for the session to end, which is what a test
// of a save which is refused needs. Focus is moved onto the discard button next to save first, the way
// saveEdit does it, so that the value typed last is offered to the session before the save runs.
export async function pressSave(page: Page): Promise<void> {
  const discardButton = page.locator("#documentedit-button-discard")
  await expect(discardButton, "discard button").toBeVisible()
  await discardButton.focus()
  const saveButton = page.locator("#documentedit-button-save")
  await expect(saveButton, "save button").toBeEnabled()
  await saveButton.click()
}

// The per-entry revert button of one slot of a repeated field, which reverts that entry alone.
export function slotRevert(page: Page, propertyId: string, slot: number): Locator {
  return fieldSlots(page, propertyId).nth(slot).locator(".pd-claimcardinality-button-revert").first()
}

// The value input of one slot of a field. The input class says which kind of value the field holds.
export function slotInput(page: Page, propertyId: string, slot: number, input: string): Locator {
  return fieldSlots(page, propertyId).nth(slot).locator(input)
}

// The input of one slot's own value, without the inputs of the sub-fields which hang off it. A field
// whose entries carry sub-fields renders their inputs inside the same slot, and a sub-field of the same
// kind as the field itself would otherwise be matched just as well, so the value block of the slot's
// own claim is what is looked in.
export function slotValue(page: Page, propertyId: string, slot: number, input: string): Locator {
  return fieldSlots(page, propertyId).nth(slot).locator(`.pd-claiminput-${propertyId} > .pd-claiminput-value`).locator(input).first()
}

// Writes into a rich text field and commits it. A rich text field is typed into rather than filled,
// because the editor is a content editable surface and not an input, and it writes its value into the
// editing session when the focus leaves it.
export async function fillHtmlField(page: Page, propertyId: string, text: string, what: string): Promise<void> {
  const editor = field(page, propertyId).locator(".pd-inputhtml-editor").first()
  await expect(editor, `rich text editor of ${what}`).toBeVisible({ timeout: LOADING_TIMEOUT })
  await editor.click()
  await page.keyboard.type(text)
  await editor.blur()
  await settleEdit(page)
  await expect(editor, `rich text editor of ${what} after typing`).toContainText(text)
}

// Everything below is about one rich text editor of a form. Which part of the form holds it differs between
// the applications and between the forms (a whole field, or one slot of a field which may hold several
// values), so the part holding it is what these take, and the editor inside it is what they address.

// The toolbar of the rich text editor held by the given part of the form.
export function htmlToolbar(editor: Locator): Locator {
  return editor.locator(".pd-inputhtml-toolbar")
}

// One toolbar button, addressed by the part of its class name which says what it does.
export function htmlToolbarButton(editor: Locator, name: string): Locator {
  return htmlToolbar(editor).locator(`.pd-inputhtml-button-${name}`)
}

// The element ProseMirror makes editable inside the mount point of the editor. The mount point itself is
// not focusable and holds no text, so everything about focus and about the value is asserted on this one.
export function htmlEditorContent(editor: Locator): Locator {
  return editor.locator('.pd-inputhtml-editor [contenteditable="true"]').first()
}

// What one toolbar button is, as a keyboard test sees it: what it does, whether it can be used at all,
// whether it is the tab stop of the toolbar, and whether it is the focused element.
export interface HtmlToolbarButtonState {
  name: string
  disabled: boolean
  tabIndex: number
  focused: boolean
}

export async function htmlToolbarState(editor: Locator): Promise<Array<HtmlToolbarButtonState>> {
  return await htmlToolbar(editor)
    .locator("button")
    .evaluateAll((buttons) =>
      buttons.map((button) => ({
        name:
          Array.from(button.classList)
            .find((name) => name.startsWith("pd-inputhtml-button-"))
            ?.replace("pd-inputhtml-button-", "") ?? "",
        disabled: (button as HTMLButtonElement).disabled,
        tabIndex: (button as HTMLButtonElement).tabIndex,
        focused: button === document.activeElement,
      })),
    )
}

// The button the toolbar currently offers as its single tab stop, which is the whole point of the roving
// tabindex: one Tab reaches the toolbar and the next one leaves it, however many buttons it holds.
export async function htmlTabbableButton(editor: Locator): Promise<string> {
  const tabbable = (await htmlToolbarState(editor)).filter((button) => button.tabIndex === 0)
  expect(
    tabbable.map((button) => button.name),
    "exactly one button of the toolbar is a tab stop",
  ).toHaveLength(1)
  return tabbable[0].name
}

// The name of the toolbar button which is focused, or the empty string when focus is elsewhere.
export async function htmlFocusedButton(editor: Locator): Promise<string> {
  const focused = (await htmlToolbarState(editor)).filter((button) => button.focused)
  return focused.length === 1 ? focused[0].name : ""
}

// Moves focus with an arrow key of the toolbar and asserts where it lands: the button has to be both the
// focused one and the one the toolbar offers as its tab stop, because the two are kept in step by the
// focusin handler and a button which is focused but not tabbable would strand the next Tab.
export async function pressHtmlToolbarKey(page: Page, editor: Locator, key: string, expected: string): Promise<void> {
  await page.keyboard.press(key)
  await expect.poll(() => htmlFocusedButton(editor), { message: `${key} moves focus to the ${expected} button` }).toBe(expected)
  expect(await htmlTabbableButton(editor), `${key} makes the ${expected} button the tab stop of the toolbar`).toBe(expected)
}

// Activates a toolbar button from the keyboard rather than with a click, which is how the command it
// stands for is reached without a pointer. Focusing the button also makes it the tab stop of the toolbar,
// which is what the roving tabindex is for.
export async function activateHtmlToolbarButton(page: Page, editor: Locator, name: string): Promise<void> {
  const button = htmlToolbarButton(editor, name)
  await expect(button, `${name} button`).toBeEnabled()
  await button.focus()
  await page.keyboard.press("Enter")
}

// The HTML of the value being edited, as the editor holds it. The editor writes the claim from this, so
// a command which changed this changed the value.
export async function htmlEditorValue(editor: Locator): Promise<string> {
  return await htmlEditorContent(editor).innerHTML()
}

// Waits until the value being edited holds the given HTML. A key press is delivered to the page and
// handled there, so what it did is not in the DOM the moment the press returns.
export async function expectHtmlEditorValue(editor: Locator, expected: string, message: string): Promise<void> {
  await expect.poll(() => htmlEditorValue(editor), { message }).toContain(expected)
}

// The text the browser has selected, which is what a mark shortcut applies to. A shortcut pressed while
// the selection is still collapsed only arms the mark for what is typed next and leaves the value alone,
// so a test which applies a mark to existing text waits for the selection before pressing the shortcut.
export async function selectedText(page: Page): Promise<string> {
  return await page.evaluate(() => window.getSelection()?.toString() ?? "")
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
//
// What it answers is that a change has landed, not that the caller's has: it resolves on the first post to
// answer, which is another slot's change when one was still on its way when this was asked for. Follow it
// with expectNothingPending, which covers the rest of what the form owes. The two are needed together: this
// one alone can be handed the wrong answer, and expectNothingPending alone is satisfied by a form which has
// not taken the change on yet.
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
  await expectNothingPending(page)
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

// The identifiers of the classes the create page offers to start a document of, read out of the identifier
// every class button carries in its own CSS class, so what is offered is compared as a set of documents
// rather than as a list of labels, which differ between languages. A class which is a subclass of more than
// one class is listed once under each of them, so the identifiers are deduplicated.
export async function offeredClasses(page: Page): Promise<Array<string>> {
  const ids = await page
    .locator(".pd-classtreelabel-button")
    .evaluateAll((buttons) =>
      buttons.map((button) => [...button.classList].map((name) => /^pd-classtreelabel-button-(.+)$/.exec(name)?.[1]).find((id) => id !== undefined) ?? ""),
    )
  return [...new Set(ids)].sort()
}

// Starts creating a document of the given class. The create view lists the classes as a tree and a class is
// chosen by clicking it, so the class is addressed by the identifier in its class name.
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
  await expectNothingPending(page)
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

// One facet of the filters panel, addressed by the CSS class it carries: its kind followed by the identifiers
// of the property path it filters on. A nested facet passes both properties of its path.
export function filter(page: Page, kind: "ref" | "has" | "time" | "amount", ...props: Array<string>): Locator {
  return page.locator([".pd-filtersresult", kind, ...props].join("-"))
}

// The checkbox of one value of a facet, addressed by the id the facet gives it, which is the kind, the
// property path and the identifier of the value document joined by slashes.
export function filterValue(page: Page, kind: "ref" | "has", props: Array<string>, value: string): Locator {
  return page.locator(`[id="${[kind, ...props, value].join("/")}"]`)
}

// The checkbox of one property of the has facet on the document itself. That facet filters on no property
// path of its own, so the id its checkboxes carry has an empty path segment in the middle, which is what a
// path of a single empty string produces.
export function hasValue(page: Page, propertyId: string): Locator {
  return filterValue(page, "has", [""], propertyId)
}

// Runs an action which changes the search from inside the result page, and waits until the results of the
// changed search have come back. What the page then makes of them is not waited for, so a caller waits for
// that itself: searchAgain is that wait for a change which is expected to find something and to show it in
// the feed, and a caller whose change finds nothing, or which shows what it found in something else, waits
// for what it expects instead.
//
// Nothing the page renders can be waited for instead of the response. The results header goes on reporting
// the count of the previous search for as long as the new one is in flight, because useSearchResults replaces
// its total only once the new results are in and does not clear it while it fetches them, and the feed goes
// on showing the results that count belongs to. A control which was just used says nothing either: it renders
// state of the search session rather than of the search, so it takes the change the moment it is clicked.
// Waiting for the network to go idle does not bridge the two: the fetch is issued a few ticks after the
// click, and until it is the page is idle, so that wait can be over before the search has even started.
//
// Which results are waited for is decided by the change itself: a change is posted to the session and
// answered with the version the session is at afterwards, and the results of that version are fetched under
// it, so the two together follow the change from the click to the results it produced. Waiting for the next
// results of any version instead is satisfied by an answer to a question asked before the action: a result
// page fetches its results whenever anything about it changes, so one of those fetches can still be in
// flight when the action runs, and what the page shows when such a wait returns is still the previous
// search.
// The version of the results cannot be waited for the way the change itself is, because it is not known
// until the change has been answered: the answer has to be read first, and the results of the changed search
// can be back before it has been. The action is what makes that likely rather than rare, because an action
// which waits for something the page does with the changed search returns after the results it is waiting
// for have already arrived. Every results response is therefore recorded from before the action, and the
// version is looked for among the ones which have come back as well as the ones still to come.
export async function applySearchChange(page: Page, action: () => Promise<void>): Promise<void> {
  const fetched = new Set<string>()
  const record = (response: Response) => {
    if (response.url().includes(SEARCH_RESULTS_API)) {
      const version = new URL(response.url()).searchParams.get("version")
      if (version !== null) {
        fetched.add(version)
      }
    }
  }
  page.on("response", record)
  let version: number
  try {
    const updated = page.waitForResponse((response) => response.url().includes(SEARCH_UPDATE_API) && response.request().method() === "POST", {
      timeout: LOADING_TIMEOUT,
    })
    await action()
    ;({ version } = (await (await updated).json()) as { version: number })
    await expect
      .poll(() => fetched.has(String(version)), { message: `the results of the search at version ${version} the change produced`, timeout: LOADING_TIMEOUT })
      .toBe(true)
  } finally {
    page.off("response", record)
  }

  // The answer having arrived is not the same as it being on the screen: the results are rendered a tick
  // later, so a read which follows this can otherwise still be a read of the search before the change. What
  // renders the results publishes the address they came from, so waiting for that address to name the
  // version the change produced waits for the render itself.
  await expectResultsVersion(page, version)
}

// The element rendering the results, in either of the two views the results are shown in. Each publishes
// the address its results came from, the way the filters panel publishes the address of its facets.
const RESULTS_ELEMENT = ".pd-searchresultsfeed-list-results, .pd-searchresultstable-list-results"

// Waits until what is rendered are the results of the given version of the search.
async function expectResultsVersion(page: Page, version: number): Promise<void> {
  await expect
    .poll(async () => await versionOfElement(page, RESULTS_ELEMENT), {
      message: `the results on the screen are the ones of the search at version ${version}`,
      timeout: LOADING_TIMEOUT,
    })
    .toBe(String(version))
}

// Runs an action which changes the search and waits until the results of the changed search are on the
// screen, which is what a change expected to find something is followed by.
export async function searchAgain(page: Page, action: () => Promise<void>): Promise<void> {
  await applySearchChange(page, action)
  await expectResults(page)
}

// Waits for a value of a facet to be on the page, and says which facet was looked at and what that facet
// offers when it is not there.
//
// A value is addressed by the identifiers of the property it belongs to and of the value itself, which is
// what the markup carries and what makes the address independent of the interface language. Those
// identifiers name nothing a reader of a failure can recognise, while the facet renders both as labels, so
// the labels are what the failure is written with: which facet was looked at, and which values it offers
// instead of the one which was asked for.
async function expectFilterValueOffered(facet: Locator, value: Locator): Promise<void> {
  try {
    await expect(value, "value of the facet to select").toBeVisible({ timeout: LOADING_TIMEOUT })
  } catch {
    const title = ((await facet.locator(".pd-filtersresult-title").first().textContent()) ?? "").replace(/\s+/g, " ").trim()
    const offered = (await facet.locator(".pd-reffiltertreerow-label, .pd-hasfiltersresult-label").allTextContents())
      .map((label) => label.replace(/\s+/g, " ").trim())
      .filter((label) => label !== "")
    throw new Error(
      `the facet "${title || "which is on the page unnamed"}" does not offer the value to select (${String(value)}): it offers ${offered.length > 0 ? offered.join(", ") : "no value at all"}`,
    )
  }
}

// Selects a value of a facet and waits until the search it started has come back. A facet grows a clear
// button only once its filter is active, so that is what tells a click which took effect apart from one
// which has not been applied yet.
export async function applyFilterValue(page: Page, facet: Locator, value: Locator): Promise<void> {
  // The values of a facet are fetched per facet rather than coming with the panel, so a facet card is on the
  // page before the values it offers are, and a facet renders only its first values and keeps the rest
  // behind a button of its own. Which of them the wanted value is among follows the counts, so it is asked
  // for rather than assumed to be in the first batch: every pass shows another batch, and the cap is far
  // above what any facet of the test data holds.
  const more = facet.locator(".pd-filtersresult-more")
  for (let pass = 0; pass < FILTER_PASSES && (await value.count()) === 0; pass++) {
    await settle(page)
    if (!(await more.isVisible().catch(() => false))) {
      break
    }
    await more.dispatchEvent("click").catch(() => null)
  }
  await expectFilterValueOffered(facet, value)
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

// The part of a screenshot name which identifies a mnemonic: the mnemonic in lower case, with its
// underscores written as dashes.
export function slug(mnemonic: string): string {
  return mnemonic.toLowerCase().replaceAll("_", "-")
}

// Closes the sort and grouping dialog so that the results behind it are shown unobstructed.
export async function closeSortDialog(page: Page): Promise<void> {
  const closeButton = page.locator(".pd-searchsortdialog-button-close")
  await expect(closeButton, "the button which closes the sort dialog").toBeVisible()
  await closeButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel"), "the closed sort dialog").toBeHidden()
}

// Screenshots the sort and grouping dialog itself. The screenshot covers the viewport rather than the whole
// page, because the dialog is drawn over the page and only dims what is behind it. A column which comes from
// a facet is named by the property it is for, which the dialog fetches, so the capture waits until every
// label on the page has resolved.
//
// The panel behind the dialog is settled first, because the columns the dialog offers to add are the facets
// the panel has loaded: a dialog opened over a panel which is still loading offers however many of them had
// arrived by then, which is not the same twice. Settling it is what fixes the list to every facet the search
// has.
export async function checkpointDialog(page: Page, name: string): Promise<void> {
  const panel = page.locator(".pd-searchsortdialog-panel")
  await expect(panel, "the sort dialog").toBeVisible()
  await settleFilters(page)
  await expectNothingLoading(page)
  // The dialog is taller than the window and scrolls inside itself, and reaching a control in it scrolls it
  // there: adding a column which the list of them holds below the fold scrolls the dialog down, and ticking
  // that column's checkbox afterwards scrolls back only as far as the row it is in. The dialog is then left
  // standing wherever the columns the search had loaded by then happened to put those two, which is not the
  // same twice. It is anchored at its top before the capture, the way a whole page capture anchors the page.
  await panel.evaluate((element) => element.scrollTo({ top: 0, left: 0, behavior: "instant" }))
  await checkpoint(page, name, { mask: volatile(page), fullPage: false })
}

// Adds the given column to the sort order and waits for the results the change produced.
export async function addSortColumn(page: Page, column: string): Promise<void> {
  const button = page.locator(`.pd-searchsortdialog-button-add-${column}`)
  await expect(button, `the button which adds the ${column} column`).toBeVisible()
  await searchAgain(page, async () => await button.click())
}

// Opens the sort and grouping dialog.
export async function openSortDialog(page: Page): Promise<void> {
  const sortButton = page.locator(".pd-searchresultsheader-button-sort")
  await expect(sortButton).toBeVisible()
  await sortButton.click()
  await expect(page.locator(".pd-searchsortdialog-panel")).toBeVisible()
}

// The checkbox which groups the results by the given column. It is offered only for a reference column
// which every column before it is also grouped by, so that the grouped columns stay the leading ones.
export function groupCheckbox(page: Page, column: string): Locator {
  return sortColumn(page, column).locator(".pd-searchsortdialog-checkbox-group")
}

// The checkbox which renders each of the given column's group values as a full result card instead of a
// one-line heading. It is offered only while the column is grouped by.
export function expandCheckbox(page: Page, column: string): Locator {
  return sortColumn(page, column).locator(".pd-searchsortdialog-checkbox-expand")
}

// The sort order entry for the given column, from which its buttons are reached.
export function sortColumn(page: Page, column: string): Locator {
  return page.locator(`.pd-searchsortdialog-item-sort-${column}`)
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

// The bounds and the counts the API reports next to a facet's histogram, read from the response the facet
// itself loaded: it publishes the very URL it fetched, so the numbers are the ones it is rendering rather
// than ones from a separately built request. The server sends them in a header next to the buckets, as
// "exists=44, from="0.5", missing=1377, ...".
//
// Assertions are written against these rather than against the numbers of the test data wherever the rule
// the view follows is what matters, so that a document another test adds cannot make this file fail.
export async function facetMetadata(page: Page, facet: Locator): Promise<Record<string, number>> {
  const url = await facet.getAttribute("data-url")
  expect(url, "the facet publishes the URL it loaded its values from").toBeTruthy()
  const response = await fetchFromPage(page, url!)
  expect(response.status, "the request the facet made is answered").toBe(200)
  const metadata = response.headers["metadata"]
  expect(metadata, "the answer carries the bounds and counts of the facet").toBeTruthy()
  const values: Record<string, number> = {}
  for (const part of metadata.split(",")) {
    const match = /^\s*([a-z_]+)="?(-?[0-9.]+(?:[eE][-+]?[0-9]+)?)"?\s*$/.exec(part)
    if (match) {
      values[match[1]] = Number(match[2])
    }
  }
  return values
}

// Asserts that the file the given link points at is served whole, so that the attachment is reachable from
// the saved document and not just referenced by it. The size is compared against the file on disk, which is
// what says the whole file came back and not only its first bytes.
export async function expectAttachmentServed(page: Page, link: Locator, path: string, what: string): Promise<void> {
  await expect(link, `link to the attachment of ${what}`).toBeVisible()
  const href = await link.getAttribute("href")
  expect(href, `address of the attachment of ${what}`).toMatch(/^\/f\/[0-9A-Za-z]+$/)

  const response = await fetchFromPage(page, href!)
  expect(response.status, `status of the attachment of ${what}`).toBe(200)
  expect(response.length, `size of the attachment of ${what}`).toBe(statSync(path).size)
  expect(response.headers["content-disposition"], `filename the attachment of ${what} is served under`).toContain(basename(path))
}

// Holds every upload the page starts at its first request, until the returned release function is called.
// An upload of a small file is over before a screenshot of it can be taken, so the states which exist only
// while it is running (the progress bar, the cancel button) are reachable only by making the upload wait.
export async function holdUploads(page: Page): Promise<() => Promise<void>> {
  let held = true
  await page.route(BEGIN_UPLOAD_URL, async (route) => {
    while (held) {
      await new Promise((resolve) => setTimeout(resolve, 50))
    }
    // The browser drops the request when the upload is cancelled while it is held here, and continuing a
    // request which the browser no longer waits for throws, which is the expected outcome then.
    await route.continue().catch(() => null)
  })
  return async () => {
    held = false
    // Waits for the handler above to finish, so a route is never removed while it is still held.
    await page.unrouteAll({ behavior: "ignoreErrors" })
  }
}

// The request every upload starts with. Holding it is how a test freezes an upload in flight.
export const BEGIN_UPLOAD_URL = "**/api/f/beginUpload"

// The downloadOverlay a download shows while it runs. It closes itself once the download is over.
export function downloadOverlay(page: Page) {
  return page.locator(".pd-downloadoverlay-dialog")
}

// Holds back part of the traffic a bulk download makes, so that the overlay can be screenshotted in a state
// which stays put while the screenshot is taken: without this the progress runs to the end and the overlay
// closes itself before a stable screenshot of it exists. Every request for a file goes through here. The
// metadata requests (HEAD, one per attachment, which the download makes while it collects the files to
// download) are held while the preparation progress is screenshotted, and the content request (GET) of the
// last attachment is held while the final progress is screenshotted. Neither changes what is downloaded.
export async function holdFileRequests(page: Page, attachments: number, metadata: RequestGate, lastContent: RequestGate): Promise<void> {
  let contentRequests = 0
  await page.route("**/f/*", async (route) => {
    if (route.request().method() === "HEAD") {
      await metadata.wait
    } else {
      contentRequests += 1
      if (contentRequests === attachments) {
        await lastContent.wait
      }
    }
    await route.continue()
  })
}

export function requestGate(): RequestGate {
  let open!: () => void
  const wait = new Promise<void>((resolve) => {
    open = resolve
  })
  return { open, wait }
}

// A promise which a request handler waits on and the test opens when it wants the request to go through.
export interface RequestGate {
  open: () => void
  wait: Promise<void>
}
