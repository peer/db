import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const homeComponent = shallowRef<Raw<Component> | null>(null)

export function registerHomeComponent(component: Component): void {
  homeComponent.value = markRaw(component)
}

export function getHomeComponent(): Readonly<ShallowRef<Raw<Component> | null>> {
  return homeComponent
}
