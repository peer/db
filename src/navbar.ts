import type { Ref, StyleValue, TemplateRef } from "vue"

import { onBeforeUnmount, onMounted, ref, useTemplateRef } from "vue"

export function useNavbar(): { navbar: TemplateRef<HTMLElement>; attrs: Ref<{ style: StyleValue; class: { "animate-navbar": boolean } }> } {
  const navbar = useTemplateRef<HTMLElement>("navbar")
  const attrs = ref({
    style: { position: "absolute" as "absolute" | "fixed", top: "0px" },
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
      return
    }

    if (currentScrollPosition > lastScrollPosition) {
      if (attrs.value.style.position !== "absolute") {
        attrs.value.class["animate-navbar"] = false
        const { top } = navbar.value.getBoundingClientRect()
        attrs.value.style.position = "absolute"
        if (currentScrollPosition - lastScrollPosition < 10) {
          // Scroll speed is small enough for lastScrollPosition to be probably a better value
          // so that navbar appears at the location where the user started scrolling.
          attrs.value.style.top = `${lastScrollPosition + top}px`
        } else {
          attrs.value.style.top = `${currentScrollPosition + top}px`
        }
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
  }

  onMounted(() => {
    window.addEventListener("scroll", onScroll, { passive: true })
  })

  onBeforeUnmount(() => {
    window.removeEventListener("scroll", onScroll)
  })

  return { navbar, attrs }
}
