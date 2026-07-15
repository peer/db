import type { ShallowRef } from "vue"

import { shallowRef } from "vue"

// These registries let a consumer configure which document properties SearchResult.vue uses for
// the description, the tags, and the preview images. Each registry is a list of property IDs, empty
// by default. While a registry is empty the consumer falls back to its built-in defaults; as soon
// as a property is registered the registry fully replaces those defaults (they are no longer used).

type Holder = { [k: symbol]: ShallowRef<string[]> | undefined }

function propertyListRegistry(key: string): ShallowRef<string[]> {
  const g = globalThis as unknown as Holder
  return (g[Symbol.for(key)] ??= shallowRef<string[]>([]))
}

function registerProperty(registry: ShallowRef<string[]>, propertyId: string): void {
  if (!registry.value.includes(propertyId)) {
    registry.value = [...registry.value, propertyId]
  }
}

// Description properties. The HTML claims of these properties (the first match, resolved by
// language) are shown as the result's description. Only HTML claims are used. Empty by default; the
// consumer falls back to DESCRIPTION while empty.
const descriptionProperties = propertyListRegistry("peerdb-search.registry.descriptionProperties")

export function registerDescriptionProperty(propertyId: string): void {
  registerProperty(descriptionProperties, propertyId)
}

export function getDescriptionProperties(): Readonly<ShallowRef<string[]>> {
  return descriptionProperties
}

// Tags properties. The ref, identifier ("id"), and string claims of these properties are shown as
// tags: ref claims render as the referenced document's label, identifier and string claims render as
// their literal value. Empty by default; the consumer falls back to INSTANCE_OF and SUBCLASS_OF
// while empty.
const tagsProperties = propertyListRegistry("peerdb-search.registry.tagsProperties")

export function registerTagsProperty(propertyId: string): void {
  registerProperty(tagsProperties, propertyId)
}

export function getTagsProperties(): Readonly<ShallowRef<string[]>> {
  return tagsProperties
}

// Preview properties. The link claims of these properties are shown as preview images. Only link
// claims are used. Empty by default, which means no preview.
const previewProperties = propertyListRegistry("peerdb-search.registry.previewProperties")

export function registerPreviewProperty(propertyId: string): void {
  registerProperty(previewProperties, propertyId)
}

export function getPreviewProperties(): Readonly<ShallowRef<string[]>> {
  return previewProperties
}
