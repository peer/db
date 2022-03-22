import type { Ref } from "vue"
import type { Router } from "vue-router"
import type { SearchResult } from "@/types"

export async function makeSearch(router: Router, progress: Ref<number>, form: HTMLFormElement) {
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
        // eslint-disable-next-line  @typescript-eslint/no-explicit-any
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

export async function doSearch(
  router: Router,
  progress: Ref<number>,
  query: string,
  abortSignal: AbortSignal,
): Promise<{ results: SearchResult[]; total: string } | null> {
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
