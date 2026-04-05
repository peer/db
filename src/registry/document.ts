import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const documentComponents = shallowRef<Map<string, Raw<Component>>>(new Map())

export function registerDocumentComponent(classId: string, component: Component): void {
  const updated = new Map(documentComponents.value)
  updated.set(classId, markRaw(component))
  documentComponents.value = updated
}

export function getDocumentComponents(): Readonly<ShallowRef<Map<string, Raw<Component>>>> {
  return documentComponents
}
