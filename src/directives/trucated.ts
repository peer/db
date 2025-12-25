import type { DirectiveBinding } from "vue"

interface TruncatedBinding {
  onChange: (truncated: boolean, el: HTMLElement) => void
}

type TruncatedEl = HTMLElement & {
  _truncatedObservers?: {
    resizeObserver: ResizeObserver
    mutationObserver: MutationObserver
  }
}

function isTruncated(el: HTMLElement): boolean {
  return el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight
}

export const truncated = {
  mounted(el: TruncatedEl, binding: DirectiveBinding<TruncatedBinding>) {
    const { onChange } = binding.value
    if (!onChange) return

    const check = () => {
      onChange(isTruncated(el), el)
    }

    // Initial check after layout.
    requestAnimationFrame(check)

    // Watch resize.
    const resizeObserver = new ResizeObserver(check)
    resizeObserver.observe(el)

    // Watch text changes.
    const mutationObserver = new MutationObserver(check)
    mutationObserver.observe(el, {
      childList: true,
      subtree: true,
      characterData: true,
    })

    el._truncatedObservers = {
      resizeObserver,
      mutationObserver,
    }
  },

  updated(el: TruncatedEl, binding: DirectiveBinding<TruncatedBinding>) {
    const { onChange } = binding.value
    if (!onChange) return

    requestAnimationFrame(() => {
      onChange(isTruncated(el), el)
    })
  },

  unmounted(el: TruncatedEl) {
    const obs = el._truncatedObservers
    if (!obs) return

    obs.resizeObserver.disconnect()
    obs.mutationObserver.disconnect()
  },
}
