import type { Raw, ShallowRef } from "vue"

import type { GetDisplayLabel } from "@/types"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.displayLabelFunctions")
type Holder = { [k: symbol]: ShallowRef<Map<string, Raw<GetDisplayLabel>>> | undefined }
const g = globalThis as unknown as Holder
const displayLabelFunctions: ShallowRef<Map<string, Raw<GetDisplayLabel>>> = (g[KEY] ??= shallowRef<Map<string, Raw<GetDisplayLabel>>>(new Map()))

export function registerDisplayLabelFunction(classId: string, fn: GetDisplayLabel): void {
  const updated = new Map(displayLabelFunctions.value)
  updated.set(classId, markRaw(fn))
  displayLabelFunctions.value = updated
}

export function getDisplayLabelFunctions(): Readonly<ShallowRef<Map<string, Raw<GetDisplayLabel>>>> {
  return displayLabelFunctions
}
