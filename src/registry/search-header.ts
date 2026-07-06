import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.searchHeaderComponents")
type Holder = {
  [KEY]?: ShallowRef<Raw<Component>[]>
}
const g = globalThis as unknown as Holder
const searchHeaderComponents: ShallowRef<Raw<Component>[]> = (g[KEY] ??= shallowRef<Raw<Component>[]>([]))

// Registered components render at the top of the search results column, above the results header,
// and receive the current search session as the searchSession prop.
export function registerSearchHeaderComponent(component: Component): void {
  searchHeaderComponents.value = [...searchHeaderComponents.value, markRaw(component)]
}

export function getSearchHeaderComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return searchHeaderComponents
}
