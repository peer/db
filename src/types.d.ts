export type SearchResult = {
  _id: string
  _count?: number
  _type?: string
  _unit?: string
}

export type AmountHistogramResult = {
  min: number
  count: number
}

export type TimeHistogramResult = {
  min: string
  count: number
}

// TODO: Define the document better.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type PeerDBDocument = any

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

export type RelFilterState = string[]

export type AmountFilterState = null | "none" | { gte: number; lte: number }

export type TimeFilterState = null | "none" | { gte: string; lte: string }

export type FiltersState = { rel: Record<string, RelFilterState>; amount: Record<string, AmountFilterState>; time: Record<string, TimeFilterState> }

export type ServerQuery = { s?: string; q?: string; filters?: Filters }

export type ClientQuery = { s?: string; at?: string; q?: string }

export type Mutable<T> = {
  -readonly [k in keyof T]: Mutable<T[k]>
}
