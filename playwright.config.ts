import { defineConfig, devices } from "@playwright/test"

export default defineConfig({
  testDir: "./tests",
  testMatch: "*.test.ts",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 120000, // 2 minutes per test.
  // What produced this run, so that one report can be told from another without dating it by hand: the
  // artifacts of two pipelines of the same suite are otherwise indistinguishable. The values reach the
  // report itself (they are carried through the blob reports into the merged one), so they travel with the
  // artifacts wherever those are downloaded to. They are empty for a run which is not a pipeline, which is
  // what a local run is. The pipeline sets them and test-e2e.sh passes them into the container the tests
  // run in.
  metadata: {
    commit: process.env.CI_COMMIT_SHA ?? "",
    // The branch or the tag the pipeline is for, whichever it was started from.
    ref: process.env.CI_COMMIT_REF_NAME ?? "",
    pipeline: process.env.CI_PIPELINE_ID ?? "",
    job: process.env.CI_JOB_ID ?? "",
  },
  reporter: [
    ["html", { outputFolder: "playwright-report", open: "never" }],
    ["junit", { outputFile: "test-results/junit.xml" }],
  ],
  use: {
    ...devices["Desktop Chrome"],
    baseURL: process.env.PEERDB_URL || "https://localhost:8080",
    ignoreHTTPSErrors: false,
    trace: "on-first-retry",
    viewport: { width: 1280, height: 720 },
    headless: true,
    // Assertions are generally used when the element should be present. This timeout only accounts for asynchronicity.
    actionTimeout: 10000,
    contextOptions: {
      reducedMotion: "reduce", // Avoids animation-related test flakiness.
    },
    launchOptions: {
      args: [
        "--font-render-hinting=none",
        "--disable-skia-runtime-opts",
        "--disable-font-subpixel-positioning",
        "--disable-lcd-text",
        "--force-color-profile=srgb",
        "--disable-partial-raster",
        "--disable-gpu",
        "--disable-threaded-animation",
        "--disable-checker-imaging",
      ],
    },
  },
  snapshotPathTemplate: "playwright-screenshots/{testFilePath}/{arg}{ext}",
  expect: {
    timeout: 10000,
    toMatchSnapshot: {
      threshold: 0,
    },
  },
  projects: [
    // Read-only projects run first and share one populated database, so their screenshots are
    // reproducible. Every project which changes documents runs after them, and each such project
    // is isolated from the others so one project's edits cannot leak into another's screenshots.
    {
      name: "chrome",
      testMatch: /chrome\/.*\.test\.ts$/,
    },
    {
      name: "pages",
      testMatch: /pages\/.*\.test\.ts$/,
    },
    {
      name: "search",
      testMatch: /search\/.*\.test\.ts$/,
    },
    {
      name: "document",
      testMatch: /document\/.*\.test\.ts$/,
    },
    {
      name: "permissions",
      testMatch: /permissions\/.*\.test\.ts$/,
    },
    {
      name: "edit",
      testMatch: /edit\/.*\.test\.ts$/,
      dependencies: ["chrome", "pages", "search", "document", "permissions"],
    },
    {
      name: "create",
      testMatch: /create\/.*\.test\.ts$/,
      dependencies: ["edit"],
    },
  ],
})
