import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, ref, shallowRef } from "vue"

const footerStartComponents = shallowRef<Raw<Component>[]>([])
const footerEndComponents = shallowRef<Raw<Component>[]>([])

export const creditsDisabled = ref(false)

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
