import type { ComputedRef, InjectionKey, Ref } from "vue"
import type { Router } from "vue-router"

import { computed, inject } from "vue"
import { useRouter } from "vue-router"

import { isPlainClick, parseUrl } from "@/utils"

// SELF_TEMPLATE is the RFC 6570 level 1 expression standing for the id of the document the HTML belongs to,
// expanded in link targets. HTML which is not written per document (a field's instructions in particular,
// which are shared by every document of the class) can this way still link to a page of the current document,
// e.g. "/d/request/{self}" for its request page. Braces cannot appear in an URI, so a link target never
// contains the expression by accident and an unexpanded one cannot be mistaken for a working link. The name
// matches the "self" token search shortcuts use for the same document.
const SELF_TEMPLATE = "{self}"

// selfDocumentKey provides the id of the document the rendered HTML belongs to, for expanding SELF_TEMPLATE
// in link targets. It is provided by the view which knows which document that is; without it link targets are
// rendered as written. See progress.ts for the symbol pattern.
export const selfDocumentKey: InjectionKey<() => string> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-selfDocument") : Symbol()

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

// transformInternalHtml parses the given HTML once, expands SELF_TEMPLATE in link targets against self (the
// id of the document the HTML belongs to, null when it is not known) and adds CSS classes on each anchor.
// Link icons are rendered via CSS rules in theme.css based on these classes.
function transformInternalHtml(html: string, router: Router, self: string | null): string {
  if (!html) return ""

  const doc = new DOMParser().parseFromString(html, "text/html")

  for (const anchor of doc.body.querySelectorAll("a")) {
    let href = anchor.getAttribute("href")
    if (!href) continue

    // Expansion runs on the attribute value as written, before the target is parsed as an URL: braces are not
    // valid URI characters, so parsing percent-encodes them and the expression is not found anymore. The value
    // substituted in is an identifier, which needs no encoding, so this is a plain string replacement.
    if (self !== null && href.includes(SELF_TEMPLATE)) {
      href = href.replaceAll(SELF_TEMPLATE, self)
      anchor.setAttribute("href", href)
    }

    const classes = classifyLink(href, router)
    if (classes.length === 0) continue

    anchor.classList.add(...classes)

    if (classes.includes(LINK_CLASS_EXTERNAL)) {
      anchor.relList.add("noreferrer")
    }
  }

  // TODO: Instead of transforming HTML string to another HTML string, just use insert the transformed DOM.
  //       It is redundant to generate transformed HTML string just for browser to parse it again.
  //       We could have our own version of "v-html" directive which first does the transformation.
  return doc.body.innerHTML
}

// useTransformedHtml returns a ComputedRef that runs transformInternalHtml on
// the source html only when the source (or the document it belongs to) changes.
export function useTransformedHtml(html: Ref<string | null | undefined>): ComputedRef<string> {
  const router = useRouter()
  const self = inject(selfDocumentKey, null)
  return computed(() => transformInternalHtml(html.value ?? "", router, self?.() ?? null))
}

// useInternalLinksClick returns a click handler that intercepts clicks on
// anchors previously classified as SPA-routable (pd-link-internal without
// pd-link-internal-noview) and routes them through Vue Router. All other
// link kinds (file, external, internal-noview, unclassified) keep their
// default browser behaviour.
export function useInternalLinksClick(): (event: MouseEvent) => Promise<void> {
  const router = useRouter()

  return async (event: MouseEvent) => {
    // Only act on plain left-click without modifier keys.
    if (!isPlainClick(event)) return

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
    await router.push(href)
  }
}
