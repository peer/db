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

export type SearchResult = RelSearchResult | AmountSearchResult | TimeSearchResult | StringSearchResult

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

type TranslatablePlainString = {
  en: string
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
  name: TranslatablePlainString
  score: number
  scores: Record<string, number>
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
  uncertaintyLower?: number
  uncertaintyUpper?: number
  unit: AmountUnit
}

type AmountRangeClaim = CoreClaim & {
  prop: DocumentReference
  lower: number
  upper: number
  uncertaintyLower?: number
  uncertaintyUpper?: number
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
  uncertaintyLower?: string
  uncertaintyUpper?: string
  precision: TimePrecision
}

type TimeRangeClaim = CoreClaim & {
  prop: DocumentReference
  lower: string
  upper: string
  uncertaintyLower?: string
  uncertaintyUpper?: string
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
  // Name and score are optional on the frontend because
  // search results do not have them initially.
  name?: TranslatablePlainString
  score?: number
  scores?: Record<string, number>
  mnemonic?: string
  active?: ClaimTypes
  inactive?: ClaimTypes
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

export type RelFilterState = (string | typeof NONE)[]

export type AmountFilterState = null | typeof NONE | { gte: number; lte: number }

export type TimeFilterState = null | typeof NONE | { gte: string; lte: string }

export type StringFilterState = (string | typeof NONE)[]

export type FiltersState = {
  rel: Record<string, RelFilterState>
  amount: Record<string, AmountFilterState>
  time: Record<string, TimeFilterState>
  str: Record<string, StringFilterState>
}

export type ServerQuery = { s?: string; q?: string; filters?: Filters }

export type ClientQuery = { s?: string; at?: string; q?: string }

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
