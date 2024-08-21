import type { Ref, DeepReadonly } from "vue"
import type { RouteLocationNormalizedLoaded, Router, RouteParams } from "vue-router"
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
  ClientSearchState,
  ServerSearchState,
  RelSearchResult,
  AmountSearchResult,
  TimeSearchResult,
  StringSearchResult,
  QueryValuesWithOptional,
} from "@/types"

import { ref, watch, readonly, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import { getURL, postURL } from "@/api"
import { encodeQuery, timestampToSeconds, anySignal } from "@/utils"
import { NONE } from "@/symbols"

export { NONE } from "@/symbols"

export const SEARCH_INITIAL_LIMIT = 50
export const SEARCH_INCREASE = 50
export const FILTERS_INITIAL_LIMIT = 10
export const FILTERS_INCREASE = 10

function queryToFormData(route: RouteLocationNormalizedLoaded): FormData {
  const form = new FormData()
  if (Array.isArray(route.query.q)) {
    if (route.query.q[0] != null) {
      form.set("q", route.query.q[0])
    }
  } else if (route.query.q != null) {
    form.set("q", route.query.q)
  }
  return form
}

export async function postSearch(router: Router, form: HTMLFormElement, abortSignal: AbortSignal, progress: Ref<number>) {
  const searchState = await postURL<ServerSearchState>(
    router.apiResolve({
      name: "SearchCreate",
    }).href,
    new FormData(form),
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return
  }
  await router.push({
    name: "SearchResults",
    params: {
      s: searchState.s,
    },
    query: encodeQuery({
      q: searchState.q,
    }),
  })
}

export async function postFilters(
  router: Router,
  route: RouteLocationNormalizedLoaded,
  s: string,
  updatedState: FiltersState,
  abortSignal: AbortSignal,
  progress: Ref<number>,
) {
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
  const form = queryToFormData(route)
  form.set("s", s)
  form.set("filters", JSON.stringify(filters))
  const updatedSearchState: ServerSearchState = await postURL(
    router.apiResolve({
      name: "SearchCreate",
    }).href,
    form,
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return
  }
  if (s !== updatedSearchState.s || route.query.q !== updatedSearchState.q) {
    await router.push({
      name: "SearchResults",
      params: {
        s: updatedSearchState.s,
      },
      query: encodeQuery({
        q: updatedSearchState.q,
      }),
    })
  }
}

function getSearchURL(router: Router, params: RouteParams, query: QueryValuesWithOptional): string {
  return router.apiResolve({
    name: "SearchResults",
    params,
    query: encodeQuery(query),
  }).href
}

export function useSearch(
  s: Ref<string>,
  el: Ref<Element | null>,
  progress: Ref<number>,
  redirect: (searchState: ServerSearchState) => Promise<void | undefined>,
): {
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  return useSearchResults<SearchResult>(
    el,
    progress,
    () => {
      return getSearchURL(
        router,
        {
          s: s.value,
        },
        {
          q: route.query.q,
        },
      )
    },
    redirect,
  )
}

export function useFilters(
  s: Ref<string>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<SearchFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<SearchFilterResult>(
    el,
    progress,
    () => {
      return router.apiResolve({
        name: "SearchFilters",
        params: {
          s: s.value,
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
  redirect?: ((searchState: ServerSearchState) => Promise<void | undefined>) | null,
): {
  results: DeepReadonly<Ref<Type[]>>
  total: DeepReadonly<Ref<number | null>>
  filters: DeepReadonly<Ref<FiltersState>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const route = useRoute()

  const _results = ref<Type[]>([]) as Ref<Type[]>
  const _total = ref<number | null>(null)
  const _filters = ref<FiltersState>({ rel: {}, amount: {}, time: {}, str: {}, index: [], size: null })
  const _moreThanTotal = ref(false)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : (_results as unknown as Readonly<Ref<readonly DeepReadonly<Type>[]>>)
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const filters = import.meta.env.DEV ? readonly(_filters) : _filters
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
        _filters.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
        _moreThanTotal.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getSearchResults<Type>(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSearchResults", newURL, err)
        _results.value = []
        _total.value = null
        _filters.value = { rel: {}, amount: {}, time: {}, str: {}, index: [], size: null }
        _moreThanTotal.value = false
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
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

  return {
    results,
    total,
    filters,
    moreThanTotal,
    error,
    url,
  }
}

export function useRelFilterValues(
  s: Ref<string>,
  result: Ref<RelSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<RelValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<RelSearchResult>(
    el,
    progress,
    () => {
      const r = result.value
      if (!r.id || !r.type) {
        return null
      }
      if (r.type === "rel") {
        return router.apiResolve({
          name: "SearchRelFilter",
          params: {
            s: s.value,
            prop: r.id,
          },
        }).href
      } else {
        throw new Error(`unexpected type "${r.type}" for property "${r.id}"`)
      }
    },
    null,
  )
}

export function useAmountHistogramValues(
  s: Ref<string>,
  result: Ref<AmountSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<AmountValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<number | null>>
  max: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<AmountValuesResult[]>([])
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
        return router.apiResolve({
          name: "SearchAmountFilter",
          params: {
            s: s.value,
            prop: r.id,
            unit: r.unit,
          },
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
    error,
    url,
  }
}

export function useTimeHistogramValues(
  s: Ref<string>,
  result: Ref<TimeSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<TimeValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<bigint | null>>
  max: DeepReadonly<Ref<bigint | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<TimeValuesResult[]>([])
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
        return router.apiResolve({
          name: "SearchTimeFilter",
          params: {
            s: s.value,
            prop: r.id,
          },
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
    error,
    url,
  }
}

export function useStringFilterValues(
  s: Ref<string>,
  result: Ref<StringSearchResult>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<StringValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<StringValuesResult[]>([])
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
        return router.apiResolve({
          name: "SearchStringFilter",
          params: {
            s: s.value,
            prop: r.id,
          },
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
        data = await getStringValues<StringValuesResult>(newURL, el, signal, progress)
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
    error,
    url,
  }
}

export function useIndexFilterValues(
  s: Ref<string>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<IndexValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<IndexValuesResult[]>([])
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
      return router.apiResolve({
        name: "SearchIndexFilter",
        params: {
          s: s.value,
        },
      }).href
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
        data = await getStringValues<IndexValuesResult>(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useIndexFilterValues", newURL, err)
        _results.value = []
        _total.value = null
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
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
    error,
    url,
  }
}

export function useSizeHistogramValues(
  s: Ref<string>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<SizeValuesResult[]>>
  total: DeepReadonly<Ref<number | null>>
  min: DeepReadonly<Ref<number | null>>
  max: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<SizeValuesResult[]>([])
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
      return router.apiResolve({
        name: "SearchSizeFilter",
        params: {
          s: s.value,
        },
      }).href
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
        data = await getHistogramValues(newURL, el, controller.signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSizeHistogramValues", newURL, err)
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
    error,
    url,
  }
}

async function getSearchResults<T extends SearchResult | SearchFilterResult | RelSearchResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{ results: T[]; total: number | string; query?: string; filters?: Filters } | ServerSearchState> {
  const { doc, metadata } = await getURL(url, el, abortSignal, progress)
  if (abortSignal.aborted) {
    return { q: "", s: "" }
  }

  if (Array.isArray(doc)) {
    if (!("total" in metadata)) {
      throw new Error(`"total" metadata is missing`)
    }
    const total = metadata["total"] as number | string
    const res = { results: doc, total } as { results: T[]; total: number | string; query?: string; filters?: Filters }
    if ("query" in metadata) {
      res.query = metadata["query"] as string
    }
    if ("filters" in metadata) {
      res.filters = JSON.parse(metadata["filters"] as string)
    }
    return res
  }

  return doc as { q: string; s: string }
}

async function getHistogramValues<Type extends AmountValuesResult | TimeValuesResult | SizeValuesResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{
  results: Type[]
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
    results: Type[]
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

async function getStringValues<Type extends StringValuesResult | IndexValuesResult>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<{
  results: Type[]
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
    results: Type[]
    total: number
  }

  return res
}

export function useSearchState(
  s: Ref<string | null | undefined>,
  el: Ref<Element | null>,
  redirect: (searchState: ServerSearchState) => Promise<void | undefined>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<SearchResult[]>>
  searchState: DeepReadonly<Ref<ClientSearchState>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<SearchResult[]>([])
  const _searchState = ref<ClientSearchState>({})
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const searchState = import.meta.env.DEV ? readonly(_searchState) : _searchState
  const error = import.meta.env.DEV ? readonly(_error) : _error
  const url = import.meta.env.DEV ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    s,
    async (s, oldS, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }

      // We want to eagerly remove any error.
      _error.value = null

      if (!s) {
        _results.value = []
        _searchState.value = {}
        _url.value = null
        return
      }
      const newURL = getSearchURL(router, { s }, {})
      _url.value = newURL
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const signal = anySignal(mainController.signal, controller.signal)
      let data
      try {
        data = await getSearchResults<SearchResult>(newURL, el, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSearchState", newURL, err)
        _results.value = []
        _searchState.value = {}
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      if (!("results" in data)) {
        _results.value = []
        _searchState.value = {}
        await redirect(data)
        return
      }
      _results.value = data.results
      _searchState.value = {
        s, // If "results" were returned, then s is valid.
        q: data.query,
      }
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    searchState,
    error,
    url,
  }
}
