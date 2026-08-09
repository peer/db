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

// getURL fetches the document at the URL, going through the request queue and the in-process GET cache, and returns it together with its decoded metadata.
// It returns null when abortSignal is aborted, in which case nothing is cached. It throws FetchError when the response is not JSON, and rejects with the
// abort reason when the abort lands while the request is queued or in flight.
//
// TODO: Improve priority with "el".
export async function getURL<T>(
  url: string,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number> | null,
): Promise<{ doc: T; metadata: Metadata } | null> {
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
    // A null result means getURLDirect stopped on abort, so there is nothing to cache.
    if (abortSignal.aborted || res === null) {
      return null
    }
    localGetCache.set(url, res)
    return res
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

// getURLDirect fetches the document at the URL, bypassing both the request queue and the in-process GET cache, and returns it together with its decoded
// metadata. It returns null when abortSignal is aborted. It throws FetchError when the response is not JSON, and rejects with the abort reason when the
// abort lands while the request is in flight.
export async function getURLDirect<T>(url: string, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<{ doc: T; metadata: Metadata } | null> {
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
    if (abortSignal.aborted) {
      return null
    }
    const contentType = response.headers.get("Content-Type")
    if (!contentType || !contentType.includes("application/json")) {
      const body = await response.text()
      if (abortSignal.aborted) {
        return null
      }
      throw new FetchError(`fetch GET error ${response.status}: ${body}`, {
        status: response.status,
        body,
        url,
        requestID: response.headers.get("Request-ID"),
      })
    }
    const doc = (await response.json()) as T
    if (abortSignal.aborted) {
      return null
    }
    return { doc, metadata: decodeMetadata(response.headers, siteContext.metadataHeaderPrefix ?? "") }
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

// headURLDirect makes a HEAD request for the URL and returns the response headers. It returns null when abortSignal is aborted. It throws FetchError when
// the response is not successful, and rejects with the abort reason when the abort lands while the request is in flight.
export async function headURLDirect(url: string, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<Headers | null> {
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
    if (abortSignal.aborted) {
      return null
    }
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

// postJSON posts data as JSON to the URL and returns the parsed JSON response. It returns null when abortSignal is aborted. It throws FetchError when the
// response is not JSON, and rejects with the abort reason when the abort lands while the request is in flight.
export async function postJSON<T>(url: string, data: object, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<T | null> {
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
    if (abortSignal.aborted) {
      return null
    }
    const contentType = response.headers.get("Content-Type")
    if (!contentType || !contentType.includes("application/json")) {
      const body = await response.text()
      if (abortSignal.aborted) {
        return null
      }
      throw new FetchError(`fetch POST error ${response.status}: ${body}`, {
        status: response.status,
        body,
        url,
        requestID: response.headers.get("Request-ID"),
      })
    }
    const doc = (await response.json()) as T
    if (abortSignal.aborted) {
      return null
    }
    return doc
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}

// postBlob posts a blob to the URL and returns the parsed JSON response. It returns null when abortSignal is aborted. It throws FetchError when the
// response is not JSON, and rejects with the abort reason when the abort lands while the request is in flight.
export async function postBlob<T>(url: string, data: Blob, abortSignal: AbortSignal, progress: Ref<number> | null): Promise<T | null> {
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
    if (abortSignal.aborted) {
      return null
    }
    const contentType = response.headers.get("Content-Type")
    if (!contentType || !contentType.includes("application/json")) {
      const body = await response.text()
      if (abortSignal.aborted) {
        return null
      }
      throw new FetchError(`fetch POST error ${response.status}: ${body}`, {
        status: response.status,
        body,
        url,
        requestID: response.headers.get("Request-ID"),
      })
    }
    const doc = (await response.json()) as T
    if (abortSignal.aborted) {
      return null
    }
    return doc
  } finally {
    if (progress) {
      progress.value -= 1
    }
  }
}
