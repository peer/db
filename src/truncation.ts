import type { ComponentPublicInstance, DeepReadonly, Ref } from "vue"

import { onBeforeUnmount, readonly, ref } from "vue"

function isTruncated(el: Element): boolean {
  return el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight
}

function itemKey(groupId: string, itemId: string): string {
  return `${groupId}:${itemId}`
}

export function useTruncationTracking(): {
  track: (groupId: string, itemId: string) => (el: Element | ComponentPublicInstance | null) => void
  updated: (el: Element) => void
  truncated: DeepReadonly<Ref<Map<string, Set<string>>>>
} {
  const keyToElement = new Map<string, Element>()
  const elementToItem = new Map<Element, [string, string]>()
  const _truncated = ref(new Map<string, Set<string>>())
  const truncated = import.meta.env.DEV ? readonly(_truncated) : _truncated

  function addTruncated(groupId: string, itemId: string) {
    if (!_truncated.value.has(groupId)) {
      _truncated.value.set(groupId, new Set<string>())
    }
    _truncated.value.get(groupId)!.add(itemId)
  }

  function deleteTruncated(groupId: string, itemId: string) {
    const group = _truncated.value.get(groupId)
    if (group) {
      group.delete(itemId)
      if (group.size === 0) {
        _truncated.value.delete(groupId)
      }
    }
  }

  function updated(el: Element) {
    const item = elementToItem.get(el)
    if (item) {
      const [groupId, itemId] = item
      if (isTruncated(el)) {
        addTruncated(groupId, itemId)
      } else {
        deleteTruncated(groupId, itemId)
      }
    }
  }

  const observer = new ResizeObserver((entries) => {
    for (const entry of entries) {
      updated(entry.target)
    }
  })

  onBeforeUnmount(() => observer.disconnect())

  function trackElement(groupId: string, itemId: string, el: Element) {
    const key = itemKey(groupId, itemId)
    const old = keyToElement.get(key)
    if (old) {
      elementToItem.delete(old)
      observer.unobserve(old)
    }
    keyToElement.set(key, el)
    elementToItem.set(el, [groupId, itemId])
    observer.observe(el)
  }

  function untrackElement(groupId: string, itemId: string) {
    const key = itemKey(groupId, itemId)
    const old = keyToElement.get(key)
    if (!old) return
    keyToElement.delete(key)
    elementToItem.delete(old)
    observer.unobserve(old)
    deleteTruncated(groupId, itemId)
  }

  return {
    track: (groupId, itemId) => {
      return (el) => {
        if (el) {
          if (!(el instanceof Element)) {
            el = el.$el as Element
          }
          trackElement(groupId, itemId, el)
        } else {
          untrackElement(groupId, itemId)
        }
      }
    },
    updated,
    truncated,
  }
}
