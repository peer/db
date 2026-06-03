import type { Ref } from "vue"

import type { Metadata } from "@/types"

import siteContext from "@/context"
import { decodeMetadata } from "@/metadata"
import { Queue } from "@/queue"

const queue = new Queue({ concurrency: 100 })

// TODO: Use WeakRef with already reactive and new D() documents.
const localGetCache = new Map<string, { doc: unknown; metadata: Metadata }>()

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

// clearCache empties the in-process GET cache. It is called on sign-out so that
// responses cached for the previous identity (which may include role-restricted
// fields or results) are not reused under the new roles.
export function clearCache() {
  localGetCache.clear()
}

// TODO: Improve priority with "el".
export async function getURL<T>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number> | null,
): Promise<{ doc: T; metadata: Metadata }> {
  // Is it already cached?
  const cached = localGetCache.get(url)
  if (cached) {
    return cached as { doc: T; metadata: Metadata }
  }

  if (progress) {
    progress.value += 1
  }
  try {
    const res = await queue.add(
      async () => {
        // We check again.
        const cached = localGetCache.get(url)
        if (cached) {
          return cached as { doc: T; metadata: Metadata }
        }

        return await getURLDirect<T>(url, abortSignal, progress)
      },
      {
        signal: abortSignal,
      },
    )
    localGetCache.set(url, res)
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
    return { doc: (await response.json()) as T, metadata: decodeMetadata(response.headers, siteContext.metadataHeaderPrefix ?? "") }
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

export async function headURLDirect(url: string, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<Headers> {
  if (progress) {
    progress.value += 1
  }
  try {
    const response = await fetch(url, {
      method: "HEAD",
      mode: "cors",
      credentials: "same-origin",
      referrer: document.location.href,
      referrerPolicy: "strict-origin-when-cross-origin",
      signal: abortSignal,
    })
    if (!response.ok) {
      throw new FetchError(`fetch HEAD error ${response.status}`, {
        status: response.status,
        body: "",
        url,
        requestID: response.headers.get("Request-ID"),
      })
    }
    return response.headers
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
      headers: { "Content-Type": "application/json" },
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

export async function postBlob<T>(url: string, data: Blob, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<T> {
  if (progress) {
    progress.value += 1
  }
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
    if (progress) {
      progress.value -= 1
    }
  }
}
