import type { Ref, DeepReadonly } from "vue"
import type { RouteLocationNormalizedLoaded, LocationQueryRaw } from "vue-router"
import type {
  SearchResult,
  SearchFilterResult,
  RelValuesResult,
  AmountValuesResult,
  TimeValuesResult,
  StringValuesResult,
  IndexValuesResult,
  SizeValuesResult,
  RelFilter,
  AmountFilter,
  TimeFilter,
  StringFilter,
  IndexFilter,
  SizeFilter,
  Filters,
  FiltersState,
  ClientQuery,
  ServerQuery,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
  Router,
} from "@/types"

import { ref, watch, readonly, onBeforeUnmount } from "vue"
import { useRoute } from "vue-router"
import { getURL, postURL } from "@/api"
import { timestampToSeconds, useRouter } from "@/utils"
import { NONE } from "@/symbols"

export { NONE } from "@/symbols"

export const SEARCH_INITIAL_LIMIT = 50
export const SEARCH_INCREASE = 50
export const FILTERS_INITIAL_LIMIT = 10
export const FILTERS_INCREASE = 10

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
  // TODO: Support also OR between values.
  for (const str of updatedState.index) {
    filters.and.push({ index: { str } })
  }
  if (updatedState.size) {
    // TODO: Support also OR between value and none.
    if (updatedState.size === NONE) {
      filters.and.push({ size: { none: true } })
    } else {
      filters.and.push({ size: { ...updatedState.size } })
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

function getSearchURL(router: Router, params: string): string {
  return router.apiResolve({ name: "DocumentSearch" }).href + "?" + params
}

export function useSearch(
  el: Ref<Element | null>,
  progress: Ref<number>,
  redirect: (query: LocationQueryRaw) => Promise<void | undefined>,
): {
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
} {
  const router = useRouter()
  const route = useRoute()

  return useSearchResults<SearchResult>(
    el,
    progress,
    () => {
      const params = new URLSearchParams()
      queryToData(route, params)
      return getSearchURL(router, params.toString())
    },
    redirect,
  )
}

export function useFilters(
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<SearchFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  return useSearchResults<SearchFilterResult>(
    el,
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
    null,
  )
}

function filtersToFiltersState(filters: Filters): FiltersState {
  if ("and" in filters) {
    const state: FiltersState = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
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
      for (const str of s.index) {
        if (!state.index.includes(str)) {
          state.index.push(str)
        }
      }
      if (s.size) {
        if (!state.size) {
          state.size = s.size
        } else {
          throw new Error(`duplicate size filter`)
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
        index: [],
        size: null,
      }
    } else {
      return {
        rel: {
          [filters.rel.prop]: [(filters.rel as RelFilter).value],
        },
        amount: {},
        time: {},
        str: {},
        index: [],
        size: null,
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
        index: [],
        size: null,
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
        index: [],
        size: null,
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
        index: [],
        size: null,
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
        index: [],
        size: null,
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
        index: [],
        size: null,
      }
    } else {
      return {
        rel: {},
        amount: {},
        time: {},
        str: {
          [filters.str.prop]: [(filters.str as StringFilter).str],
        },
        index: [],
        size: null,
      }
    }
  }
  if ("index" in filters) {
    return {
      rel: {},
      amount: {},
      time: {},
      str: {},
      index: [(filters.index as IndexFilter).str],
      size: null,
    }
  }
  if ("size" in filters) {
    if ("none" in filters.size && filters.size.none) {
      return {
        rel: {},
        amount: {},
        time: {},
        str: {},
        index: [],
        size: NONE,
      }
    } else {
      return {
        rel: {},
        amount: {},
        time: {},
        str: {},
        index: [],
        size: {
          gte: (filters.size as SizeFilter).gte,
          lte: (filters.size as SizeFilter).lte,
        },
      }
    }
  }
  throw new Error(`invalid filter`)
}

function useSearchResults<Type extends SearchResult | SearchFilterResult | RelSearchResult>(
  el: Ref<Element | null>,
  progress: Ref<number>,
  getURL: () => string | null,
  redirect?: ((query: LocationQueryRaw) => Promise<void | undefined>) | null,
): {
  results: DeepReadonly<Ref<Type[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
} {
  const route = useRoute()

  const _results = ref<Type[]>([]) as Ref<Type[]>
  const _total = ref<number | null>(null)
  const _filters = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {}, index: [], size: null })
  const _moreThanTotal = ref(false)
  const results = import.meta.env.DEV ? readonly(_results) : (_results as unknown as Readonly<Ref<readonly DeepReadonly<Type>[]>>)
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const filters = import.meta.env.DEV ? readonly(_filters) : _filters
  const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal

  const initialRouteName = route.name
  watch(
    getURL,
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _results.value = []
        _total.value = null
        _filters.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
        _moreThanTotal.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getSearchResults<Type>(url, el, controller.signal, progress)
      if (!("results" in data)) {
        _results.value = []
        _total.value = null
        _filters.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
        _moreThanTotal.value = false
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
      if (data.filters) {
        _filters.value = filtersToFiltersState(data.filters)
      } else {
        _filters.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
      }
    },
    {
      immediate: true,
    },
  )

  const controller = new AbortController()
  onBeforeUnmount(() => controller.abort())

  return {
    results,
    total,
    filters,
    moreThanTotal,
  }
}

export function useRelFilterValues(
  result: RelSearchResult,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<RelValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const data = useSearchResults<RelSearchResult>(
    el,
    progress,
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !result._id || !result._type) {
        return null
      }
      if (result._type === "rel") {
        return router.apiResolve({
          name: "DocumentSearchRelFilter",
          params: {
            s,
            prop: result._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${result._type}" for property "${result._id}"`)
      }
    },
    null,
  )
  return {
    results: data.results as DeepReadonly<Ref<RelSearchResult[]>>,
    total: data.total,
  }
}

export function useAmountHistogramValues(
  result: AmountSearchResult,
  el: Ref<Element | null>,
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
      if (!s || !result._id || !result._type) {
        return null
      }
      if (result._type === "amount") {
        if (!result._unit) {
          throw new Error(`property "${result._id}" is missing unit`)
        }
        return router.apiResolve({
          name: "DocumentSearchAmountFilter",
          params: {
            s,
            prop: result._id,
            unit: result._unit,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${result._type}" for property "${result._id}"`)
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
      const data = await getHistogramValues(url, el, controller.signal, progress)
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
  result: TimeSearchResult,
  el: Ref<Element | null>,
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
      if (!s || !result._id || !result._type) {
        return null
      }
      if (result._type === "time") {
        return router.apiResolve({
          name: "DocumentSearchTimeFilter",
          params: {
            s,
            prop: result._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${result._type}" for property "${result._id}"`)
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
      const data = await getHistogramValues(url, el, controller.signal, progress)
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
  result: StringSearchResult,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<StringValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<StringValuesResult[]>([])
  const _total = ref<number | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total

  const initialRouteName = route.name
  watch(
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !result._id || !result._type) {
        return null
      }
      if (result._type === "string") {
        return router.apiResolve({
          name: "DocumentSearchStringFilter",
          params: {
            s,
            prop: result._id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${result._type}" for property "${result._id}"`)
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
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getStringValues<StringValuesResult>(url, el, controller.signal, progress)
      _results.value = data.results as StringValuesResult[]
      _total.value = data.total
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
  }
}

export function useIndexFilterValues(
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<IndexValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<IndexValuesResult[]>([])
  const _total = ref<number | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total

  const initialRouteName = route.name
  watch(
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!screen) {
        return null
      }
      return router.apiResolve({
        name: "DocumentSearchIndexFilter",
        params: {
          s,
        },
      }).href
    },
    async (url, oldURL, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      if (!url) {
        _results.value = []
        _total.value = null
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getStringValues<IndexValuesResult>(url, el, controller.signal, progress)
      _results.value = data.results as IndexValuesResult[]
      _total.value = data.total
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
  }
}

export function useSizeHistogramValues(
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<SizeValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<number | null>>
  max: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<SizeValuesResult[]>([])
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
      if (!s) {
        return null
      }
      return router.apiResolve({
        name: "DocumentSearchSizeFilter",
        params: {
          s,
        },
      }).href
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
      const data = await getHistogramValues(url, el, controller.signal, progress)
      _results.value = data.results as SizeValuesResult[]
      _total.value = data.total
      _min.value = data.min != null ? parseInt(data.min) : null
      _max.value = data.max != null ? parseInt(data.max) : null
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

async function getSearchResults<Type extends SearchResult | SearchFilterResult | RelSearchResult>(
  url: string,
  el: Ref<Element | null>,
  abortSignal: AbortSignal,
  progress?: Ref<number>,
): Promise<{ results: Type[]; total: string; query?: string; filters?: Filters } | { q: string; s: string }> {
  const { doc, headers } = await getURL(url, el, abortSignal, progress)

  if (Array.isArray(doc)) {
    const total = headers.get("Peerdb-Total")
    if (total === null) {
      throw new Error("Peerdb-Total header is null")
    }
    const res = { results: doc, total } as { results: Type[]; total: string; query?: string; filters?: Filters }
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

async function getHistogramValues<Type extends AmountValuesResult | TimeValuesResult | SizeValuesResult>(
  url: string,
  el: Ref<Element | null>,
  abortSignal: AbortSignal,
  progress?: Ref<number>,
): Promise<{
  results: Type[]
  total: number
  min?: string
  max?: string
  interval?: string
}> {
  const { doc, headers } = await getURL(url, el, abortSignal, progress)

  const total = headers.get("Peerdb-Total")
  if (total === null) {
    throw new Error("Peerdb-Total header is null")
  }
  const res = { results: doc, total: parseInt(total) } as {
    results: Type[]
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

async function getStringValues<Type extends StringValuesResult | IndexValuesResult>(
  url: string,
  el: Ref<Element | null>,
  abortSignal: AbortSignal,
  progress?: Ref<number>,
): Promise<{
  results: Type[]
  total: number
}> {
  const { doc, headers } = await getURL(url, el, abortSignal, progress)

  const total = headers.get("Peerdb-Total")
  if (total === null) {
    throw new Error("Peerdb-Total header is null")
  }
  const res = { results: doc, total: parseInt(total) } as {
    results: Type[]
    total: number
  }

  return res
}

export function useSearchState(
  el: Ref<Element | null>,
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
      const data = await getSearchResults<SearchResult>(getSearchURL(router, params.toString()), el, controller.signal, progress)
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
