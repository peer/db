import type { BareItem, Key } from "structured-field-values"
import type { NONE } from "@/symbols"

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

type TranslatableHTMLString = {
  en: string
}

type AmountUnit = "@" | "1" | "/" | "kg/kg" | "kg" | "kg/m³" | "m" | "m²" | "m/s" | "V" | "W" | "Pa" | "C" | "J" | "°C" | "rad" | "Hz" | "$" | "B" | "px" | "s"

type TimePrecision = "G" | "100M" | "10M" | "M" | "100k" | "10k" | "k" | "100y" | "10y" | "y" | "m" | "d" | "h" | "min" | "s"

type CoreClaim = {
  id: string
  confidence: number
  meta?: ClaimTypes
}

type DocumentReference = {
  id: string
  score: number
}

type IdentifierClaim = CoreClaim & {
  prop: DocumentReference
  id: string
}

type ReferenceClaim = CoreClaim & {
  prop: DocumentReference
  iri: string
}

type TextClaim = CoreClaim & {
  prop: DocumentReference
  html: TranslatableHTMLString
}

type StringClaim = CoreClaim & {
  prop: DocumentReference
  string: string
}

type AmountClaim = CoreClaim & {
  prop: DocumentReference
  amount: number
  unit: AmountUnit
}

type AmountRangeClaim = CoreClaim & {
  prop: DocumentReference
  lower: number
  upper: number
  unit: AmountUnit
}

type RelationClaim = CoreClaim & {
  prop: DocumentReference
  to: DocumentReference
}

type FileClaim = CoreClaim & {
  prop: DocumentReference
  type: string
  url: string
  preview?: string[]
}

type NoValueClaim = CoreClaim & {
  prop: DocumentReference
}

type UnknownValueClaim = CoreClaim & {
  prop: DocumentReference
}

type TimeClaim = CoreClaim & {
  prop: DocumentReference
  timestamp: string
  precision: TimePrecision
}

type TimeRangeClaim = CoreClaim & {
  prop: DocumentReference
  lower: string
  upper: string
  precision: TimePrecision
}

type ClaimTypes = {
  id?: IdentifierClaim[]
  ref?: ReferenceClaim[]
  text?: TextClaim[]
  string?: StringClaim[]
  amount?: AmountClaim[]
  amountRange?: AmountRangeClaim[]
  rel?: RelationClaim[]
  file?: FileClaim[]
  none?: NoValueClaim[]
  unknown?: UnknownValueClaim[]
  time?: TimeClaim[]
  timeRange?: TimeRangeClaim[]
}

type Claim =
  | IdentifierClaim
  | ReferenceClaim
  | TextClaim
  | StringClaim
  | AmountClaim
  | AmountRangeClaim
  | RelationClaim
  | FileClaim
  | NoValueClaim
  | UnknownValueClaim
  | TimeClaim
  | TimeRangeClaim

export type PeerDBDocument = {
  id: string
  // Score is optional on the frontend because
  // search results do not have it initially.
  score?: number
  scores?: Record<string, number>
  mnemonic?: string
  claims?: ClaimTypes
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
  unit: string
  gte: number
  lte: number
}

export type AmountNoneFilter = {
  prop: string
  unit: string
  none: true
}

export type TimeFilter = {
  prop: string
  gte: string
  lte: string
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
  gte: number
  lte: number
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

export type AmountFilterState = null | typeof NONE | { gte: number; lte: number }

export type TimeFilterState = null | typeof NONE | { gte: string; lte: string }

export type StringFilterState = (string | typeof NONE)[]

export type IndexFilterState = string[]

export type SizeFilterState = null | typeof NONE | { gte: number; lte: number }

export type FiltersState = {
  rel: Record<string, RelFilterState>
  amount: Record<string, AmountFilterState>
  time: Record<string, TimeFilterState>
  str: Record<string, StringFilterState>
  index: IndexFilterState
  size: SizeFilterState
}

export type ServerSearchState = { s: string; q: string }

export type ClientSearchState = { s?: string; q?: string }

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

export type BeginUploadRequest = {
  size: number
  mediaType: string
  filename: string
}

export type BeginUploadResponse = {
  session: string
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
