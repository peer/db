import type { Ref } from "vue"

import type { Metadata } from "@/types"

import { decodeMetadata } from "@/metadata"
import { Queue } from "@/queue"

const queue = new Queue({ concurrency: 100 })

const localGetCache = new Map<string, WeakRef<{ doc: unknown; metadata: Metadata }>>()

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

export function deleteFromCache(url: string) {
  localGetCache.delete(url)
}

// TODO: Improve priority with "el".
export async function getURL<T>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
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
  try {
    const res = await queue.add(
      async () => {
        // We check again.
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

        return await getURLDirect<T>(url, abortSignal, progress)
      },
      {
        signal: abortSignal,
      },
    )
    localGetCache.set(url, new WeakRef(res))
    return res
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

export async function getURLDirect<T>(url: string, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<{ doc: T; metadata: Metadata }> {
  if (progress) {
    progress.value += 1
  }
  try {
    const response = await fetch(url, {
      method: "GET",
      // Mode and credentials match crossorigin=anonymous in link preload header.
      mode: "cors",
      credentials: "same-origin",
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
    return { doc: (await response.json()) as T, metadata: decodeMetadata(response.headers) }
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

export async function postJSON<T>(url: string, data: object, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<T> {
  if (progress) {
    progress.value += 1
  }
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(data),
      mode: "same-origin",
      credentials: "same-origin",
      redirect: "error",
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
      signal: abortSignal,
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
    return (await response.json()) as T
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

export async function postBlob<T>(url: string, data: Blob, abortSignal: AbortSignal, progress: Ref<number>): Promise<T> {
  progress.value += 1
  try {
    const response = await fetch(url, {
      method: "POST",
      body: data,
      mode: "same-origin",
      credentials: "same-origin",
      redirect: "error",
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
      signal: abortSignal,
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
    return (await response.json()) as T
  } finally {
    progress.value -= 1
  }
}
