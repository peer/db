import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.searchResultComponents")
type Holder = { [k: symbol]: ShallowRef<Map<string, Raw<Component>>> | undefined }
const g = globalThis as unknown as Holder
const searchResultComponents: ShallowRef<Map<string, Raw<Component>>> =
  (g[KEY] ??= shallowRef<Map<string, Raw<Component>>>(new Map()))

export function registerSearchResultComponent(classId: string, component: Component): void {
  const updated = new Map(searchResultComponents.value)
  updated.set(classId, markRaw(component))
  searchResultComponents.value = updated
}

export function getSearchResultComponents(): Readonly<ShallowRef<Map<string, Raw<Component>>>> {
  return searchResultComponents
}
