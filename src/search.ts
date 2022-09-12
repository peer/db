import type { Ref, DeepReadonly } from "vue"
import type { Router, LocationQueryRaw } from "vue-router"
import type { SearchResult, PeerDBDocument } from "@/types"

import { ref, watch, readonly, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import { assert } from "@vue/compiler-core"
import { getURL, postURL } from "@/api"

const SEARCH_INITIAL_LIMIT = 50
const SEARCH_INCREASE = 50
const FILTERS_INITIAL_LIMIT = 10
const FILTERS_INCREASE = 10

export async function postSearch(router: Router, form: HTMLFormElement, progress: Ref<number>) {
  const query = await postURL(
    router.resolve({
      name: "DocumentSearch",
    }).href,
    form,
    progress,
  )
  await router.push({
    name: "DocumentSearch",
    query: query as LocationQueryRaw,
  })
}

function updateDocs(
  router: Router,
  docs: Ref<PeerDBDocument[]>,
  limit: number,
  results: readonly SearchResult[],
  priority: number,
  progress: Ref<number>,
  abortSignal: AbortSignal,
) {
  assert(limit <= results.length, `${limit} <= ${results.length}`)
  for (let i = docs.value.length; i < limit; i++) {
    docs.value.push(results[i])
    getDocument(router, results[i], priority, progress, abortSignal).then((data) => {
      docs.value[i] = data
    })
  }
}

function getSearchURL(router: Router, params: string): string {
  return router.resolve({ name: "DocumentSearch" }).href + "?" + params
}

export function useSearch(
  progress: Ref<number>,
  redirect: (query: LocationQueryRaw) => Promise<void | undefined>,
): {
  docs: DeepReadonly<Ref<PeerDBDocument[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  return useResults(
    0,
    progress,
    () => {
      const params = new URLSearchParams()
      if (Array.isArray(route.query.s)) {
        if (route.query.s[0] != null) {
          params.set("s", route.query.s[0])
        }
      } else if (route.query.s != null) {
        params.set("s", route.query.s)
      }
      if (Array.isArray(route.query.q)) {
        if (route.query.q[0] != null) {
          params.set("q", route.query.q[0])
        }
      } else if (route.query.q != null) {
        params.set("q", route.query.q)
      }
      return getSearchURL(router, params.toString())
    },
    SEARCH_INITIAL_LIMIT,
    SEARCH_INCREASE,
    redirect,
  )
}

export function useFilters(progress: Ref<number>): {
  docs: DeepReadonly<Ref<PeerDBDocument[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  return useResults(
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
      return router.resolve({
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

function useResults(
  priority: number,
  progress: Ref<number>,
  getURL: () => string | null,
  initialLimit: number,
  increase: number,
  redirect?: ((query: LocationQueryRaw) => Promise<void | undefined>) | null,
): {
  docs: DeepReadonly<Ref<PeerDBDocument[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  let limit = 0

  const _docs = ref<PeerDBDocument[]>([])
  const _results = ref<SearchResult[]>([])
  // We start with -1, so that until data is loaded the
  // first time, we do not flash "no results found".
  const _total = ref(-1)
  const _moreThanTotal = ref(false)
  const _hasMore = ref(false)
  const docs = import.meta.env.DEV ? readonly(_docs) : _docs
  const results = import.meta.env.DEV ? readonly(_results) : _results
  const total = import.meta.env.DEV ? readonly(_total) : _total
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
        _total.value = -1
        _moreThanTotal.value = false
        _hasMore.value = false
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getResults(url, priority, progress, controller.signal)
      if (!("results" in data)) {
        _docs.value = []
        _results.value = []
        _total.value = -1
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
      limit = Math.min(initialLimit, results.value.length)
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, priority, progress, controller.signal)
    },
    {
      immediate: true,
    },
  )

  const controller = new AbortController()
  onBeforeUnmount(() => controller.abort())

  return {
    docs,
    total,
    results,
    moreThanTotal,
    hasMore,
    loadMore: () => {
      limit = Math.min(limit + increase, results.value.length)
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, priority, progress, controller.signal)
    },
  }
}

async function getResults(
  url: string,
  priority: number,
  progress: Ref<number>,
  abortSignal: AbortSignal,
): Promise<{ results: SearchResult[]; total: string; query?: string } | { q: string; s: string }> {
  const { doc, headers } = await getURL(url, priority, progress, abortSignal)

  if (Array.isArray(doc)) {
    const total = headers.get("Peerdb-Total")
    if (total === null) {
      throw new Error("Peerdb-Total header is null")
    }
    const res = { results: doc, total } as { results: SearchResult[]; total: string; query?: string }
    const query = headers.get("Peerdb-Query")
    if (query !== null) {
      res.query = query
    }
    return res
  }

  return doc as { q: string; s: string }
}

export async function getDocument(router: Router, result: SearchResult, priority: number, progress: Ref<number>, abortSignal: AbortSignal): Promise<PeerDBDocument> {
  const { doc } = await getURL(
    router.resolve({
      name: "DocumentGet",
      params: {
        id: result._id,
      },
    }).href,
    priority,
    progress,
    abortSignal,
  )
  // We add any extra fields from the result (e.g., _count).
  // This also adds _id if it is not already present.
  return Object.assign({}, doc, result)
}

export function useFilterValues(
  property: PeerDBDocument,
  progress: Ref<number>,
): {
  docs: DeepReadonly<Ref<PeerDBDocument[]>>
  results: DeepReadonly<Ref<SearchResult[]>>
  total: DeepReadonly<Ref<number>>
  hasMore: DeepReadonly<Ref<boolean>>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  return useResults(
    -2,
    progress,
    () => {
      let s
      if (Array.isArray(route.query.s)) {
        s = route.query.s[0]
      } else {
        s = route.query.s
      }
      if (!s || !property._id) {
        return null
      }
      return router.resolve({
        name: "DocumentSearchFilterGet",
        params: {
          s,
          prop: property._id,
        },
      }).href
    },
    FILTERS_INITIAL_LIMIT,
    FILTERS_INCREASE,
    null,
  )
}

export function useSearchState(
  progress: Ref<number>,
  redirect: (query: LocationQueryRaw) => Promise<void | undefined>,
): {
  results: DeepReadonly<Ref<SearchResult[]>>
  query: DeepReadonly<Ref<{ s?: string; at?: string; q?: string }>>
} {
  const router = useRouter()
  const route = useRoute()

  const _results = ref<SearchResult[]>([])
  const _query = ref<{ s?: string; at?: string; q?: string }>({})
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
      const data = await getResults(getSearchURL(router, params.toString()), 0, progress, controller.signal)
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
        q: decodeURIComponent(data.query as string),
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
