import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.homeComponent")
type Holder = { [k: symbol]: ShallowRef<Raw<Component> | null> | undefined }
const g = globalThis as unknown as Holder
const homeComponent: ShallowRef<Raw<Component> | null> =
  (g[KEY] ??= shallowRef<Raw<Component> | null>(null))

export function registerHomeComponent(component: Component): void {
  homeComponent.value = markRaw(component)
}

export function getHomeComponent(): Readonly<ShallowRef<Raw<Component> | null>> {
  return homeComponent
}
