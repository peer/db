import { defineConfig, devices } from "@playwright/test"

export default defineConfig({
  testDir: "./tests",
  testMatch: "*.test.ts",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 120000, // 2 minutes per test.
  reporter: [
    ["html", { outputFolder: "playwright-report", open: "never" }],
    ["junit", { outputFile: "test-results/junit.xml" }],
  ],
  use: {
    baseURL: process.env.PEERDB_URL || "https://localhost:8080",
    ignoreHTTPSErrors: false,
    trace: "on-first-retry",
    viewport: { width: 1280, height: 720 },
    headless: true,
    // Assertions are generally used when the element should be present. This timeout only accounts for asynchronicity.
    actionTimeout: 10000,
    ...devices["Desktop Chrome"],
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
