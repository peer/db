import { ComponentPublicInstance } from "vue"
import { reactive, readonly, onBeforeUnmount } from "vue"

function isTruncated(el: Element): boolean {
  return el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight
}

function itemKey(groupId: string, itemId: string): string {
  return `${groupId}:${itemId}`
}

export function useTruncationTracking(): {
  track: (groupId: string, itemId: string) => (el: Element | ComponentPublicInstance | null) => void
  cellUpdated: (el: Element) => void
  truncated: ReadonlyMap<string, ReadonlySet<string>>
} {
  const keyToElement = new Map<string, Element>()
  const elementToItem = new Map<Element, [string, string]>()
  const _truncated = reactive(new Map<string, Set<string>>())
  const truncated = import.meta.env.DEV ? readonly(_truncated) : _truncated

  function addTruncated(groupId: string, itemId: string) {
    if (!_truncated.has(groupId)) {
      _truncated.set(groupId, new Set<string>())
    }
    _truncated.get(groupId)!.add(itemId)
  }

  function deleteTruncated(groupId: string, itemId: string) {
    const group = _truncated.get(groupId)
    if (group) {
      group.delete(itemId)
      if (group.size === 0) {
        _truncated.delete(groupId)
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
    cellUpdated: updated,
    truncated,
  }
}
