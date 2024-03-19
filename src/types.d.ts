import type { Router as VueRouter, RouteLocationRaw, RouteLocationNormalizedLoaded, RouteLocation } from "vue-router"
import type { BareItem, Key } from "structured-field-values"
import type { NONE } from "@/symbols"

export type RelSearchResult = {
  _id: string
  _count: number
  _type: "rel"
}

export type AmountSearchResult = {
  _id: string
  _count: number
  _type: "amount"
  _unit: AmountUnit
}

export type TimeSearchResult = {
  _id: string
  _count: number
  _type: "time"
}

export type StringSearchResult = {
  _id: string
  _count: number
  _type: "string"
}

export type IndexSearchResult = {
  _count: number
  _type: "index"
}

export type SizeSearchResult = {
  _count: number
  _type: "size"
}

export type SearchFilterResult = RelSearchResult | AmountSearchResult | TimeSearchResult | StringSearchResult | IndexSearchResult | SizeSearchResult

export type SearchResult = {
  _id: string
}

export type RelValuesResult = {
  _id: string
  _count: number
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
  _id: string
  confidence: number
  meta?: ClaimTypes
}

type DocumentReference = {
  _id: string
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
  _id: string
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

export type ServerQuery = { s?: string; q?: string; filters?: Filters }

export type ClientQuery = { s?: string; at?: string; q?: string }

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

export type Router = VueRouter & {
  apiResolve(
    to: RouteLocationRaw,
    currentLocation?: RouteLocationNormalizedLoaded,
  ): RouteLocation & {
    href: string
  }
}

// Symbol is not generated by the server side, but we can easily support it here.
type ItemTypes = BareItem | ItemTypes[]

export type Metadata = Record<Key, ItemTypes>

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
