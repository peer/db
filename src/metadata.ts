import type { ItemTypes, Metadata } from "@/types"
import type { Item } from "structured-field-values"

// TODO: Consider moving to https://www.npmjs.com/package/structured-headers, once it supports parsing timestamps.
import { decodeDict } from "structured-field-values"

const metadataHeaderPrefix = ""
const metadataHeader = metadataHeaderPrefix + "Metadata"

function convertItem(item: Item): ItemTypes {
  if (item.params !== null) {
    throw new Error("params not supported")
  }

  if (Array.isArray(item.value)) {
    return item.value.map((i) => convertItem(i))
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
