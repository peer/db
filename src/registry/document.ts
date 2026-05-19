import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.documentComponents")
type Holder = { [k: symbol]: ShallowRef<Map<string, Raw<Component>>> | undefined }
const g = globalThis as unknown as Holder
const documentComponents: ShallowRef<Map<string, Raw<Component>>> =
  (g[KEY] ??= shallowRef<Map<string, Raw<Component>>>(new Map()))

export function registerDocumentComponent(classId: string, component: Component): void {
  const updated = new Map(documentComponents.value)
  updated.set(classId, markRaw(component))
  documentComponents.value = updated
}

export function getDocumentComponents(): Readonly<ShallowRef<Map<string, Raw<Component>>>> {
  return documentComponents
}
