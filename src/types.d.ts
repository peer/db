export type SearchResult = {
  _id: string
  _count?: number
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
      none: true
    }

export type FilterState = string[]

export type FiltersState = Record<string, FilterState>

export type ServerQuery = { s?: string; q?: string; filters?: Filters }

export type ClientQuery = { s?: string; at?: string; q?: string }
