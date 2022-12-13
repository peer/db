import type { Ref } from "vue"
import type { PeerDBDocument, Router } from "@/types"

import { ref, readonly } from "vue"
import { Queue } from "@/queue"

const queue = new Queue({ concurrency: 100 })

const localGetCache = new Map<string, WeakRef<{ doc: object; headers: Headers }>>()

const _globalProgress = ref(0)
export const globalProgress = import.meta.env.DEV ? readonly(_globalProgress) : _globalProgress

// TODO: Improve priority with "el".
export async function getURL(url: string, el: Ref<Element | null>, abortSignal: AbortSignal, progress?: Ref<number>): Promise<{ doc: object; headers: Headers }> {
  // Is it already cached?
  const weakRef = localGetCache.get(url)
  if (weakRef) {
    const cached = weakRef.deref()
    if (cached) {
      return cached
    } else {
      // Weak reference's target has been reclaimed.
      localGetCache.delete(url)
    }
  }

  if (progress) {
    progress.value += 1
  }
  _globalProgress.value += 1
  try {
    const res = await queue.add(
      async () => {
        // We check again.
        const weakRef = localGetCache.get(url)
        if (weakRef) {
          const cached = weakRef.deref()
          if (cached) {
            return cached
          } else {
            // Weak reference's target has been reclaimed.
            localGetCache.delete(url)
          }
        }

        const response = await fetch(url, {
          method: "GET",
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
        return { doc: await response.json(), headers: response.headers }
      },
      {
        signal: abortSignal,
      },
    )
    localGetCache.set(url, new WeakRef(res))
    return res
  } finally {
    _globalProgress.value -= 1
    if (progress) {
      progress.value -= 1
    }
  }
}

export async function postURL(url: string, form: FormData, progress: Ref<number>): Promise<object> {
  progress.value += 1
  _globalProgress.value += 1
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
      },
      // Have to cast to "any". See: https://github.com/microsoft/TypeScript/issues/30584
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      body: new URLSearchParams(form as any),
      mode: "same-origin",
      credentials: "omit",
      redirect: "error",
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
    })
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    return await response.json()
  } finally {
    _globalProgress.value -= 1
    progress.value -= 1
  }
}

export async function getDocument(router: Router, id: string, el: Ref<Element | null>, abortSignal: AbortSignal, progress?: Ref<number>): Promise<PeerDBDocument> {
  const { doc } = await getURL(
    router.apiResolve({
      name: "DocumentGet",
      params: {
        id,
      },
    }).href,
    el,
    abortSignal,
    progress,
  )
  return doc as PeerDBDocument
}
