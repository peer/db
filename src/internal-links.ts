import { useRouter } from "vue-router"

// useInternalLinksClick returns a click handler that intercepts clicks on
// anchor elements inside a container (typically content rendered via v-html)
// and routes same-origin URLs through Vue Router instead of letting the
// browser perform a full document navigation. External links, modifier-key
// clicks, middle-clicks, target="_blank", and download links are passed
// through unchanged.
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

    const href = anchor.getAttribute("href")
    if (!href) return
    // Pure hash links are handled by the browser (and Vue Router scrolling).
    if (href.startsWith("#")) return

    let url: URL
    try {
      url = new URL(href, window.location.href)
    } catch {
      return
    }

    // Only intercept same-origin URLs; external links keep default behaviour.
    if (url.origin !== window.location.origin) return

    // Only intercept URLs that resolve to a route the SPA actually renders.
    // Same-origin paths served directly by the backend (e.g. /f/<id> file
    // downloads) are registered without a view and must keep their default
    // browser behaviour.
    const path = url.pathname + url.search + url.hash
    const resolved = router.resolve(path)
    if (resolved.matched.length === 0) return
    if (!resolved.meta.hasView) return

    event.preventDefault()
    void router.push(resolved)
  }
}
