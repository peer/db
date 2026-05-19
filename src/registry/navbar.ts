import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.navbarComponents")
type Holder = { [k: symbol]: ShallowRef<Raw<Component>[]> | undefined }
const g = globalThis as unknown as Holder
const navbarComponents: ShallowRef<Raw<Component>[]> =
  (g[KEY] ??= shallowRef<Raw<Component>[]>([]))

export function registerNavbarComponent(component: Component): void {
  navbarComponents.value = [...navbarComponents.value, markRaw(component)]
}

export function getNavbarComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return navbarComponents
}
