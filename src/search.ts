import type { DeepReadonly, InjectionKey, Ref } from "vue"
import type { LocationQuery, Router } from "vue-router"

import type {
  CreateSearchSessionRequest,
  CreateSearchSessionResponse,
  Filter,
  FilterResult,
  HasFilterResult,
  HistogramAmountResult,
  HistogramTimeResult,
  QueryValues,
  RefFilterResult,
  Result,
  SearchSession,
  SearchSessionData,
  SearchSessionRef,
  SearchShortcutRequest,
  UpdateSearchSessionResponse,
} from "@/types"

import { computed, onBeforeUnmount, readonly, ref, watch } from "vue"
import { stringifyQuery, useRoute, useRouter } from "vue-router"

import { getURL, getURLDirect, postJSON } from "@/api"
import { anySignal, encodeQuery } from "@/utils"

export const FILTERS_INITIAL_LIMIT = 10
export const FILTERS_INCREASE = 10

// PrefilterPayload is a single reference prefilter derived from a search shortcut query: a property
// path and the target value ids it constrains to.
export type PrefilterPayload = { prop: string[]; to: { id: string }[] }

// SearchShortcutController is provided by the SearchGet view so navbar search shortcut buttons can
// toggle the current session's prefilters in place instead of navigating. It is absent outside SearchGet.
export type SearchShortcutController = {
  prefilters: Readonly<Ref<DeepReadonly<Filter[]> | undefined>>
  // applyPrefilters replaces the session's prefilters with payloads (generating Base/ID for each), or
  // clears them when payloads is null or empty.
  applyPrefilters: (payloads: PrefilterPayload[] | null) => Promise<void>
}

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const searchShortcutControllerKey: InjectionKey<SearchShortcutController> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-search-shortcut-controller") : Symbol()

// queryToPrefilterPayloads maps a search shortcut query (the SearchShortcut route query) to reference
// prefilter payloads, mirroring the backend parseSearchShortcutQuery: each key is a property (split on
// ":" for a nested sub-reference), each value is a target id, and the reserved "reverse" and "language"
// keys are skipped.
export function queryToPrefilterPayloads(query: QueryValues): PrefilterPayload[] {
  const payloads: PrefilterPayload[] = []
  for (const [key, value] of Object.entries(query)) {
    if (key === "reverse" || key === "language") {
      continue
    }
    const prop = key.split(":")
    const values = Array.isArray(value) ? value : [value]
    payloads.push({ prop, to: values.map((id) => ({ id })) })
  }
  return payloads
}

// prefilterSignature normalizes a property path and its target ids to a stable string so two prefilters
// can be compared ignoring filter id/base and value order.
function prefilterSignature(prop: readonly string[], ids: readonly string[]): string {
  return prop.join(":") + "=" + [...ids].sort().join(",")
}

// prefiltersMatch reports whether the session's prefilters are exactly the set of reference prefilters
// described by payloads (order-independent). Non-reference prefilters never match a shortcut payload.
export function prefiltersMatch(prefilters: DeepReadonly<Filter[]> | undefined, payloads: PrefilterPayload[]): boolean {
  const have: string[] = []
  for (const f of prefilters ?? []) {
    if (!("ref" in f)) {
      return false
    }
    have.push(
      prefilterSignature(
        f.prop,
        (f.ref.to ?? []).map((t) => t.id),
      ),
    )
  }
  const want = payloads.map((p) =>
    prefilterSignature(
      p.prop,
      p.to.map((t) => t.id),
    ),
  )
  if (have.length !== want.length) {
    return false
  }
  have.sort()
  want.sort()
  return have.every((sig, i) => sig === want[i])
}

// createSearchSession creates a new search session for the query and navigates to it.
export async function createSearchSession(router: Router, query: string, language: string, abortSignal: AbortSignal, progress: Ref<number>): Promise<void> {
  const response = await postJSON<CreateSearchSessionResponse>(
    router.apiResolve({
      name: "SearchCreate",
    }).href,
    { query, language } satisfies CreateSearchSessionRequest,
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return
  }
  await router.push({
    name: "SearchGet",
    params: {
      id: response.id,
    },
  })
}

// createShortcutSession creates a session from the search shortcut navigates to it.
export async function createShortcutSession(router: Router, query: LocationQuery, language: string, abortSignal: AbortSignal, progress: Ref<number>): Promise<void> {
  // We add the current UI language to the shortcut query unless it already sets one explicitly.
  const augmentedQuery: LocationQuery = { ...query }
  if (!("language" in augmentedQuery)) {
    augmentedQuery.language = language
  }
  const response = await postJSON<CreateSearchSessionResponse>(
    router.apiResolve({
      name: "SearchShortcut",
    }).href,
    { query: stringifyQuery(augmentedQuery) } satisfies SearchShortcutRequest,
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return
  }
  await router.replace({
    name: "SearchGet",
    params: {
      id: response.id,
    },
  })
}

export async function updateSearchSession(
  router: Router,
  sessionId: string,
  searchData: DeepReadonly<SearchSessionData>,
  abortSignal: AbortSignal,
  progress: Ref<number>,
): Promise<UpdateSearchSessionResponse | null> {
  const payload: DeepReadonly<SearchSessionData> = {
    view: searchData.view,
    query: searchData.query,
    ...(searchData.language ? { language: searchData.language } : {}),
    ...(searchData.filters && searchData.filters.length > 0 ? { filters: searchData.filters } : {}),
    ...(searchData.prefilters && searchData.prefilters.length > 0 ? { prefilters: searchData.prefilters } : {}),
    ...(searchData.reverse ? { reverse: searchData.reverse } : {}),
  }
  const response = await postJSON<UpdateSearchSessionResponse>(
    router.apiResolve({
      name: "SearchUpdate",
      params: {
        id: sessionId,
      },
    }).href,
    payload,
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return null
  }
  return response
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

// Composite key identifying a filter result by its type, properties, and (for amount filters) unit.
// Filter results are deduplicated by this key, so it also uniquely identifies a result when rendering
// a list of them. The same prop can carry filters of different types (for example a ref filter and a
// has filter), so the type must be part of the key.
export function filterResultKey(filter: DeepReadonly<FilterResult>): string {
  const unit = filter.type === "amount" ? (filter.unit ?? "") : ""
  return `${filter.type}/${filter.props?.join("/") ?? ""}/${unit}`
}

export function useFilters(
  searchSessionRef: Ref<SearchSessionRef>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<FilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  const {
    results: rawResults,
    total,
    error,
    url,
  } = useSearchResults<FilterResult>(el, progress, () => {
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

  // Deduplicate by prop/type/unit: prefer active entries (with filterId) over inactive ones.
  // TODO: Support multiple filters for the same prop/type/unit.
  const results = computed(() => {
    const best = new Map<string, FilterResult>()
    for (const r of rawResults.value) {
      const key = filterResultKey(r)
      const existing = best.get(key)
      if (!existing || (r.filterId && !existing.filterId)) {
        best.set(key, r)
      }
    }
    return [...best.values()]
  })

  return { results, total, error, url }
}

function useSearchResults<T extends Result | FilterResult | RefFilterResult | HasFilterResult>(
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
  const results = process.env.NODE_ENV !== "production" ? readonly(_results) : (_results as unknown as Readonly<Ref<readonly DeepReadonly<T>[]>>)
  const total = process.env.NODE_ENV !== "production" ? readonly(_total) : _total
  const moreThanTotal = process.env.NODE_ENV !== "production" ? readonly(_moreThanTotal) : _moreThanTotal
  const error = process.env.NODE_ENV !== "production" ? readonly(_error) : _error
  const url = process.env.NODE_ENV !== "production" ? readonly(_url) : _url

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
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
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

export function useRefFilters(
  searchSessionRef: Ref<SearchSessionRef>,
  filterId: Ref<string>,
  props: Ref<readonly string[]>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<RefFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<RefFilterResult>(el, progress, () => {
    // TODO: Implement proper versioning.
    //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
    //       but the backend does not really use the parameter and always returns the latest version.
    const query = encodeQuery({ version: `${searchSessionRef.value.version}` })
    const id = filterId.value
    if (id) {
      // Active filter: use filter ID route.
      return router.apiResolve({
        name: "SearchFilterGet",
        params: { id: searchSessionRef.value.id, filter: id },
        query,
      }).href
    }
    // Inactive filter: use prop-based route.
    if (props.value.length === 2) {
      // Sub-ref filter: use parentProp + prop route.
      return router.apiResolve({
        name: "SearchSubRefFilter",
        params: { id: searchSessionRef.value.id, parentProp: props.value[0], prop: props.value[1] },
        query,
      }).href
    }
    return router.apiResolve({
      name: "SearchRefFilter",
      params: { id: searchSessionRef.value.id, prop: props.value[0] },
      query,
    }).href
  })
}

export function useHasFilters(
  searchSessionRef: Ref<SearchSessionRef>,
  filterId: Ref<string>,
  props: Ref<readonly string[]>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<HasFilterResult[]>>
  total: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()

  return useSearchResults<HasFilterResult>(el, progress, () => {
    // TODO: Implement proper versioning.
    //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
    //       but the backend does not really use the parameter and always returns the latest version.
    const query = encodeQuery({ version: `${searchSessionRef.value.version}` })
    const id = filterId.value
    if (id) {
      // Active filter: use filter ID route.
      return router.apiResolve({
        name: "SearchFilterGet",
        params: { id: searchSessionRef.value.id, filter: id },
        query,
      }).href
    }
    if (props.value.length === 1) {
      // Sub-has filter: use parentProp route.
      return router.apiResolve({
        name: "SearchSubHasFilter",
        params: { id: searchSessionRef.value.id, parentProp: props.value[0] },
        query,
      }).href
    }
    // Inactive top-level filter: use has-based route.
    return router.apiResolve({
      name: "SearchHasFilter",
      params: { id: searchSessionRef.value.id },
      query,
    }).href
  })
}

export function useAmountHistogramValues(
  searchSessionRef: Ref<SearchSessionRef>,
  filterId: Ref<string>,
  props: Ref<readonly string[]>,
  unit: Ref<string | undefined>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<HistogramAmountResult[]>>
  total: DeepReadonly<Ref<number | null>>
  missing: DeepReadonly<Ref<number | null>>
  from: DeepReadonly<Ref<number | null>>
  to: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<HistogramAmountResult[]>([])
  const _total = ref<number | null>(null)
  const _missing = ref<number | null>(null)
  const _from = ref<number | null>(null)
  const _to = ref<number | null>(null)
  const _interval = ref<number | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = process.env.NODE_ENV !== "production" ? readonly(_results) : _results
  const total = process.env.NODE_ENV !== "production" ? readonly(_total) : _total
  const missing = process.env.NODE_ENV !== "production" ? readonly(_missing) : _missing
  const from = process.env.NODE_ENV !== "production" ? readonly(_from) : _from
  const to = process.env.NODE_ENV !== "production" ? readonly(_to) : _to
  const interval = process.env.NODE_ENV !== "production" ? readonly(_interval) : _interval
  const error = process.env.NODE_ENV !== "production" ? readonly(_error) : _error
  const url = process.env.NODE_ENV !== "production" ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    () => {
      // TODO: Implement proper versioning.
      //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
      //       but the backend does not really use the parameter and always returns the latest version.
      const query = encodeQuery({ version: `${searchSessionRef.value.version}` })
      const id = filterId.value
      if (id) {
        // Active filter: use filter ID route.
        return router.apiResolve({
          name: "SearchFilterGet",
          params: { id: searchSessionRef.value.id, filter: id },
          query,
        }).href
      }
      // Inactive filter: use prop-based route. Sub-amount uses parentProp + prop.
      const isSub = props.value.length === 2
      const routeParams: Record<string, string> = { id: searchSessionRef.value.id }
      let routeName: string
      if (isSub) {
        routeParams.parentProp = props.value[0]
        routeParams.prop = props.value[1]
        routeName = "SearchSubAmountFilter"
        if (unit.value) {
          routeParams.unit = unit.value
          routeName = "SearchSubAmountFilterWithUnit"
        }
      } else {
        routeParams.prop = props.value[0]
        routeName = "SearchAmountFilter"
        if (unit.value) {
          routeParams.unit = unit.value
          routeName = "SearchAmountFilterWithUnit"
        }
      }
      return router.apiResolve({
        name: routeName,
        params: routeParams,
        query,
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
        _missing.value = null
        _from.value = null
        _to.value = null
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
        _missing.value = null
        _from.value = null
        _to.value = null
        _interval.value = null
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results
      _total.value = data.total
      _missing.value = data.missing != null ? data.missing : null
      _from.value = data.from != null ? parseFloat(data.from) : null
      _to.value = data.to != null ? parseFloat(data.to) : null
      _interval.value = data.interval != null ? parseFloat(data.interval) : null
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    missing,
    from,
    to,
    interval,
    error,
    url,
  }
}

export function useTimeHistogramValues(
  searchSessionRef: Ref<SearchSessionRef>,
  filterId: Ref<string>,
  props: Ref<readonly string[]>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  results: DeepReadonly<Ref<HistogramTimeResult[]>>
  total: DeepReadonly<Ref<number | null>>
  missing: DeepReadonly<Ref<number | null>>
  from: DeepReadonly<Ref<number | null>>
  to: DeepReadonly<Ref<number | null>>
  interval: DeepReadonly<Ref<number | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<HistogramTimeResult[]>([])
  const _total = ref<number | null>(null)
  const _missing = ref<number | null>(null)
  const _from = ref<number | null>(null)
  const _to = ref<number | null>(null)
  const _interval = ref<number | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const results = process.env.NODE_ENV !== "production" ? readonly(_results) : _results
  const total = process.env.NODE_ENV !== "production" ? readonly(_total) : _total
  const missing = process.env.NODE_ENV !== "production" ? readonly(_missing) : _missing
  const from = process.env.NODE_ENV !== "production" ? readonly(_from) : _from
  const to = process.env.NODE_ENV !== "production" ? readonly(_to) : _to
  const interval = process.env.NODE_ENV !== "production" ? readonly(_interval) : _interval
  const error = process.env.NODE_ENV !== "production" ? readonly(_error) : _error
  const url = process.env.NODE_ENV !== "production" ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    () => {
      // TODO: Implement proper versioning.
      //       Currently we pass version as a query parameter for reactivity to detect change and for busting the cache,
      //       but the backend does not really use the parameter and always returns the latest version.
      const query = encodeQuery({ version: `${searchSessionRef.value.version}` })
      const id = filterId.value
      if (id) {
        // Active filter: use filter ID route.
        return router.apiResolve({
          name: "SearchFilterGet",
          params: { id: searchSessionRef.value.id, filter: id },
          query,
        }).href
      }
      // Inactive filter: use prop-based route. Sub-time uses parentProp + prop.
      if (props.value.length === 2) {
        return router.apiResolve({
          name: "SearchSubTimeFilter",
          params: { id: searchSessionRef.value.id, parentProp: props.value[0], prop: props.value[1] },
          query,
        }).href
      }
      return router.apiResolve({
        name: "SearchTimeFilter",
        params: { id: searchSessionRef.value.id, prop: props.value[0] },
        query,
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
        _missing.value = null
        _from.value = null
        _to.value = null
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
        _missing.value = null
        _from.value = null
        _to.value = null
        _interval.value = null
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _results.value = data.results
      _total.value = data.total
      _missing.value = data.missing != null ? data.missing : null
      _from.value = data.from != null ? parseFloat(data.from) : null
      _to.value = data.to != null ? parseFloat(data.to) : null
      _interval.value = data.interval != null ? parseFloat(data.interval) : null
    },
    {
      immediate: true,
    },
  )

  return {
    results,
    total,
    missing,
    from,
    to,
    interval,
    error,
    url,
  }
}

async function getSearchResults<T extends Result | FilterResult | RefFilterResult | HasFilterResult>(
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
  missing?: number
  from?: string
  to?: string
  interval?: string
}> {
  const { doc, metadata } = await getURL(url, el, abortSignal, progress)
  if (abortSignal.aborted) {
    return { results: [], total: 0 }
  }

  if (!("total" in metadata)) {
    throw new Error(`"total" metadata is missing`)
  }
  const total = parseInt(String(metadata["total"]), 10)
  const res = { results: doc, total: total } as {
    results: T[]
    total: number
    missing?: number
    from?: string
    to?: string
    interval?: string
  }
  if ("missing" in metadata) {
    res.missing = metadata["missing"] as number
  }
  if ("from" in metadata) {
    res.from = String(metadata["from"])
  }
  if ("to" in metadata) {
    res.to = String(metadata["to"])
  }
  if ("interval" in metadata) {
    res.interval = String(metadata["interval"])
  }

  return res
}

export function useSearchSession(
  searchSessionRef: Ref<SearchSessionRef | null>,
  progress: Ref<number>,
): {
  searchSession: DeepReadonly<Ref<SearchSession | null>>
  error: DeepReadonly<Ref<string | null>>
  url: DeepReadonly<Ref<string | null>>
} {
  const router = useRouter()
  const route = useRoute()

  const _searchSession = ref<SearchSession | null>(null)
  const _error = ref<string | null>(null)
  const _url = ref<string | null>(null)
  const searchSession = process.env.NODE_ENV !== "production" ? readonly(_searchSession) : _searchSession
  const error = process.env.NODE_ENV !== "production" ? readonly(_error) : _error
  const url = process.env.NODE_ENV !== "production" ? readonly(_url) : _url

  const mainController = new AbortController()
  onBeforeUnmount(() => mainController.abort())

  const initialRouteName = route.name
  watch(
    searchSessionRef,
    // TODO: Use the pattern where we construct the URL here and then use it in the watcher once proper versioning is implemented.
    //       For now we use whole searchSessionRef and getURLDirect so that we always load the latest version in DocumentGet which uses fake version.
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
        // TODO: Use the pattern where we construct the URL here and then use it in the watcher once proper versioning is implemented.
        //       For now we use whole searchSessionRef and getURLDirect so that we always load the latest version in DocumentGet which uses fake version.
        data = await getURLDirect<SearchSession>(newURL, signal, progress)
      } catch (err) {
        if (signal.aborted) {
          return
        }
        console.error("useSearchSession", newURL, err)
        _searchSession.value = null
        // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
        _error.value = `${err}`
        return
      }
      if (signal.aborted) {
        return
      }
      _searchSession.value = data.doc
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

export function useLocationAt(searchResults: Ref<DeepReadonly<Result[]>>, searchTotal: Ref<number | null>, visibles: DeepReadonly<Ref<Set<string>>>) {
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
      const sorted = Array.from(visibles.value)
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
