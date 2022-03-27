import { Ref, DeepReadonly, onBeforeUnmount } from "vue"
import type { Router } from "vue-router"
import type { SearchResult, PeerDBDocument } from "@/types"

import { ref, watch, readonly } from "vue"
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

function updateHasMore(hasMore: Ref<"yes" | "limit" | "no">, limit: number, results: number, total: number, moreThanTotal: boolean) {
  if (limit < results) {
    hasMore.value = "yes"
  } else if (total > results) {
    hasMore.value = "limit"
  } else if (moreThanTotal) {
    hasMore.value = "limit"
  } else {
    hasMore.value = "no"
  }
}

function updateDocs(router: Router, docs: Ref<PeerDBDocument[]>, limit: number, results: SearchResult[], progress: Ref<number>, abortSignal: AbortSignal) {
  assert(limit <= results.length, `${limit} <= ${results.length}`)
  for (let i = docs.value.length; i < limit; i++) {
    docs.value.push(results[i])
    getDocument(router, results[i]._id, progress, abortSignal).then((data) => {
      docs.value[i] = data
    })
  }
}

export function useSearch(progress: Ref<number>): {
  docs: DeepReadonly<Ref<PeerDBDocument[]>>
  total: DeepReadonly<Ref<number>>
  moreThanTotal: DeepReadonly<Ref<boolean>>
  hasMore: DeepReadonly<Ref<"yes" | "limit" | "no">>
  loadMore: () => void
} {
  const router = useRouter()
  const route = useRoute()

  let results = <SearchResult[]>[]
  let limit = 0

  const _docs = ref<PeerDBDocument[]>([])
  const _total = ref(0)
  const _moreThanTotal = ref(false)
  const _hasMore = ref<"yes" | "limit" | "no">("no")
  const docs = import.meta.env.DEV ? readonly(_docs) : _docs
  const total = import.meta.env.DEV ? readonly(_total) : _total
  const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal
  const hasMore = import.meta.env.DEV ? readonly(_hasMore) : _hasMore

  const initialRouteName = route.name
  watch(
    () => {
      const params = new URLSearchParams()
      if (Array.isArray(route.query.q)) {
        if (route.query.q[0] != null) {
          params.set("q", route.query.q[0])
        }
      } else {
        if (route.query.q != null) {
          params.set("q", route.query.q)
        }
      }
      if (Array.isArray(route.query.s)) {
        if (route.query.s[0] != null) {
          params.set("s", route.query.s[0])
        }
      } else {
        if (route.query.s != null) {
          params.set("s", route.query.s)
        }
      }
      return params.toString()
    },
    async (query, oldQuery, onCleanup) => {
      // Watch can continue to run for some time after the route changes.
      if (initialRouteName !== route.name) {
        return
      }
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      const data = await getSearch(router, query, progress, controller.signal)
      if (data === null) {
        return
      }
      results = data.results
      if (data.total.endsWith("+")) {
        _moreThanTotal.value = true
        _total.value = parseInt(data.total.substring(0, data.total.length - 2))
      } else {
        _moreThanTotal.value = false
        _total.value = parseInt(data.total)
      }
      _docs.value = []
      limit = Math.min(INITIAL_LIMIT, results.length)
      updateHasMore(_hasMore, limit, results.length, total.value, moreThanTotal.value)
      updateDocs(router, _docs, limit, results, progress, controller.signal)
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
    moreThanTotal,
    hasMore,
    loadMore: () => {
      limit = Math.min(limit + INCREASE, results.length)
      updateHasMore(_hasMore, limit, results.length, total.value, moreThanTotal.value)
      updateDocs(router, _docs, limit, results, progress, controller.signal)
    },
  }
}

async function getSearch(router: Router, query: string, progress: Ref<number>, abortSignal: AbortSignal): Promise<{ results: SearchResult[]; total: string } | null> {
  progress.value += 1
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentSearch",
      }).href +
        "?" +
        query,
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
    const data = await response.json()
    if (Array.isArray(data)) {
      const total = response.headers.get("Peerdb-Total")
      if (total === null) {
        throw new Error("Peerdb-Total header is null")
      }
      return { results: data, total }
    } else {
      await router.replace({
        name: "DocumentSearch",
        query: data,
      })
      return null
    }
  } finally {
    progress.value -= 1
  }
}

export async function getDocument(router: Router, id: string, progress: Ref<number>, abortSignal: AbortSignal): Promise<PeerDBDocument> {
  progress.value += 1
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentGet",
        params: {
          id,
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
    // TODO: JSON response should include _id field, but until then we add it here.
    doc._id = id
    return doc
  } finally {
    progress.value -= 1
  }
}
