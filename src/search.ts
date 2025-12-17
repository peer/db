import type { Ref, DeepReadonly } from "vue"
import type { Router } from "vue-router"

import type {
  Result,
  FilterResult,
  RelFilterResult,
  HistogramAmountResult,
  HistogramTimeResult,
  StringFilterResult,
  RelFilter,
  AmountFilter,
  TimeFilter,
  StringFilter,
  Filters,
  FiltersState,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
  ClientSearchSession,
  ServerSearchSession,
  AmountUnit,
  SearchSessionRef,
  CreateSearchSessionRequest,
} from "@/types"

import { ref, watch, readonly, onBeforeUnmount, computed } from "vue"
import { useRoute, useRouter } from "vue-router"
import { getURL, postJSON, getURLDirect } from "@/api"
import { encodeQuery, timestampToSeconds, anySignal } from "@/utils"
import { NONE } from "@/symbols"

export { NONE } from "@/symbols"

export const FILTERS_INITIAL_LIMIT = 10
export const FILTERS_INCREASE = 10

function filtersStateToFilters(filters: FiltersState | DeepReadonly<FiltersState> | undefined): Filters {
  const f: Filters = {
    and: [],
  }
  if (filters) {
    for (const [prop, values] of Object.entries(filters.rel)) {
      // TODO: Support also OR between values.
      for (const value of values) {
        if (value === NONE) {
          f.and.push({ rel: { prop, none: true } })
        } else {
          f.and.push({ rel: { prop, value } })
        }
      }
    }
    for (const [path, value] of Object.entries(filters.amount)) {
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
        f.and.push({ amount: { prop, unit: unit as AmountUnit, none: true } })
      } else {
        f.and.push({ amount: { prop, unit: unit as AmountUnit, ...value } })
      }
    }
    for (const [prop, value] of Object.entries(filters.time)) {
      if (!value) {
        continue
      }
      // TODO: Support also OR between value and none.
      if (value === NONE) {
        f.and.push({ time: { prop, none: true } })
      } else {
        f.and.push({ time: { prop, ...value } })
      }
    }
    for (const [prop, strings] of Object.entries(filters.str)) {
      // TODO: Support also OR between values.
      for (const str of strings) {
        if (str === NONE) {
          f.and.push({ str: { prop, none: true } })
        } else {
          f.and.push({ str: { prop, str } })
        }
      }
    }
  }
  return f
}

function clientToServerSearchSession(searchSession: ClientSearchSession | DeepReadonly<ClientSearchSession>): ServerSearchSession {
  const s: ServerSearchSession = {
    id: searchSession.id,
    version: searchSession.version,
    query: searchSession.query,
  }
  const filters = filtersStateToFilters(searchSession.filters)
  // TODO: Currently assumes only "and" filters are set.
  if ("and" in filters && filters.and.length > 0) {
    s.filters = filters
  }
  return s
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

function serverToClientSearchSession(searchSession: ServerSearchSession): ClientSearchSession {
  const s: ClientSearchSession = {
    id: searchSession.id,
    version: searchSession.version,
    query: searchSession.query,
  }
  if (searchSession.filters) {
    s.filters = filtersToFiltersState(searchSession.filters)
  }
  return s
}

export async function createSearchSession(router: Router, createSearchSessionRequest: CreateSearchSessionRequest, abortSignal: AbortSignal, progress: Ref<number>) {
  const payload: {
    query: string
    filters?: Filters
  } = {
    query: createSearchSessionRequest.query,
  }
  const filters = filtersStateToFilters(createSearchSessionRequest.filters)
  // TODO: Currently assumes only "and" filters are set.
  if ("and" in filters && filters.and.length > 0) {
    payload.filters = filters
  }
  const sessionRef = await postJSON<SearchSessionRef>(
    router.apiResolve({
      name: "SearchCreate",
    }).href,
    payload,
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return
  }
  await router.push({
    name: "SearchGet",
    params: {
      id: sessionRef.id,
    },
  })
}

export async function updateSearchSession(
  router: Router,
  searchSession: ClientSearchSession | DeepReadonly<ClientSearchSession>,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<SearchSessionRef | null> {
  const updatedSearchSessionRef = await postJSON<SearchSessionRef>(
    router.apiResolve({
      name: "SearchUpdate",
      params: {
        id: searchSession.id,
      },
    }).href,
    clientToServerSearchSession(searchSession),
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return null
  }
  if (updatedSearchSessionRef.id !== searchSession.id) {
    throw new Error(`unexpected search session ID change, new ${updatedSearchSessionRef.id}, old ${searchSession.id}`)
  }
  return updatedSearchSessionRef
}

export function useSearch(
  searchSessionRef: Ref<SearchSessionRef | null>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<Result[]>>
  total: DeepReadonly<Ref<number | null>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<Result>(el, progress, () => {
    if (!searchSessionRef.value) {
      return null
    }
    return router.apiResolve({
      name: "SearchResults",
      params: {
        id: searchSessionRef.value.id,
      },
      // TODO: Implement proper versioning.
      //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
      //       but the backend does not really use the parameter and always returns the latest version.
      query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
    }).href
  })
}

export function useFilters(
  searchSessionRef: Ref<SearchSessionRef | null>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<FilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<FilterResult>(el, progress, () => {
    if (!searchSessionRef.value) {
      return null
    }
    return router.apiResolve({
      name: "SearchFilters",
      params: {
        id: searchSessionRef.value.id,
      },
      // We should not really be passing a version here, it is not used by the API (currently it
      // is ignored and always the latest version is returned), but we pass it anyway so that
      // URL changes when version changes and search results are re-fetched.
      // TODO: Change this once we have proper support for versions.
      query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
    }).href
  })
}

function useSearchResults<T extends Result | FilterResult | RelSearchResult>(
  el: Ref<Element | null>,
  progress: Ref<number>,
  getURL: () => string | null,
): {
  results: DeepReadonly<Ref<T[]>>
  total: DeepReadonly<Ref<number | null>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const route = useRoute()

  const _results = ref<T[]>([]) as Ref<T[]>
  const _total = ref<number | null>(null)
  const _moreThanTotal = ref(false)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : (_results as unknown as Readonly<Ref<readonly DeepReadonly<T>[]>>)
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    getURL,
    async (newURL, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      _url.value = newURL || null

      // We want to eagerly remove any error.
      _error.value = null

      if (!newURL) {
        _results.value = []
        _total.value = null
        _moreThanTotal.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getSearchResults<T>(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSearchResults", newURL, err)
        _results.value = []
        _total.value = null
        _moreThanTotal.value = false
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results
      if (typeof data.total === "string") {
        if (data.total.endsWith("+")) {
          _moreThanTotal.value = true
          _total.value = parseInt(data.total.substring(0, data.total.length - 1))
        } else {
          // This should not really happen, but we still cover the case.
          _moreThanTotal.value = false
          _total.value = parseInt(data.total)
        }
      } else {
        _moreThanTotal.value = false
        _total.value = data.total
      }
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    moreThanTotal,
    error,
    url,
  }
}

export function useRelFilterValues(
  searchSessionRef: Ref<SearchSessionRef | null>,
  result: Ref<RelSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<RelFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<RelSearchResult>(el, progress, () => {
    const r = result.value
    if (!r.id || !r.type) {
      return null
    }
    if (r.type === "rel") {
      if (!searchSessionRef.value) {
        return null
      }
      return router.apiResolve({
        name: "SearchRelFilter",
        params: {
          id: searchSessionRef.value.id,
          prop: r.id,
        },
        // TODO: Implement proper versioning.
        //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
        //       but the backend does not really use the parameter and always returns the latest version.
        query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
      }).href
    } else {
      throw new Error(`unexpected type "${r.type}" for property "${r.id}"`)
    }
  })
}

export function useAmountHistogramValues(
  searchSessionRef: Ref<SearchSessionRef | null>,
  result: Ref<AmountSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<HistogramAmountResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<number | null>>
  max: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<HistogramAmountResult[]>([])
  const _total = ref<number | null>(null)
  const _min = ref<number | null>(null)
  const _max = ref<number | null>(null)
  const _interval = ref<number | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const min = import.meta.env.DEV ? readonly(_min) : _min
  const max = import.meta.env.DEV ? readonly(_max) : _max
  const interval = import.meta.env.DEV ? readonly(_interval) : _interval
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    () => {
      const r = result.value
      if (!r.id || !r.type) {
        return null
      }
      if (r.type === "amount") {
        if (!r.unit) {
          throw new Error(`property "${r.id}" is missing unit`)
        }
        if (!searchSessionRef.value) {
          return null
        }
        return router.apiResolve({
          name: "SearchAmountFilter",
          params: {
            id: searchSessionRef.value.id,
            prop: r.id,
            unit: r.unit,
          },
          // TODO: Implement proper versioning.
          //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
          //       but the backend does not really use the parameter and always returns the latest version.
          query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
        }).href
      } else {
        throw new Error(`unexpected type "${r.type}" for property "${r.id}"`)
      }
    },
    async (newURL, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      _url.value = newURL || null

      // We want to eagerly remove any error.
      _error.value = null

      if (!newURL) {
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getHistogramValues(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useAmountHistogramValues", newURL, err)
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results as HistogramAmountResult[]
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
    error,
    url,
  }
}

export function useTimeHistogramValues(
  searchSessionRef: Ref<SearchSessionRef | null>,
  result: Ref<TimeSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<HistogramTimeResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<bigint | null>>
  max: DeepReadonly<Ref<bigint | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<HistogramTimeResult[]>([])
  const _total = ref<number | null>(null)
  const _min = ref<bigint | null>(null)
  const _max = ref<bigint | null>(null)
  const _interval = ref<number | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const min = import.meta.env.DEV ? readonly(_min) : _min
  const max = import.meta.env.DEV ? readonly(_max) : _max
  const interval = import.meta.env.DEV ? readonly(_interval) : _interval
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    () => {
      const r = result.value
      if (!r.id || !r.type) {
        return null
      }
      if (r.type === "time") {
        if (!searchSessionRef.value) {
          return null
        }
        return router.apiResolve({
          name: "SearchTimeFilter",
          params: {
            id: searchSessionRef.value.id,
            prop: r.id,
          },
          // TODO: Implement proper versioning.
          //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
          //       but the backend does not really use the parameter and always returns the latest version.
          query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
        }).href
      } else {
        throw new Error(`unexpected type "${r.type}" for property "${r.id}"`)
      }
    },
    async (newURL, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      _url.value = newURL || null

      // We want to eagerly remove any error.
      _error.value = null

      if (!newURL) {
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getHistogramValues(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useTimeHistogramValues", newURL, err)
        _results.value = []
        _total.value = null
        _min.value = null
        _max.value = null
        _interval.value = null
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results as HistogramTimeResult[]
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
    error,
    url,
  }
}

export function useStringFilterValues(
  searchSessionRef: Ref<SearchSessionRef | null>,
  result: Ref<StringSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<StringFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<StringFilterResult[]>([])
  const _total = ref<number | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    () => {
      const r = result.value
      if (!r.id || !r.type) {
        return null
      }
      if (r.type === "string") {
        if (!searchSessionRef.value) {
          return null
        }
        return router.apiResolve({
          name: "SearchStringFilter",
          params: {
            id: searchSessionRef.value.id,
            prop: r.id,
          },
          // TODO: Implement proper versioning.
          //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
          //       but the backend does not really use the parameter and always returns the latest version.
          query: encodeQuery({ version: `${searchSessionRef.value.version}` }),
        }).href
      } else {
        throw new Error(`unexpected type "${r.type}" for property "${r.id}"`)
      }
    },
    async (newURL, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      _url.value = newURL || null

      // We want to eagerly remove any error.
      _error.value = null

      if (!newURL) {
        _results.value = []
        _total.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getStringValues<StringFilterResult>(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useStringFilterValues", newURL, err)
        _results.value = []
        _total.value = null
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results as StringFilterResult[]
      _total.value = data.total
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    error,
    url,
  }
}

async function getSearchResults<T extends Result | FilterResult | RelSearchResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{ results: T[]; total: number | string }> {
  const { doc, metadata } = await getURL(url, el, abortSignal, progress)
  if (abortSignal.aborted) {
    return { results: [], total: 0 }
  }

  if (!("total" in metadata)) {
    throw new Error(`"total" metadata is missing`)
  }
  const total = metadata["total"] as number | string
  return { results: doc, total } as { results: T[]; total: number | string }
}

async function getHistogramValues<T extends HistogramAmountResult | HistogramTimeResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{
  results: T[]
  total: number
  min?: string
  max?: string
  interval?: string
}> {
  const { doc, metadata } = await getURL(url, el, abortSignal, progress)
  if (abortSignal.aborted) {
    return { results: [], total: 0 }
  }

  if (!("total" in metadata)) {
    throw new Error(`"total" metadata is missing`)
  }
  const total = metadata["total"] as number
  const res = { results: doc, total: total } as {
    results: T[]
    total: number
    min?: string
    max?: string
    interval?: string
  }
  if ("min" in metadata) {
    res.min = metadata["min"] as string
  }
  if ("max" in metadata) {
    res.max = metadata["max"] as string
  }
  if ("interval" in metadata) {
    res.interval = metadata["interval"] as string
  }

  return res
}

async function getStringValues<T extends StringFilterResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{
  results: T[]
  total: number
}> {
  const { doc, metadata } = await getURL(url, el, abortSignal, progress)
  if (abortSignal.aborted) {
    return { results: [], total: 0 }
  }

  if (!("total" in metadata)) {
    throw new Error(`"total" metadata is missing`)
  }
  const total = metadata["total"] as number
  const res = { results: doc, total: total } as {
    results: T[]
    total: number
  }

  return res
}

export function useSearchSession(
  searchSessionRef: Ref<SearchSessionRef | null | undefined>,
  progress: Ref<number>,
): {
  searchSession: DeepReadonly<Ref<ClientSearchSession | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _searchSession = ref<ClientSearchSession | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const searchSession = import.meta.env.DEV ? readonly(_searchSession) : _searchSession
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    searchSessionRef,
    async (searchSessionRef, old, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }

      // We want to eagerly remove any error.
      _error.value = null

      if (!searchSessionRef) {
        _searchSession.value = null
        _url.value = null
        return
      }
      const newURL = router.apiResolve({
        name: "SearchGet",
        params: {
          id: searchSessionRef.id,
        },
      }).href
      _url.value = newURL
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getURLDirect<ServerSearchSession>(newURL, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSearchSession", newURL, err)
        _searchSession.value = null
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _searchSession.value = serverToClientSearchSession(data.doc)
    },
    {
      immediate: true,
    },
  )

  return {
    searchSession,
    error,
    url,
  }
}

export function useLocationAt(searchResults: Ref<DeepReadonly<Result[]>>, searchTotal: Ref<number | null>, visibles: ReadonlySet<string>) {
  const router = useRouter()
  const route = useRoute()

  const idToIndex = computed(() => {
    const map = new Map<string, number>()
    for (const [i, result] of searchResults.value.entries()) {
      map.set(result.id, i)
    }
    return map
  })

  const initialRouteName = route.name
  watch(
    () => {
      const sorted = Array.from(visibles)
      sorted.sort((a, b) => (idToIndex.value.get(a) ?? Infinity) - (idToIndex.value.get(b) ?? Infinity))
      return sorted[0]
    },
    async (topId, oldTopId, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      // Initial data has not yet been loaded, so we wait.
      if (!topId && searchTotal.value === null) {
        return
      }
      await router.replace({
        name: route.name as string,
        params: route.params,
        // We do not want to set an empty "at" query parameter.
        query: encodeQuery({ ...route.query, at: topId || undefined }),
        hash: route.hash,
      })
    },
    {
      immediate: true,
    },
  )
}
