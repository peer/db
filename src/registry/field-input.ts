import type { Component, Raw, ShallowRef } from "vue"

import { markRaw, shallowRef } from "vue"

// Components a field can name through its FIELD_INPUT_COMPONENT claim to have the edit form render
// its value with, instead of the input the field's value type picks. The claim holds the name a
// component is registered under here, because a component cannot be imported from a name a document
// carries: the app owning the component registers it, and the claim only names it. That is also what
// lets an app built on PeerDB offer its own inputs - it registers them the same way, from its own
// entry point. Names are shared across everything registered, so an app registering a name PeerDB
// already uses replaces that input everywhere.
//
// A field's input component receives what the value type's own input would: the value model (plus
// the precision model for amounts and times), required / invalid / readonly, and the props the
// field's COMPONENT_PROPS sub-claim carries (as strings). It has to expose the ValidatedInput shape
// like the built-in inputs do, so the enclosing form can validate, focus, and revert it.
const KEY = Symbol.for("peerdb-search.registry.fieldInputComponents")
type Holder = { [k: symbol]: ShallowRef<Map<string, Raw<Component>>> | undefined }
const g = globalThis as unknown as Holder
const fieldInputComponents: ShallowRef<Map<string, Raw<Component>>> = (g[KEY] ??= shallowRef<Map<string, Raw<Component>>>(new Map()))

export function registerFieldInputComponent(name: string, component: Component): void {
  const updated = new Map(fieldInputComponents.value)
  updated.set(name, markRaw(component))
  fieldInputComponents.value = updated
}

export function getFieldInputComponents(): Readonly<ShallowRef<Map<string, Raw<Component>>>> {
  return fieldInputComponents
}

// PeerDB's own inputs, so a field can name any of them (e.g. the identifier input for a string
// field). The glob is resolved at build time into static imports, one per component file, so the
// set follows the directory instead of a list kept in step with it by hand. Each is registered
// under its file name without the extension ("InputString" for InputString.vue). The pattern is
// relative because a glob is resolved before aliases are, so an aliased pattern matches nothing.
const inputComponents = import.meta.glob<{ default: Component }>("../partials/input/*.vue", { eager: true })
for (const [path, module] of Object.entries(inputComponents)) {
  registerFieldInputComponent(path.slice(path.lastIndexOf("/") + 1, -".vue".length), module.default)
}
