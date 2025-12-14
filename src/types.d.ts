import type { BareItem, Key } from "structured-field-values"
import type { Component } from "vue"

import type { NONE } from "@/symbols"

type TranslatableHTMLString = Record<string, string>

type AmountUnit = "@" | "1" | "/" | "kg/kg" | "kg" | "kg/m³" | "m" | "m²" | "m/s" | "V" | "W" | "Pa" | "C" | "J" | "°C" | "rad" | "Hz" | "$" | "B" | "px" | "s"

type TimePrecision = "G" | "100M" | "10M" | "M" | "100k" | "10k" | "k" | "100y" | "10y" | "y" | "m" | "d" | "h" | "min" | "s"

export type RelSearchResult = {
  id: string
  count: number
  type: "rel"
}

export type AmountSearchResult = {
  id: string
  count: number
  type: "amount"
  unit: AmountUnit
}

export type TimeSearchResult = {
  id: string
  count: number
  type: "time"
}

export type StringSearchResult = {
  id: string
  count: number
  type: "string"
}

export type IndexSearchResult = {
  count: number
  type: "index"
}

export type SizeSearchResult = {
  count: number
  type: "size"
}

export type SearchFilterResult = RelSearchResult | AmountSearchResult | TimeSearchResult | StringSearchResult | IndexSearchResult | SizeSearchResult

export type SearchResult = {
  id: string
}

export type RelValuesResult = {
  id: string
  count: number
}

export type AmountValuesResult = {
  min: number
  count: number
}

export type TimeValuesResult = {
  min: string
  count: number
}

export type StringValuesResult = {
  str: string
  count: number
}

export type IndexValuesResult = {
  str: string
  count: number
}

export type SizeValuesResult = {
  min: number
  count: number
}

export type RelFilter = {
  prop: string
  value: string
}

export type RelNoneFilter = {
  prop: string
  none: true
}

export type AmountFilter = {
  prop: string
  unit: AmountUnit
  gte?: number
  lte?: number
}

export type AmountNoneFilter = {
  prop: string
  unit: AmountUnit
  none: true
}

export type TimeFilter = {
  prop: string
  gte?: string
  lte?: string
}

export type TimeNoneFilter = {
  prop: string
  none: true
}

export type StringFilter = {
  prop: string
  str: string
}

export type StringNoneFilter = {
  prop: string
  none: true
}

export type IndexFilter = {
  str: string
}

export type SizeFilter = {
  gte?: number
  lte?: number
}

export type SizeNoneFilter = {
  none: true
}

export type Filters =
  | {
      and: Filters[]
    }
  | {
      or: Filters[]
    }
  | {
      not: Filters
    }
  | { rel: RelFilter | RelNoneFilter }
  | { amount: AmountFilter | AmountNoneFilter }
  | { time: TimeFilter | TimeNoneFilter }
  | { str: StringFilter | StringNoneFilter }
  | { index: IndexFilter }
  | { size: SizeFilter | SizeNoneFilter }

export type RelFilterState = (string | typeof NONE)[]

export type AmountFilterState = null | typeof NONE | { gte?: number; lte?: number }

export type TimeFilterState = null | typeof NONE | { gte?: string; lte?: string }

export type StringFilterState = (string | typeof NONE)[]

export type IndexFilterState = string[]

export type SizeFilterState = null | typeof NONE | { gte?: number; lte?: number }

export type FiltersState = {
  rel: Record<string, RelFilterState>
  amount: Record<string, AmountFilterState>
  time: Record<string, TimeFilterState>
  str: Record<string, StringFilterState>
  index: IndexFilterState
  size: SizeFilterState
}

export type RelFilterStateChange = {
  type: "rel"
  id: string
  value: RelFilterState
}

export type AmountFilterStateChange = {
  type: "amount"
  id: string
  unit: AmountUnit
  value: AmountFilterState
}

export type TimeFilterStateChange = {
  type: "time"
  id: string
  value: TimeFilterState
}

export type StringFilterStateChange = {
  type: "string"
  id: string
  value: StringFilterState
}

export type IndexFilterStateChange = {
  type: "index"
  value: IndexFilterState
}

export type SizeFilterStateChange = {
  type: "size"
  value: SizeFilterState
}

export type FilterStateChange =
  | RelFilterStateChange
  | AmountFilterStateChange
  | TimeFilterStateChange
  | StringFilterStateChange
  | IndexFilterStateChange
  | SizeFilterStateChange

export type ServerSearchState = {
  s: string
  q: string
  p?: string
  filters?: Filters
  promptDone?: boolean
  promptCalls?: object[]
  promptError?: boolean
}

type TextProvider = {
  model: string
  maxContextLength: number
  maxResponseLength: number
  temperature: number
  seed?: number
  promptCaching?: boolean
  forceOutputJsonSchema?: boolean
}

type TextRecorderMessage = {
  role: string
  content?: string
  toolUseId?: string
  toolUseName?: string
  toolDuration?: number
  toolCalls?: TextRecorderCall[]
  isError?: boolean
  isRefusal?: boolean
}

type TextRecorderUsedTokens = {
  maxTotal: number
  maxResponse: number
  prompt: number
  response: number
  total: number
  cacheCreationInputTokens?: number
  cacheReadInputTokens?: number
}

type TextRecorderUsedTime = {
  prompt?: number
  response?: number
  total?: number
  apiCall: number
}

type TextRecorderCall = {
  id: string
  provider: TextProvider
  messages?: TextRecorderMessage[]
  usedTokens?: Record<string, TextRecorderUsedTokens>
  usedTime?: Record<string, TextRecorderUsedTime>
  duration?: number
}

export type ClientSearchState = {
  s: string
  q: string
  p?: string
  filters?: FiltersState
  promptDone?: boolean
  promptCalls?: TextRecorderCall[]
  promptError?: boolean
}

export type SearchStateCreateResponse = { s: string; q?: string; p?: string }

export type SiteContext = {
  domain: string
  build?: {
    version?: string
    buildTimestamp?: string
    revision?: string
  }
  index: string
  title: string
}

// Symbol is not generated by the server side, but we can easily support it here.
type ItemTypes = BareItem | ItemTypes[]

export type Metadata = Record<Key, ItemTypes>

export type QueryValues = Record<string, string | string[]>

export type QueryValuesWithOptional = Record<string, string | (string | null)[] | undefined | null>

export type StorageBeginUploadRequest = {
  size: number
  mediaType: string
  filename: string
}

export type StorageBeginUploadResponse = {
  session: string
}

export type DocumentCreateResponse = {
  id: string
}

export type DocumentBeginEditResponse = {
  session: string
  version: string
}

export type DocumentEndEditResponse = {
  changeset: string
}

export type DocumentBeginMetadata = {
  at: string
  id: string
  version: string
}

export type SearchViewType = "table" | "feed"

export type SelectButtonOption<T> = {
  name: string
  value: T
  icon?: {
    component: string | Component
    alt: string
  }
  disabled?: boolean
  progress?: number
}

// It is recursive.
export type Mutable<T> = {
  -readonly [k in keyof T]: Mutable<T[k]>
}

// It is not recursive.
type Required<T> = {
  [k in keyof T]-?: T[k]
}

// It is not recursive.
type Optional<T> = {
  [k in keyof T]+?: T[k]
}

export type Constructor<T> = new (json: object) => T
export type Constructee<C> = C extends Constructor<infer R> ? R : never
