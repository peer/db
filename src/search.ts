import type { Ref, DeepReadonly } from "vue"
import type { Router, RouteLocationNormalizedLoaded, LocationQueryRaw } from "vue-router"
import type {
  SearchResult,
  RelValuesResult,
  AmountValuesResult,
  TimeValuesResult,
  StringValuesResult,
  PeerDBDocument,
  RelFilter,
  AmountFilter,
  TimeFilter,
  StringFilter,
  Filters,
  FiltersState,
  ClientQuery,
  ServerQuery,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
} from "@/types"

import { ref, watch, readonly, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import { assert } from "@vue/compiler-core"
import { getURL, postURL } from "@/api"
import { timestampToSeconds } from "@/utils"
import { NONE } from "@/symbols"

export { NONE } from "@/symbols"

const SEARCH_INITIAL_LIMIT = 50
const SEARCH_INCREASE = 50
const FILTERS_INITIAL_LIMIT = 10
const FILTERS_INCREASE = 10
// If the last increase would be equal or less than this number, just skip to the end.
const SKIP_TO_END = 2

function queryToData(route: RouteLocationNormalizedLoaded, data: FormData | URLSearchParams) {
  if (Array.isArray(route.query.s)) {
    if (route.query.s[0] != null) {
      data.set("s", route.query.s[0])
    }
  } else if (route.query.s != null) {
    data.set("s", route.query.s)
  }
  if (Array.isArray(route.query.q)) {
    if (route.query.q[0] != null) {
      data.set("q", route.query.q[0])
    }
  } else if (route.query.q != null) {
    data.set("q", route.query.q)
  }
}

export async function postSearch(router: Router, form: HTMLFormElement, progress: Ref<number>) {
  const query: ServerQuery = await postURL(
    router.apiResolve({
      name: "DocumentSearch",
    }).href,
    new FormData(form),
    progress,
  )
  await router.push({
    name: "DocumentSearch",
    query: {
      s: query.s,
      q: query.q,
    },
  })
}

export async function postFilters(router: Router, route: RouteLocationNormalizedLoaded, updatedState: FiltersState, progress: Ref<number>) {
  const filters: Filters = {
    and: [],
  }
  for (const [prop, values] of Object.entries(updatedState.rel)) {
    // TODO: Support also OR between values.
    for (const value of values) {
      if (value === NONE) {
        filters.and.push({ rel: { prop, none: true } })
      } else {
        filters.and.push({ rel: { prop, value } })
      }
    }
  }
  for (const [path, value] of Object.entries(updatedState.amount)) {
    if (!value) {
      continue
    }
    const segments = path.split("/")
    if (segments.length !== 2) {
      throw new Error(`invalid amount filter path: ${path}`)
    }
    const [prop, unit] = segments
    // TODO: Support also OR between value and none.
    if (value === NONE) {
      filters.and.push({ amount: { prop, unit, none: true } })
    } else {
      filters.and.push({ amount: { prop, unit, ...value } })
    }
  }
  for (const [prop, value] of Object.entries(updatedState.time)) {
    if (!value) {
      continue
    }
    // TODO: Support also OR between value and none.
    if (value === NONE) {
      filters.and.push({ time: { prop, none: true } })
    } else {
      filters.and.push({ time: { prop, ...value } })
    }
  }
  for (const [prop, strings] of Object.entries(updatedState.str)) {
    // TODO: Support also OR between values.
    for (const str of strings) {
      if (str === NONE) {
        filters.and.push({ str: { prop, none: true } })
      } else {
        filters.and.push({ str: { prop, str } })
      }
    }
  }
  const form = new FormData()
  queryToData(route, form)
  form.set("filters", JSON.stringify(filters))
  const updatedQuery: ServerQuery = await postURL(
    router.apiResolve({
      name: "DocumentSearch",
    }).href,
    form,
    progress,
  )
  if (route.query.s !== updatedQuery.s || route.query.q !== updatedQuery.q) {
    await router.push({
      name: "DocumentSearch",
      query: {
        s: updatedQuery.s,
        q: updatedQuery.q,
      },
    })
  }
}

function updateDocs(
  router: Router,
  docs: Ref<PeerDBDocument[]>,
  limit: number,
  results: readonly SearchResult[],
  priority: number,
  abortSignal: AbortSignal,
  progress: Ref<number>,
) {
  assert(limit <= results.length, `${limit} <= ${results.length}`)
  for (let i = docs.value.length; i < limit; i++) {
    progress.value += 1
    docs.value.push(results[i])
    getDocument(router, results[i], priority, abortSignal, progress)
      .then((data) => {
        docs.value[i] = data
      })
      .finally(() => {
        progress.value -= 1
      })
  }
}

function getSearchURL(router: Router, params: string): string {
  return router.apiResolve({ name: "DocumentSearch" }).href + "?" + params
}

export function useSearch(
  progress: Ref<number>,
  redirect: (query: LocationQueryRaw) => Promise<void | undefined>,
): {
  docs: DeepReadonly<Ref<(PeerDBDocument & SearchResult)[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  return useSearchResults(
    0,
    progress,
    () => {
      const params = new URLSearchParams()
      queryToData(route, params)
      return getSearchURL(router, params.toString())
    },
    SEARCH_INITIAL_LIMIT,
    SEARCH_INCREASE,
    redirect,
  )
}

export function useFilters(progress: Ref<number>): {
  docs: DeepReadonly<Ref<(PeerDBDocument & SearchResult)[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number | null>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  return useSearchResults(
    -1,
    progress,
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s) {
        return null
      }
      return router.apiResolve({
        name: "DocumentSearchFilters",
        params: {
          s,
        },
      }).href
    },
    FILTERS_INITIAL_LIMIT,
    FILTERS_INCREASE,
    null,
  )
}

function filtersToFiltersState(filters: Filters): FiltersState {
  if ("and" in filters) {
    const state: FiltersState = { rel: {}, amount: {}, time: {}, str: {} }
    for (const filter of filters.and) {
      const s = filtersToFiltersState(filter)
      for (const [prop, values] of Object.entries(s.rel)) {
        for (const v of values) {
          if (!state.rel[prop]) {
            state.rel[prop] = [v]
          } else if (!state.rel[prop].includes(v)) {
            state.rel[prop].push(v)
          }
        }
      }
      for (const [prop, value] of Object.entries(s.amount)) {
        if (!state.amount[prop]) {
          state.amount[prop] = value
        } else {
          throw new Error(`duplicate filter for the same amount property "${prop}"`)
        }
      }
      for (const [prop, value] of Object.entries(s.time)) {
        if (!state.time[prop]) {
          state.time[prop] = value
        } else {
          throw new Error(`duplicate filter for the same time property "${prop}"`)
        }
      }
      for (const [prop, strings] of Object.entries(s.str)) {
        for (const str of strings) {
          if (!state.str[prop]) {
            state.str[prop] = [str]
          } else if (!state.str[prop].includes(str)) {
            state.str[prop].push(str)
          }
        }
      }
    }
    return state
  }
  if ("not" in filters) {
    throw new Error(`not filter unsupported`)
  }
  if ("or" in filters) {
    throw new Error(`or filter unsupported`)
  }
  if ("rel" in filters) {
    if ("none" in filters.rel && filters.rel.none) {
      return {
        rel: {
          [filters.rel.prop]: [NONE],
        },
        amount: {},
        time: {},
        str: {},
      }
    } else {
      return {
        rel: {
          [filters.rel.prop]: [(filters.rel as RelFilter).value],
        },
        amount: {},
        time: {},
        str: {},
      }
    }
  }
  if ("amount" in filters) {
    if ("none" in filters.amount && filters.amount.none) {
      return {
        rel: {},
        amount: {
          [`${filters.amount.prop}/${filters.amount.unit}`]: NONE,
        },
        time: {},
        str: {},
      }
    } else {
      return {
        rel: {},
        amount: {
          [`${filters.amount.prop}/${filters.amount.unit}`]: {
            gte: (filters.amount as AmountFilter).gte,
            lte: (filters.amount as AmountFilter).lte,
          },
        },
        time: {},
        str: {},
      }
    }
  }
  if ("time" in filters) {
    if ("none" in filters.time && filters.time.none) {
      return {
        rel: {},
        amount: {},
        time: {
          [filters.time.prop]: NONE,
        },
        str: {},
      }
    } else {
      return {
        rel: {},
        amount: {},
        time: {
          [filters.time.prop]: {
            gte: (filters.time as TimeFilter).gte,
            lte: (filters.time as TimeFilter).lte,
          },
        },
        str: {},
      }
    }
  }
  if ("str" in filters) {
    if ("none" in filters.str && filters.str.none) {
      return {
        rel: {},
        amount: {},
        time: {},
        str: {
          [filters.str.prop]: [NONE],
        },
      }
    } else {
      return {
        rel: {},
        amount: {},
        time: {},
        str: {
          [filters.str.prop]: [(filters.str as StringFilter).str],
        },
      }
    }
  }
  throw new Error(`invalid filter`)
}

function useSearchResults(
  priority: number,
  progress: Ref<number>,
  getURL: () => string | null,
  initialLimit: number,
  increase: number,
  redirect?: ((query: LocationQueryRaw) => Promise<void | undefined>) | null,
): {
  docs: DeepReadonly<Ref<(PeerDBDocument & SearchResult)[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  let limit = 0

  const _docs = ref<(PeerDBDocument & SearchResult)[]>([])
  const _results = ref<SearchResult[]>([])
  const _total = ref<number | null>(null)
  const _filters = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {} })
  const _moreThanTotal = ref(false)
  const _hasMore = ref(false)
  const docs = import.meta.env.DEV ? readonly(_docs) : _docs
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const filters = import.meta.env.DEV ? readonly(_filters) : _filters
  const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal
  const hasMore = import.meta.env.DEV ? readonly(_hasMore) : _hasMore

  const initialRouteName = route.name
  watch(
    getURL,
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _docs.value = []
        _results.value = []
        _total.value = null
        _filters.value = { rel: {}, amount: {}, time: {}, str: {} }
        _moreThanTotal.value = false
        _hasMore.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getSearchResults(url, priority, controller.signal, progress)
      if (!("results" in data)) {
        _docs.value = []
        _results.value = []
        _total.value = null
        _filters.value = { rel: {}, amount: {}, time: {}, str: {} }
        _moreThanTotal.value = false
        _hasMore.value = false
        if (redirect) {
          await redirect(data)
        }
        return
      }
      _results.value = data.results
      if (data.total.endsWith("+")) {
        _moreThanTotal.value = true
        _total.value = parseInt(data.total.substring(0, data.total.length - 1))
      } else {
        _moreThanTotal.value = false
        _total.value = parseInt(data.total)
      }
      _docs.value = []
      if (data.filters) {
        _filters.value = filtersToFiltersState(data.filters)
      } else {
        _filters.value = { rel: {}, amount: {}, time: {}, str: {} }
      }
      limit = Math.min(initialLimit, results.value.length)
      // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
      if (limit + SKIP_TO_END >= results.value.length) {
        limit = results.value.length
      }
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, priority, controller.signal, progress)
    },
    {
      immediate: true,
    },
  )

  const controller = new AbortController()
  onBeforeUnmount(() => controller.abort())

  return {
    docs,
    results,
    total,
    filters,
    moreThanTotal,
    hasMore,
    loadMore: () => {
      limit = Math.min(limit + increase, results.value.length)
      // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
      if (limit + SKIP_TO_END >= results.value.length) {
        limit = results.value.length
      }
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, priority, controller.signal, progress)
    },
  }
}

export function useRelFilterValues(
  property: PeerDBDocument & RelSearchResult,
  progress: Ref<number>,
): {
  docs: DeepReadonly<Ref<(PeerDBDocument & RelValuesResult)[]>>
  results: DeepReadonly<Ref<RelValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  const data = useSearchResults(
    -2,
    progress,
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !property._id || !property._type) {
        return null
      }
      if (property._type === "rel") {
        return router.apiResolve({
          name: "DocumentSearchRelFilter",
          params: {
            s,
            prop: property._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${property._type}" for property "${property._id}"`)
      }
    },
    FILTERS_INITIAL_LIMIT,
    FILTERS_INCREASE,
    null,
  )
  return {
    docs: data.docs as DeepReadonly<Ref<(PeerDBDocument & RelSearchResult)[]>>,
    results: data.results as DeepReadonly<Ref<RelSearchResult[]>>,
    total: data.total,
    hasMore: data.hasMore,
    loadMore: data.loadMore,
  }
}

export function useAmountHistogramValues(
  property: PeerDBDocument & AmountSearchResult,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<AmountValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<number | null>>
  max: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<AmountValuesResult[]>([])
  const _total = ref<number | null>(null)
  const _min = ref<number | null>(null)
  const _max = ref<number | null>(null)
  const _interval = ref<number | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const min = import.meta.env.DEV ? readonly(_min) : _min
  const max = import.meta.env.DEV ? readonly(_max) : _max
  const interval = import.meta.env.DEV ? readonly(_interval) : _interval

  const initialRouteName = route.name
  watch(
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !property._id || !property._type) {
        return null
      }
      if (property._type === "amount") {
        if (!property._unit) {
          throw new Error(`property "${property._id}" is missing unit`)
        }
        return router.apiResolve({
          name: "DocumentSearchAmountFilter",
          params: {
            s,
            prop: property._id,
            unit: property._unit,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${property._type}" for property "${property._id}"`)
      }
    },
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getHistogramValues(url, -2, controller.signal, progress)
      _results.value = data.results as AmountValuesResult[]
      _total.value = data.total
      _min.value = data.min != null ? parseFloat(data.min) : null
      _max.value = data.max != null ? parseFloat(data.max) : null
      _interval.value = data.interval != null ? parseFloat(data.interval) : null
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    min,
    max,
    interval,
  }
}

export function useTimeHistogramValues(
  property: PeerDBDocument & TimeSearchResult,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<TimeValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<bigint | null>>
  max: DeepReadonly<Ref<bigint | null>>
  interval: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<TimeValuesResult[]>([])
  const _total = ref<number | null>(null)
  const _min = ref<bigint | null>(null)
  const _max = ref<bigint | null>(null)
  const _interval = ref<number | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const min = import.meta.env.DEV ? readonly(_min) : _min
  const max = import.meta.env.DEV ? readonly(_max) : _max
  const interval = import.meta.env.DEV ? readonly(_interval) : _interval

  const initialRouteName = route.name
  watch(
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !property._id || !property._type) {
        return null
      }
      if (property._type === "time") {
        return router.apiResolve({
          name: "DocumentSearchTimeFilter",
          params: {
            s,
            prop: property._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${property._type}" for property "${property._id}"`)
      }
    },
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getHistogramValues(url, -2, controller.signal, progress)
      _results.value = data.results as TimeValuesResult[]
      _total.value = data.total
      _min.value = data.min != null ? timestampToSeconds(data.min) : null
      _max.value = data.max != null ? timestampToSeconds(data.max) : null
      _interval.value = data.interval != null ? parseFloat(data.interval) : null
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    min,
    max,
    interval,
  }
}

export function useStringFilterValues(
  property: PeerDBDocument & StringSearchResult,
  progress: Ref<number>,
): {
  limitedResults: DeepReadonly<Ref<StringValuesResult[]>>
  results: DeepReadonly<Ref<StringValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  let limit = 0

  const _limitedResults = ref<StringValuesResult[]>([])
  const _results = ref<StringValuesResult[]>([])
  const _total = ref<number | null>(null)
  const _hasMore = ref(false)
  const limitedResults = import.meta.env.DEV ? readonly(_limitedResults) : _limitedResults
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const hasMore = import.meta.env.DEV ? readonly(_hasMore) : _hasMore

  const initialRouteName = route.name
  watch(
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !property._id || !property._type) {
        return null
      }
      if (property._type === "string") {
        return router.apiResolve({
          name: "DocumentSearchStringFilter",
          params: {
            s,
            prop: property._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${property._type}" for property "${property._id}"`)
      }
    },
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _limitedResults.value = []
        _results.value = []
        _total.value = null
        _hasMore.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getHistogramValues(url, -2, controller.signal, progress)
      _results.value = data.results as StringValuesResult[]
      _total.value = data.total
      limit = Math.min(FILTERS_INITIAL_LIMIT, results.value.length)
      // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
      if (limit + SKIP_TO_END >= results.value.length) {
        limit = results.value.length
      }
      _hasMore.value = limit < results.value.length
      _limitedResults.value = results.value.slice(0, limit)
    },
    {
      immediate: true,
    },
  )

  return {
    limitedResults,
    results,
    total,
    hasMore,
    loadMore: () => {
      limit = Math.min(limit + FILTERS_INCREASE, results.value.length)
      // If the last increase would be equal or less than SKIP_TO_END, just skip to the end.
      if (limit + SKIP_TO_END >= results.value.length) {
        limit = results.value.length
      }
      _hasMore.value = limit < results.value.length
      _limitedResults.value = results.value.slice(0, limit)
    },
  }
}

async function getSearchResults(
  url: string,
  priority: number,
  abortSignal: AbortSignal,
  progress?: Ref<number>,
): Promise<{ results: SearchResult[]; total: string; query?: string; filters?: Filters } | { q: string; s: string }> {
  const { doc, headers } = await getURL(url, priority, abortSignal, progress)

  if (Array.isArray(doc)) {
    const total = headers.get("Peerdb-Total")
    if (total === null) {
      throw new Error("Peerdb-Total header is null")
    }
    const res = { results: doc, total } as { results: SearchResult[]; total: string; query?: string; filters?: Filters }
    const query = headers.get("Peerdb-Query")
    if (query !== null) {
      res.query = decodeURIComponent(query)
    }
    const filters = headers.get("Peerdb-Filters")
    if (filters !== null) {
      res.filters = JSON.parse(decodeURIComponent(filters))
    }
    return res
  }

  return doc as { q: string; s: string }
}

async function getHistogramValues(
  url: string,
  priority: number,
  abortSignal: AbortSignal,
  progress?: Ref<number>,
): Promise<{ results: AmountValuesResult[] | TimeValuesResult[] | StringValuesResult[]; total: number; min?: string; max?: string; interval?: string }> {
  const { doc, headers } = await getURL(url, priority, abortSignal, progress)

  const total = headers.get("Peerdb-Total")
  if (total === null) {
    throw new Error("Peerdb-Total header is null")
  }
  const res = { results: doc, total: parseInt(total) } as {
    results: AmountValuesResult[] | TimeValuesResult[] | StringValuesResult[]
    total: number
    min?: string
    max?: string
    interval?: string
  }
  const min = headers.get("Peerdb-Min")
  if (min !== null) {
    res.min = min
  }
  const max = headers.get("Peerdb-Max")
  if (max !== null) {
    res.max = max
  }
  const interval = headers.get("Peerdb-Interval")
  if (interval !== null) {
    res.interval = interval
  }

  return res
}

export async function getDocument(router: Router, result: { _id: string }, priority: number, abortSignal: AbortSignal, progress?: Ref<number>): Promise<PeerDBDocument> {
  const { doc } = await getURL(
    router.apiResolve({
      name: "DocumentGet",
      params: {
        id: result._id,
      },
    }).href,
    priority,
    abortSignal,
    progress,
  )
  // We add any extra fields from the result (e.g., _count).
  // This also adds _id if it is not already present.
  return { ...doc, ...result }
}

export function useSearchState(
  redirect: (query: LocationQueryRaw) => Promise<void | undefined>,
  progress?: Ref<number>,
): {
  results: DeepReadonly<Ref<SearchResult[]>>
  query: DeepReadonly<Ref<ClientQuery>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<SearchResult[]>([])
  const _query = ref<ClientQuery>({})
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const query = import.meta.env.DEV ? readonly(_query) : _query

  const initialRouteName = route.name
  watch(
    () => {
      if (Array.isArray(route.query.s)) {
        return route.query.s[0]
      }
      return route.query.s
    },
    async (s, oldS, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!s) {
        _results.value = []
        _query.value = {}
        return
      }
      const params = new URLSearchParams()
      params.set("s", s)
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getSearchResults(getSearchURL(router, params.toString()), 0, controller.signal, progress)
      if (!("results" in data)) {
        _results.value = []
        _query.value = {}
        await redirect(data)
        return
      }
      _results.value = data.results
      // We know it is available because the query is without "q" parameter.
      _query.value = {
        s,
        // We set "at" here to undefined so that we control its order in the query string.
        at: undefined,
        q: data.query,
      }
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    query,
  }
}
