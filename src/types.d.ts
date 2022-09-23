export type SearchResult = {
  _id: string
  _count?: number
  _type?: string
  _unit?: string
}

export type HistogramResult = {
  min: number
  count: number
}

// TODO: Define the document better.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type PeerDBDocument = any

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
  | {
      prop: string
      value: string
    }
  | {
      prop: string
      unit?: string
      none: true
    }
  | {
      prop: string
      unit: string
      gte: number
      lte: number
    }

export type RelFilterState = string[]

export type AmountFilterState = null | ["none"] | { gte: number; lte: number }

export type FiltersState = Record<string, RelFilterState | AmountFilterState>

export type ServerQuery = { s?: string; q?: string; filters?: Filters }

export type ClientQuery = { s?: string; at?: string; q?: string }
