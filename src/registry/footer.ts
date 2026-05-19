import type { Component, Raw, Ref, ShallowRef } from "vue"

import { markRaw, ref, shallowRef } from "vue"

const START_KEY = Symbol.for("peerdb-search.registry.footerStartComponents")
const END_KEY = Symbol.for("peerdb-search.registry.footerEndComponents")
const CREDITS_KEY = Symbol.for("peerdb-search.registry.creditsDisabled")
type Holder = {
  [START_KEY]?: ShallowRef<Raw<Component>[]>
  [END_KEY]?: ShallowRef<Raw<Component>[]>
  [CREDITS_KEY]?: Ref<boolean>
}
const g = globalThis as unknown as Holder
const footerStartComponents: ShallowRef<Raw<Component>[]> =
  (g[START_KEY] ??= shallowRef<Raw<Component>[]>([]))
const footerEndComponents: ShallowRef<Raw<Component>[]> =
  (g[END_KEY] ??= shallowRef<Raw<Component>[]>([]))

export const creditsDisabled: Ref<boolean> = (g[CREDITS_KEY] ??= ref(false))

export function registerFooterStartComponent(component: Component): void {
  footerStartComponents.value = [...footerStartComponents.value, markRaw(component)]
}

export function registerFooterEndComponent(component: Component): void {
  footerEndComponents.value = [...footerEndComponents.value, markRaw(component)]
}

export function getFooterStartComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return footerStartComponents
}

export function getFooterEndComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return footerEndComponents
}
