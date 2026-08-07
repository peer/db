// @vitest-environment jsdom

import { describe, expect, test } from "vitest"
import { createApp, ref } from "vue"
import { createMemoryHistory, createRouter } from "vue-router"

import { selfDocumentKey, useTransformedHtml } from "@/internal-links"

const SELF_ID = "K2fq8VJhLoPXW5tNbBc3Ay"

// useTransformedHtml reads the router and the id of the document the HTML belongs to through
// injection, so the transformation is exercised through a bare app providing both. A self of null
// stands for HTML rendered where the document is not known (no selfDocumentKey provided).
function transform(html: string, self: string | null): string {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/d/request/:id", name: "DocumentRequest", component: { render: () => null }, meta: { hasView: true } }],
  })
  const app = createApp({})
  app.use(router)
  if (self !== null) {
    app.provide(selfDocumentKey, () => self)
  }
  return app.runWithContext(() => useTransformedHtml(ref(html)).value)
}

describe("useTransformedHtml", () => {
  test("expands {self} into the link target", () => {
    expect(transform(`<p><a href="/d/request/{self}">request</a></p>`, SELF_ID)).toBe(`<p><a href="/d/request/${SELF_ID}" class="pd-link-internal">request</a></p>`)
  })

  test("expands {self} in every link of the HTML", () => {
    expect(transform(`<p><a href="/d/request/{self}">a</a> <a href="/d/request/{self}?from={self}">b</a></p>`, SELF_ID)).toBe(
      `<p><a href="/d/request/${SELF_ID}" class="pd-link-internal">a</a> <a href="/d/request/${SELF_ID}?from=${SELF_ID}" class="pd-link-internal">b</a></p>`,
    )
  })

  test("leaves {self} as written when the document is not known", () => {
    expect(transform(`<p><a href="/d/request/{self}">request</a></p>`, null)).toContain(`href="/d/request/{self}"`)
  })
})
