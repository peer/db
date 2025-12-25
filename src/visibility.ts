import type { ComponentPublicInstance } from "vue"

import { onBeforeUnmount, reactive, readonly } from "vue"

export function useVisibilityTracking(): {
  track: (id: string) => (el: Element | ComponentPublicInstance | null) => void
  visibles: ReadonlySet<string>
} {
  const idToElement = new Map<string, Element>()
  const elementToId = new Map<Element, string>()
  const _visibles = reactive(new Set<string>())
  const visibles = import.meta.env.DEV ? readonly(_visibles) : _visibles

  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        const id = elementToId.get(entry.target)
        if (id) {
          if (entry.isIntersecting) {
            _visibles.add(id)
          } else {
            _visibles.delete(id)
          }
        }
      }
    },
    {
      root: null,
      rootMargin: "-10% 0% -10% 0%",
      threshold: 0.1,
    },
  )

  onBeforeUnmount(() => observer.disconnect())

  function trackElement(id: string, el: Element) {
    const old = idToElement.get(id)
    if (old) {
      elementToId.delete(old)
      observer.unobserve(old)
    }
    idToElement.set(id, el)
    elementToId.set(el, id)
    observer.observe(el)
  }

  function untrackElement(id: string) {
    const old = idToElement.get(id)
    if (!old) return
    idToElement.delete(id)
    elementToId.delete(old)
    observer.unobserve(old)
    _visibles.delete(id)
  }

  return {
    track: (id) => {
      return (el) => {
        if (el) {
          if (!(el instanceof Element)) {
            el = el.$el as Element
          }
          trackElement(id, el)
        } else {
          untrackElement(id)
        }
      }
    },
    visibles,
  }
}
