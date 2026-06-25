import type { Ref, StyleValue, TemplateRef } from "vue"

import { computed, ref, useTemplateRef, watchEffect } from "vue"

import { getConfig } from "@/config"

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

// Whether the navbar is in fixed mode (always at viewport top) rather than auto-hide. Reactive so callers
// can react to config or reduced-motion changes. Shared by useNavbar itself and any consumer that needs to
// account for a permanently-visible navbar (e.g. computing scroll-to-anchor offsets).
export function useFixedNavbar(): Ref<boolean> {
  const config = getConfig()
  return computed(() => !!config.value.fixedNavbar || prefersReducedMotion.value)
}

export function useNavbar(): { navbar: TemplateRef<HTMLElement>; attrs: Ref<{ style: StyleValue; class: { "animate-navbar": boolean } }> } {
  const fixedNavbar = useFixedNavbar()

  const navbar = useTemplateRef<HTMLElement>("navbar")
  const attrs = ref<{
    style: { position: "absolute" | "fixed"; top: string }
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

    if (fixedNavbar.value) {
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
