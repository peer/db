import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const navbarComponents = shallowRef<Raw<Component>[]>([])

export function registerNavbarComponent(component: Component): void {
  navbarComponents.value = [...navbarComponents.value, markRaw(component)]
}

export function getNavbarComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return navbarComponents
}
