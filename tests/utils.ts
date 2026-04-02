import type { BrowserContext, Locator, Page } from "@playwright/test"
import type { Result } from "axe-core"
import type { PageScreenshotOptions } from "playwright-core"

import AxeBuilder from "@axe-core/playwright"
import { test as baseTest } from "@playwright/test"
import { createHtmlReport } from "axe-html-reporter"
import serialize from "canonicalize"
import { createHash } from "node:crypto"
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs"

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
        if (!CONSOLE_ALLOWLIST.some((pattern) => pattern.test(text))) {
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

// Generate accessibility report after all tests complete.
test.afterAll(() => {
  createHtmlReport({
    results: {
      violations: readdirSync("a11y-report")
        .filter((f) => f.endsWith(".json"))
        .map((f) => JSON.parse(readFileSync(`a11y-report/${f}`, { encoding: "utf-8" })) as Result),
    },
    options: {
      projectKey: "PeerDB Accessibility Report",
      outputDir: "a11y-report",
      reportFileName: "a11y-report.html",
    },
  })
})

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

// Take up to 10 screenshots, wait until they stabilize. We had issues (and flakiness) because sometimes
// screenshots are not saved fully (just part of the page is visible, the rest is blank). Now we wait
// visually for screenshot to stabilize (instead of waiting just for DOM).
async function takeStableScreenshot(page: Page, screenshotOptions: PageScreenshotOptions): Promise<Buffer> {
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

export async function checkpoint(page: Page, name: string, options: CheckpointOptions = { mask: [], fullPage: true }) {
  // Move mouse to the same location so the same element gets focused every time.
  await page.mouse.move(0, 0)
  const screenshotPath = test.info().snapshotPath(`${name}.png`, { kind: "screenshot" })
  const screenshotOptions = {
    fullPage: options?.fullPage ?? true,
    mask: options?.mask,
    clip: options?.clip,
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
    // Compare snapshot buffer with the existing one.
    expect(screenshotBuffer).toMatchSnapshot(`${name}.png`)
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
    writeFileSync(`a11y-report/${violationHash}.json`, serializedViolation, { flag: "w" })
  }

  // Check for any console logs.
  await expectNoConsoleErrors(page)
}
