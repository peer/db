import type { Ref, DeepReadonly } from "vue"
import type { Router, LocationQueryRaw } from "vue-router"
import type { SearchResult, PeerDBDocument } from "@/types"

import { ref, watch, readonly, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import { assert } from "@vue/compiler-core"

const INITIAL_LIMIT = 50
const INCREASE = 50

export async function postSearch(router: Router, form: HTMLFormElement, progress: Ref<number>) {
  progress.value += 1
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentSearch",
      }).href,
      {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
        },
        // Have to cast to "any". See: https://github.com/microsoft/TypeScript/issues/30584
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        body: new URLSearchParams(new FormData(form) as any),
        mode: "same-origin",
        credentials: "omit",
        redirect: "error",
        referrer: document.location.href,
        referrerPolicy: "strict-origin-when-cross-origin",
      },
    )
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    await router.push({
      name: "DocumentSearch",
      query: await response.json(),
    })
  } finally {
    progress.value -= 1
  }
}

function updateDocs(router: Router, docs: Ref<PeerDBDocument[]>, limit: number, results: readonly SearchResult[], progress: Ref<number>, abortSignal: AbortSignal) {
  assert(limit <= results.length, `${limit} <= ${results.length}`)
  for (let i = docs.value.length; i < limit; i++) {
    docs.value.push(results[i])
    getDocument(router, results[i], progress, abortSignal).then((data) => {
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
    null,
  )
}

function useResults(
  progress: Ref<number>,
  getURL: () => string | null,
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
      const data = await getResults(url, progress, controller.signal)
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
      limit = Math.min(INITIAL_LIMIT, results.value.length)
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, progress, controller.signal)
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
      limit = Math.min(limit + INCREASE, results.value.length)
      _hasMore.value = limit < results.value.length
      updateDocs(router, _docs, limit, results.value, progress, controller.signal)
    },
  }
}

async function getResults(
  url: string,
  progress: Ref<number>,
  abortSignal: AbortSignal,
): Promise<{ results: SearchResult[]; total: string; query?: string } | { q: string; s: string }> {
  progress.value += 1
  try {
    const response = await fetch(url, {
      method: "GET",
      headers: {
        Accept: "application/json",
      },
      mode: "same-origin",
      credentials: "omit",
      redirect: "error",
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
      signal: abortSignal,
    })
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    const data = await response.json()
    if (Array.isArray(data)) {
      const total = response.headers.get("Peerdb-Total")
      if (total === null) {
        throw new Error("Peerdb-Total header is null")
      }
      const res = { results: data, total } as { results: SearchResult[]; total: string; query?: string }
      const query = response.headers.get("Peerdb-Query")
      if (query !== null) {
        res.query = query
      }
      return res
    }
    return data
  } finally {
    progress.value -= 1
  }
}

export async function getDocument(router: Router, result: SearchResult, progress: Ref<number>, abortSignal: AbortSignal): Promise<PeerDBDocument> {
  progress.value += 1
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentGet",
        params: {
          id: result._id,
        },
      }).href,
      {
        method: "GET",
        headers: {
          Accept: "application/json",
        },
        mode: "same-origin",
        credentials: "omit",
        redirect: "error",
        referrer: document.location.href,
        referrerPolicy: "strict-origin-when-cross-origin",
        signal: abortSignal,
      },
    )
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    const doc = await response.json()
    // We add any extra fields from the result (e.g., _count).
    // This also adds _id if it is not already present.
    Object.assign(doc, result)
    return doc
  } finally {
    progress.value -= 1
  }
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
      const data = await getResults(getSearchURL(router, params.toString()), progress, controller.signal)
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
