import type { Ref } from "vue"

import type { Metadata } from "@/types"

import { accessToken } from "@/auth"
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

// bearerHeader returns a fresh Headers object carrying the OIDC bearer token
// when the user is signed in, and an empty one otherwise. We build a new
// Headers per request because fetch consumes the object - sharing it across
// requests would couple their lifecycles.
function bearerHeader(): Headers {
  const headers = new Headers()
  if (accessToken.value) {
    headers.set("Authorization", `Bearer ${accessToken.value}`)
  }
  return headers
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
      headers: bearerHeader(),
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
      headers: bearerHeader(),
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
    const headers = bearerHeader()
    headers.set("Content-Type", "application/json")
    const response = await fetch(url, {
      method: "POST",
      headers,
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
      headers: bearerHeader(),
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
