import { ComponentPublicInstance } from "vue"
import { cloneDeep } from "lodash-es"
import { reactive, readonly, onBeforeUnmount } from "vue"

function isTruncated(el: Element): boolean {
  return el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight
}

function cellId(rowIndex: number, colIndex: number): string {
  return `${rowIndex}:${colIndex}`
}

export function useTruncationTracking(): {
  track: (rowIndex: number, columnIndex: number) => (el: Element | ComponentPublicInstance | null) => void
  cellUpdated: (el: Element) => void
  truncated: ReadonlyMap<number, ReadonlySet<number>>
  toggleRow: (rowIndex: number) => void
  expandedRows: ReadonlyMap<number, ReadonlySet<number>>
} {
  const idToElement = new Map<string, Element>()
  const elementToCell = new Map<Element, [number, number]>()
  const _truncated = reactive(new Map<number, Set<number>>())
  const truncated = import.meta.env.DEV ? readonly(_truncated) : _truncated
  const _expandedRows = reactive(new Map<number, Set<number>>())
  const expandedRows = import.meta.env.DEV ? readonly(_expandedRows) : _expandedRows

  function toggleRow(rowIndex: number) {
    if (!_expandedRows.has(rowIndex)) {
      const ids = cloneDeep(_truncated.get(rowIndex))

      if (!ids) {
        _expandedRows.set(rowIndex, new Set<number>())
        return
      }

      _expandedRows.set(rowIndex, ids)
      return
    }

    _expandedRows.delete(rowIndex)
  }

  function addTruncated(rowIndex: number, columnIndex: number) {
    if (!_truncated.has(rowIndex)) {
      _truncated.set(rowIndex, new Set<number>())
    }
    _truncated.get(rowIndex)!.add(columnIndex)
  }

  function deleteTruncated(rowIndex: number, columnIndex: number) {
    const row = _truncated.get(rowIndex)
    if (row) {
      row.delete(columnIndex)
      if (row.size === 0) {
        _truncated.delete(rowIndex)
      }
    }
  }

  function cellUpdated(el: Element) {
    const cell = elementToCell.get(el)
    if (cell) {
      const [rowIndex, columnIndex] = cell
      if (isTruncated(el)) {
        addTruncated(rowIndex, columnIndex)
      } else {
        deleteTruncated(rowIndex, columnIndex)
      }
    }
  }

  const observer = new ResizeObserver((entries) => {
    for (const entry of entries) {
      cellUpdated(entry.target)
    }
  })

  onBeforeUnmount(() => observer.disconnect())

  function trackElement(rowIndex: number, columnIndex: number, el: Element) {
    const id = cellId(rowIndex, columnIndex)
    const old = idToElement.get(id)
    if (old) {
      elementToCell.delete(old)
      observer.unobserve(old)
    }
    idToElement.set(id, el)
    elementToCell.set(el, [rowIndex, columnIndex])
    observer.observe(el)
  }

  function untrackElement(rowIndex: number, columnIndex: number) {
    const id = cellId(rowIndex, columnIndex)
    const old = idToElement.get(id)
    if (!old) return
    idToElement.delete(id)
    elementToCell.delete(old)
    observer.unobserve(old)
    deleteTruncated(rowIndex, columnIndex)
  }

  return {
    track: (rowIndex, columnIndex) => {
      return (el) => {
        if (el) {
          if (!(el instanceof Element)) {
            el = el.$el as Element
          }
          trackElement(rowIndex, columnIndex, el)
        } else {
          untrackElement(rowIndex, columnIndex)
        }
      }
    },
    cellUpdated,
    toggleRow,
    truncated,
    expandedRows,
  }
}
