import type { ComputedRef, Ref } from "vue"
import type { Router } from "vue-router"

import { computed } from "vue"
import { useRouter } from "vue-router"

import { parseUrl } from "@/utils"

// CSS classes stamped onto anchor elements during HTML transformation.
// There is hierarchy between LINK_CLASS_INTERNAL > LINK_CLASS_INTERNAL_NOVIEW > LINK_CLASS_FILE.
export const LINK_CLASS_INTERNAL = "pd-link-internal"
export const LINK_CLASS_INTERNAL_NOVIEW = "pd-link-internal-noview"
export const LINK_CLASS_FILE = "pd-link-file"
export const LINK_CLASS_EXTERNAL = "pd-link-external"

// classifyLink returns the set of CSS classes that should be added to
// an anchor with the given href. Returns an empty array for hrefs we do not
// touch (hash, mailto, tel, javascript, unparseable).
//
// matchStorageRoute function is similar. Keep in sync as needed.
export function classifyLink(href: string, router: Router): string[] {
  if (!href) return []
  if (href.startsWith("#")) return []

  let url: URL
  try {
    url = parseUrl(href)
  } catch {
    return []
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return []

  if (url.origin !== window.location.origin) {
    return [LINK_CLASS_EXTERNAL]
  }

  // Same origin. Decide internal-noview / file refinements based on what the
  // SPA router knows about the path.
  const resolved = router.resolve(url.pathname)
  const matched = resolved.matched.length > 0

  if (matched && resolved.name === "StorageGet") {
    // File link: noview (browser navigation) plus its own icon class.
    return [LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW, LINK_CLASS_FILE]
  }

  if (!matched || !resolved.meta.hasView) {
    return [LINK_CLASS_INTERNAL, LINK_CLASS_INTERNAL_NOVIEW]
  }

  return [LINK_CLASS_INTERNAL]
}

// transformInternalHtml parses the given HTML once and add CSS classes on each anchor.
// Link icons are rendered via CSS rules in theme.css based on these classes.
function transformInternalHtml(html: string, router: Router): string {
  if (!html) return ""

  const doc = new DOMParser().parseFromString(html, "text/html")

  for (const anchor of doc.body.querySelectorAll("a")) {
    const href = anchor.getAttribute("href")
    if (!href) continue

    const classes = classifyLink(href, router)
    if (classes.length === 0) continue

    anchor.classList.add(...classes)
  }

  // TODO: Instead of transforming HTML string to another HTML string, just use insert the transformed DOM.
  //       It is redundant to generate transformed HTML string just for browser to parse it again.
  //       We could have our own version of "v-html" directive which first does the transformation.
  return doc.body.innerHTML
}

// useTransformedHtml returns a ComputedRef that runs transformInternalHtml on
// the source html only when the source changes.
export function useTransformedHtml(html: Ref<string | null | undefined>): ComputedRef<string> {
  const router = useRouter()
  return computed(() => transformInternalHtml(html.value ?? "", router))
}

// useInternalLinksClick returns a click handler that intercepts clicks on
// anchors previously classified as SPA-routable (pd-link-internal without
// pd-link-internal-noview) and routes them through Vue Router. All other
// link kinds (file, external, internal-noview, unclassified) keep their
// default browser behaviour.
export function useInternalLinksClick(): (event: MouseEvent) => void {
  const router = useRouter()

  return (event: MouseEvent): void => {
    if (event.defaultPrevented) return
    // Only act on plain left-click without modifier keys.
    if (event.button !== 0) return
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return

    const target = event.target as HTMLElement | null
    if (!target) return

    const anchor = target.closest("a")
    if (!anchor) return

    // Skip explicit "open in new tab/window" and download links.
    if (anchor.target && anchor.target !== "" && anchor.target !== "_self") return
    if (anchor.hasAttribute("download")) return

    // Class taxonomy already encodes the routing decision: only intercept
    // anchors classified as SPA-routable internal links.
    if (!anchor.classList.contains(LINK_CLASS_INTERNAL)) return
    if (anchor.classList.contains(LINK_CLASS_INTERNAL_NOVIEW)) return

    const href = anchor.getAttribute("href")
    if (!href) return

    event.preventDefault()
    void router.push(href)
  }
}
