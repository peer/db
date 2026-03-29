import type { Component, Raw } from "vue"

import { markRaw, shallowRef } from "vue"

const navbarComponents = shallowRef<Raw<Component>[]>([])

export function registerNavbarComponent(component: Component): void {
  navbarComponents.value = [...navbarComponents.value, markRaw(component)]
}

export function getNavbarComponents() {
  return navbarComponents
}
