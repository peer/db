import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

const KEY = Symbol.for("peerdb-search.registry.documentHeaderComponents")
type Holder = {
  [KEY]?: ShallowRef<Raw<Component>[]>
}
const g = globalThis as unknown as Holder
const documentHeaderComponents: ShallowRef<Raw<Component>[]> = (g[KEY] ??= shallowRef<Raw<Component>[]>([]))

// Registered components render at the top of the document view, above the document card (and thus
// on every tab), and receive the document id as the id prop. The document navigation (search
// session and previous/next ids) is available to them through useDocumentNavigation.
export function registerDocumentHeaderComponent(component: Component): void {
  documentHeaderComponents.value = [...documentHeaderComponents.value, markRaw(component)]
}

export function getDocumentHeaderComponents(): Readonly<ShallowRef<Raw<Component>[]>> {
  return documentHeaderComponents
}
