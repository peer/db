import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const searchResultComponents = shallowRef<Map<string, Raw<Component>>>(new Map())

export function registerSearchResultComponent(classId: string, component: Component): void {
  const updated = new Map(searchResultComponents.value)
  updated.set(classId, markRaw(component))
  searchResultComponents.value = updated
}

export function getSearchResultComponents(): Readonly<ShallowRef<Map<string, Raw<Component>>>> {
  return searchResultComponents
}
