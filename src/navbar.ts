import type { Ref, StyleValue, TemplateRef } from "vue"

import { computed, onBeforeUnmount, onMounted, ref, useTemplateRef, watchEffect } from "vue"

import { getConfig } from "@/config"
import siteContext from "@/context"

const prefersReducedMotionQuery = window.matchMedia("(prefers-reduced-motion: reduce)")
const prefersReducedMotion = ref(prefersReducedMotionQuery.matches)
prefersReducedMotionQuery.addEventListener("change", (e) => {
  prefersReducedMotion.value = e.matches
})

// The current value of the navbar search query input, kept in sync by NavBarSearch while it is mounted.
// Sibling navbar components (the search shortcut buttons) and the SearchGet view read it so that clicking
// a search shortcut combines the possibly uncommitted query with the shortcut, the same query the search
// button would submit. There is at most one navbar at a time, so a single shared value is sufficient.
const navbarSearchQuery = ref("")

export function useNavbarSearchQuery(): Ref<string> {
  return navbarSearchQuery
}

export type NavbarMode = "auto" | "fixed" | "static"

// The navbar positioning mode. The site's navbarPosition feature decides it: "static" keeps the navbar in
// the document flow at the page top (it also satisfies the reduced-motion preference since nothing moves),
// "fixed" keeps it at the viewport top, and unset means auto-hide, upgraded to fixed by the provided config
// or the reduced-motion preference. Reactive so callers can react to config or reduced-motion changes.
export function useNavbarMode(): Ref<NavbarMode> {
  const config = getConfig()
  return computed(() => {
    if (siteContext.features.navbarPosition === "static") {
      return "static"
    }
    if (siteContext.features.navbarPosition === "fixed" || !!config.value.fixedNavbar || prefersReducedMotion.value) {
      return "fixed"
    }
    return "auto"
  })
}

// Whether the navbar is in fixed mode (always at viewport top). Shared by useNavbar itself and any consumer
// that needs to account for a permanently-visible navbar (e.g. computing scroll-to-anchor offsets).
export function useFixedNavbar(): Ref<boolean> {
  const mode = useNavbarMode()
  return computed(() => mode.value === "fixed")
}

export function useNavbar(): { navbar: TemplateRef<HTMLElement>; attrs: Ref<{ style: StyleValue; class: { "animate-navbar": boolean } }> } {
  const navbarMode = useNavbarMode()

  const navbar = useTemplateRef<HTMLElement>("navbar")
  const attrs = ref<{
    style: { position: "absolute" | "fixed" | "static"; top: string }
    class: { "animate-navbar": boolean }
  }>({
    style: { position: "absolute", top: "0px" },
    class: { "animate-navbar": false },
  })

  let lastScrollPosition = 0
  const supportScrollY = window.scrollY !== undefined

  function onScroll() {
    if (!navbar.value) {
      return
    }

    const currentScrollPosition = supportScrollY ? window.scrollY : document.documentElement.scrollTop
    if (currentScrollPosition <= 0) {
      attrs.value.style.position = "absolute"
      attrs.value.style.top = "0px"
      lastScrollPosition = 0
      publishNavbarTop()
      return
    }

    if (currentScrollPosition > lastScrollPosition) {
      if (attrs.value.style.position !== "absolute") {
        attrs.value.class["animate-navbar"] = false
        const { top } = navbar.value.getBoundingClientRect()
        attrs.value.style.position = "absolute"
        attrs.value.style.top = `${lastScrollPosition + top}px`
      }
    } else if (currentScrollPosition < lastScrollPosition) {
      if (attrs.value.style.position !== "fixed") {
        const { top, height } = navbar.value.getBoundingClientRect()
        if (top >= 0) {
          attrs.value.style.top = "0px"
          attrs.value.style.position = "fixed"
        } else if (top < -height) {
          if (lastScrollPosition - currentScrollPosition > 10) {
            // Scroll speed is large so we just do the animation instead.
            attrs.value.style.top = "0px"
            attrs.value.style.position = "fixed"
            attrs.value.class["animate-navbar"] = true
          } else {
            attrs.value.style.top = `${currentScrollPosition - height}px`
          }
        }
      }
    }

    lastScrollPosition = currentScrollPosition
    publishNavbarTop()
  }

  // Publish navbar's current viewport top so other components (e.g. TableOfContents) can follow it via CSS.
  // Clamped to [-height, 0]: once fully hidden the navbar cannot visually move further up, so followers should not either.
  function publishNavbarTop() {
    if (!navbar.value) return
    const { top, height } = navbar.value.getBoundingClientRect()
    const clamped = Math.min(0, Math.max(top, -height))
    document.documentElement.style.setProperty("--pd-navbar-top", `${clamped}px`)
  }

  watchEffect((onCleanup) => {
    attrs.value.style.top = "0px"
    attrs.value.class["animate-navbar"] = false

    if (navbarMode.value === "static") {
      attrs.value.style.position = "static"
      // The in-flow navbar pushes content down by itself, so content does not need the top margin.
      document.documentElement.style.setProperty("--pd-navbar-offset", "0px")
      // The navbar scrolls with the page, so followers still need its published viewport top.
      window.addEventListener("scroll", publishNavbarTop, { passive: true })
      window.addEventListener("scrollend", publishNavbarTop, { passive: true })
      publishNavbarTop()
      onCleanup(() => {
        window.removeEventListener("scroll", publishNavbarTop)
        window.removeEventListener("scrollend", publishNavbarTop)
        document.documentElement.style.removeProperty("--pd-navbar-offset")
        document.documentElement.style.removeProperty("--pd-navbar-top")
      })
      return
    }

    if (navbarMode.value === "fixed") {
      attrs.value.style.position = "fixed"
      // Fixed navbar always sits at viewport top:0, so nothing to follow.
      document.documentElement.style.setProperty("--pd-navbar-top", "0px")
      onCleanup(() => {
        document.documentElement.style.removeProperty("--pd-navbar-top")
      })
      return
    }

    lastScrollPosition = supportScrollY ? window.scrollY : document.documentElement.scrollTop
    window.addEventListener("scroll", onScroll, { passive: true })
    // Re-publish after scroll settles. The DOM rect is stale right after Vue mutates attrs.value.style during a
    // transition (Vue applies style updates async), so getBoundingClientRect inside onScroll can return the old
    // position. By the time scrollend fires, Vue has flushed and the rect is accurate.
    window.addEventListener("scrollend", publishNavbarTop, { passive: true })
    // animate-navbar runs a 100ms CSS animation that moves the navbar via transform. During the animation,
    // getBoundingClientRect reflects the mid-animation position; re-publish at animationend to capture the final.
    const el = navbar.value
    el?.addEventListener("animationend", publishNavbarTop)
    publishNavbarTop()
    onCleanup(() => {
      window.removeEventListener("scroll", onScroll)
      window.removeEventListener("scrollend", publishNavbarTop)
      el?.removeEventListener("animationend", publishNavbarTop)
      document.documentElement.style.removeProperty("--pd-navbar-top")
    })
  })

  return { navbar, attrs }
}

// Collapses a navbar search element (the search input, or the search-session link on document pages) to
// its compact form once the navbar can no longer fit it. The element must carry a minimum usable width
// while inline (set in the stylesheet, gated on NOT having collapsedClass); the measurement toggles
// collapsedClass off, and if the navbar then overflows there is no longer room, so collapsedClass is set
// and the consumer's styles shrink the element to just its button/icon. This is based on the room actually
// left for the element, not the viewport width, so it collapses sooner when other items sit next to it
// (for example a filter toggle) and stays inline longer when the navbar is otherwise empty. getTarget is a
// function (rather than a ref) so the caller can locate the element however it likes, including by query;
// skip lets the caller suspend measuring (NavBarSearch uses it while the input is expanded to a full-width
// overlay). Returns whether the element is currently collapsed, for callers that also branch on it in script.
//
// The marker is applied to the target with classList. If the target also carries a reactive Vue class
// binding, bind this returned ref into that binding as well: Vue patches a class by overwriting the whole
// class attribute, so a patch triggered by any other bound class changing would otherwise drop the marker.
export function useNavbarCollapse(getTarget: () => HTMLElement | null, collapsedClass: string, skip?: () => boolean): Ref<boolean> {
  const collapsible = ref(false)

  let navbar: HTMLElement | null = null
  let resizeObserver: ResizeObserver | null = null
  let mutationObserver: MutationObserver | null = null
  // Measured directly in the observer callbacks. In the ResizeObserver callback this runs after layout and
  // before paint, so the collapse applies in the same frame with no flash, and the browser already delivers it
  // at most once per frame. The MutationObserver callback is a microtask, so layout may still be dirty when it
  // runs, but reading scrollWidth/clientWidth forces a synchronous layout, so the measurement is correct no
  // matter the callback timing, and the microtask still runs before paint, so there is no flash there either.
  // Toggling collapsedClass changes the target's own width, not the observed navbar's box, so there is no
  // resize-observer loop to guard against, and it is an attribute change we do not observe, so it does not
  // re-trigger the MutationObserver either.
  function measure(): void {
    const target = getTarget()
    if (!target || !navbar || skip?.()) {
      return
    }
    target.classList.remove(collapsedClass)
    const overflowing = navbar.scrollWidth - navbar.clientWidth > 0.5
    if (overflowing) {
      target.classList.add(collapsedClass)
    }
    collapsible.value = overflowing
  }

  onMounted(() => {
    navbar = document.querySelector<HTMLElement>(".pd-navbar")
    if (navbar) {
      resizeObserver = new ResizeObserver(() => {
        measure()
      })
      resizeObserver.observe(navbar)
      // The ResizeObserver only fires on the navbar's own size. Also re-measure when its contents change,
      // which alters the room left for the element without resizing the navbar: a filter toggle teleported in
      // on the search feed, or a document page's prev/next buttons appearing once its results load. These can
      // settle after mount and after fonts are ready, so neither the first measurement nor fonts.ready catches
      // them on their own.
      mutationObserver = new MutationObserver(() => {
        measure()
      })
      mutationObserver.observe(navbar, { childList: true, subtree: true })
    }
    measure()
    // Web fonts load asynchronously and widen the navbar's text items (and so shrink the room left for the
    // element) after this first measurement, without resizing the navbar or changing its children. Re-measure
    // once fonts are ready so a cold load does not stay wrongly inline.
    void document.fonts?.ready.then(() => {
      measure()
    })
  })

  onBeforeUnmount(() => {
    resizeObserver?.disconnect()
    mutationObserver?.disconnect()
  })

  return collapsible
}
