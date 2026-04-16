import type { BareItem, Item } from "structured-field-values"

import type { Metadata } from "@/types"

// TODO: Consider moving to https://www.npmjs.com/package/structured-headers, once it supports parsing timestamps.
import { decodeDict } from "structured-field-values"

const metadataHeaderPrefix = ""
const metadataHeader = metadataHeaderPrefix + "Metadata"

function convertItem(item: Item): BareItem | BareItem[] {
  if (item.params !== null) {
    throw new Error("params not supported")
  }

  if (Array.isArray(item.value)) {
    // Inner lists in SFV contain only bare items, not nested lists.
    return item.value.map((i) => convertItem(i as Item) as BareItem)
  }

  return item.value
}

export function decodeMetadata(headers: Headers): Metadata {
  const header = headers.get(metadataHeader) || ""
  const result: Metadata = {}
  for (const [key, item] of Object.entries(decodeDict(header))) {
    result[key] = convertItem(item as Item)
  }
  return result
}
