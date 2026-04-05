import type { Raw, ShallowRef } from "vue"

import type { GetDisplayLabel } from "@/types"

import { markRaw, shallowRef } from "vue"

const displayLabelFunctions = shallowRef<Map<string, Raw<GetDisplayLabel>>>(new Map())

export function registerDisplayLabelFunction(classId: string, fn: GetDisplayLabel): void {
  const updated = new Map(displayLabelFunctions.value)
  updated.set(classId, markRaw(fn))
  displayLabelFunctions.value = updated
}

export function getDisplayLabelFunctions(): Readonly<ShallowRef<Map<string, Raw<GetDisplayLabel>>>> {
  return displayLabelFunctions
}
