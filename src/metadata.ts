import type { BareItem, Item } from "structured-field-values"

import type { Metadata } from "@/types"

// TODO: Consider moving to https://www.npmjs.com/package/structured-headers, once it supports parsing timestamps.
import { decodeDict, decodeList } from "structured-field-values"

import siteContext from "@/context"

// The canonical Metadata header's unprefixed name.
const metadataHeader = "Metadata"

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

// decodeMetadataNamed parses a single SFV-dictionary HTTP header into a
// flat metadata map. Pass the prefix-less header name; the
// MetadataHeaderPrefix the backend prepends is added automatically.
export function decodeMetadataNamed(headers: Headers, name: string): Metadata {
  const header = headers.get((siteContext.metadataHeaderPrefix || "") + name) || ""
  const result: Metadata = {}
  for (const [key, item] of Object.entries(decodeDict(header))) {
    result[key] = convertItem(item as Item)
  }
  return result
}

// decodeMetadataListNamed parses an SFV-list HTTP header into a flat array
// of items. An absent header (or an empty list value, which SFV serialises
// as the empty string) returns an empty array.
export function decodeMetadataListNamed(headers: Headers, name: string): (BareItem | BareItem[])[] {
  const header = headers.get((siteContext.metadataHeaderPrefix || "") + name) || ""
  if (header === "") {
    return []
  }
  return decodeList(header).map((item) => convertItem(item as Item))
}

export function decodeMetadata(headers: Headers): Metadata {
  return decodeMetadataNamed(headers, metadataHeader)
}
