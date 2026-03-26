import type { Component, Raw } from "vue"

import { markRaw, shallowRef } from "vue"

const searchResultComponents = shallowRef<Map<string, Raw<Component>>>(new Map())

export function registerSearchResultComponent(classId: string, component: Component): void {
  const updated = new Map(searchResultComponents.value)
  updated.set(classId, markRaw(component))
  searchResultComponents.value = updated
}

export function getSearchResultComponents() {
  return searchResultComponents
}
