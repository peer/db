import type { Ref } from "vue"

import { ref, readonly } from "vue"
import PQueue from "p-queue"

const queue = new PQueue({ concurrency: 100 })

const localGetCache = new Map<string, WeakRef<{ doc: object; headers: Headers }>>()

const _globalProgress = ref(0)
export const globalProgress = import.meta.env.DEV ? readonly(_globalProgress) : _globalProgress

export async function getURL(url: string, priority: number, abortSignal: AbortSignal, progress?: Ref<number>): Promise<{ doc: object; headers: Headers }> {
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
      async ({ signal }) => {
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
          signal,
        })
        if (!response.ok) {
          throw new Error(`fetch error ${response.status}: ${await response.text()}`)
        }
        return { doc: await response.json(), headers: response.headers }
      },
      {
        priority,
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

export async function postURL(url: string, form: HTMLFormElement, progress: Ref<number>): Promise<object> {
  progress.value += 1
  _globalProgress.value += 1
  try {
    const response = await fetch(url, {
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
