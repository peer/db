import type { Ref } from "vue"
import type { Metadata } from "@/types"

import { ref, readonly } from "vue"
import { Queue } from "@/queue"
import { decodeMetadata } from "./metadata"

const queue = new Queue({ concurrency: 100 })

const localGetCache = new Map<string, WeakRef<{ doc: unknown; metadata: Metadata }>>()

const _globalProgress = ref(0)
export const globalProgress = import.meta.env.DEV ? readonly(_globalProgress) : _globalProgress

export class FetchError extends Error {
  cause?: Error
  status: number
  body: string
  url: string
  requestID: string | null

  constructor(msg: string, options: { cause?: Error; status: number; body: string; url: string; requestID: string | null }) {
    // Cause gets set by super.
    super(msg, options)
    this.status = options.status
    this.body = options.body
    this.url = options.url
    this.requestID = options.requestID
  }
}

// TODO: Improve priority with "el".
export async function getURL<T>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal | null,
  progress: Ref<number> | null,
): Promise<{ doc: T; metadata: Metadata }> {
  // Is it already cached?
  const weakRef = localGetCache.get(url)
  if (weakRef) {
    const cached = weakRef.deref()
    if (cached) {
      return cached as { doc: T; metadata: Metadata }
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
          referrer: document.location.href,
          referrerPolicy: "strict-origin-when-cross-origin",
          signal: abortSignal,
        })
        const contentType = response.headers.get("Content-Type")
        if (!contentType || !contentType.includes("application/json")) {
          const body = await response.text()
          throw new FetchError(`fetch GET error ${response.status}: ${body}`, {
            status: response.status,
            body,
            url,
            requestID: response.headers.get("Request-ID"),
          })
        }
        return { doc: await response.json(), metadata: decodeMetadata(response.headers) }
      },
      {
        signal: abortSignal || undefined,
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

export async function postURL<T>(url: string, form: FormData, progress: Ref<number> | null): Promise<T> {
  if (progress) {
    progress.value += 1
  }
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
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
    })
    const contentType = response.headers.get("Content-Type")
    if (!contentType || !contentType.includes("application/json")) {
      const body = await response.text()
      throw new FetchError(`fetch POST error ${response.status}: ${body}`, {
        status: response.status,
        body,
        url,
        requestID: response.headers.get("Request-ID"),
      })
    }
    return await response.json()
  } finally {
    _globalProgress.value -= 1
    if (progress) {
      progress.value -= 1
    }
  }
}
