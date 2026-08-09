import type { Ref } from "vue"
import type { Router } from "vue-router"

import type { StorageBeginUploadRequest, StorageBeginUploadResponse, StorageEndUploadRequest, StorageUploadStatus } from "@/types"

import { createSHA256 } from "hash-wasm"

import { getURLDirect, postBlob, postJSON } from "@/api"
import { delay, encodeQuery } from "@/utils"

// 10 MB.
const MAX_PAYLOAD_SIZE = 10 << 20

// Poll interval.
const POLL_INTERVAL = 1000 // In milliseconds.

// Waiting time to switch to the indeterminate progress bar after the upload finishes.
const SWITCH_TIME_MS = 400 // 300ms (progress bar transition duration) + 100ms extra.

// uploadFile uploads the file in chunks and returns the id of the stored file. It returns the empty string when abortSignal is aborted, in which case a
// best-effort discard request is made for the upload session once its identifier is known. It throws when the file is empty or the upload fails.
//
// Handling of progress here is slightly different from the rest of the codebase
// because we support both a determinate and indeterminate progress bar.
export async function uploadFile(
  router: Router,
  file: File,
  abortSignal: AbortSignal,
  progress: Ref<number> | null,
  total: Ref<number | undefined> | null,
): Promise<string> {
  if (file.size === 0) {
    throw new Error("file is empty")
  }

  const initialProgress = progress?.value ?? 0
  let succeeded = false
  let abortListener: (() => void) | null = null
  try {
    // Initially, we show the indeterminate progress bar.
    if (progress) {
      progress.value = 1
    }
    if (total) {
      total.value = undefined
    }

    // TODO: Pass and store lastModified timestamp for the file (as different timestamp than current uploaded "at" timestamp).
    const beginUploadRequest: StorageBeginUploadRequest = {
      size: file.size,
      mediaType: file.type || "application/octet-stream",
      filename: file.name || "",
    }
    // Beginning the upload is the one request here which is not given the abort signal. The response carries
    // the identifier of the session the backend has just created, and that identifier is the only way to
    // release it again, so abandoning the request would leave the session behind with nothing able to
    // discard it. An abort which arrives while this is in flight is acted on below instead, once the
    // identifier is in hand.
    const beginUploadResponse = await postJSON<StorageBeginUploadResponse>(
      router.apiResolve({
        name: "StorageBeginUpload",
      }).href,
      beginUploadRequest,
      new AbortController().signal,
      progress,
    )
    // The signal which was passed never aborts, so a null response means the request itself failed.
    if (beginUploadResponse === null) {
      return ""
    }

    // If abortSignal is aborted after the session is created but before the upload
    // completes, fire a best-effort discard request so the backend can release the session.
    // We use fetch's keepalive so the request survives even if the page is being unloaded.
    const discardURL = router.apiResolve({
      name: "StorageDiscardUpload",
      params: {
        session: beginUploadResponse.session,
      },
    }).href
    abortListener = () => {
      if (succeeded) {
        return
      }
      fetch(discardURL, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: "{}",
        mode: "same-origin",
        credentials: "same-origin",
        redirect: "error",
        referrer: document.location.href,
        referrerPolicy: "strict-origin-when-cross-origin",
        keepalive: true,
      }).catch(() => {
        // Best-effort; ignore failures.
      })
    }
    abortSignal.addEventListener("abort", abortListener, { once: true })
    // If abort fired before we attached the listener (e.g., during the begin-upload
    // request), the listener will not run; trigger the discard manually.
    if (abortSignal.aborted) {
      abortListener()
      return ""
    }

    // We compute the SHA-256 of the file incrementally as we read each chunk, so the whole file is
    // never held in memory, and send it when ending the upload. The backend recomputes the hash of the
    // assembled file and fails the upload on a mismatch, detecting corruption in transit.
    const hasher = await createSHA256()
    if (abortSignal.aborted) {
      return ""
    }

    for (let chunkStart = 0; chunkStart < file.size; chunkStart += MAX_PAYLOAD_SIZE) {
      const chunkEnd = Math.min(chunkStart + MAX_PAYLOAD_SIZE, file.size)
      // chunk is a lazy, file-backed Blob. We read its bytes once to feed the hasher (the buffer is
      // transient and freed after), and hand the same Blob to postBlob, which streams it from the file
      // without us holding it in memory.
      const chunk = file.slice(chunkStart, chunkEnd)
      const buffer = await chunk.arrayBuffer()
      if (abortSignal.aborted) {
        return ""
      }
      hasher.update(new Uint8Array(buffer))
      // TODO: We should switch implementation of postBlob to XMLHttpRequest and obtain progress inside a chunk.
      await postBlob(
        router.apiResolve({
          name: "StorageUploadChunk",
          params: {
            session: beginUploadResponse.session,
          },
          // Because start is less than MAX_PAYLOAD_SIZE, toString() never uses scientific notation.
          query: encodeQuery({ start: chunkStart.toString() }),
        }).href,
        chunk,
        abortSignal,
        progress,
      )
      if (abortSignal.aborted) {
        return ""
      }
      // We set progress for the first time after the end of the chunk.
      // This makes a progress bar an indeterminate one during the first chunk and a determinate
      // one after the it. Otherwise the progress bar would be a determinate one with length 0
      // and no indication of progress/activity during the first chunk.
      if (progress) {
        progress.value = chunkEnd
      }
      if (total) {
        total.value = file.size
      }
    }

    // We wait after we have uploaded the last chunk before we switch to
    // the indeterminate progress bar. This gives the user a chance to
    // observe the final 100% state for at least SWITCH_TIME_MS.
    const switchTimeout = setTimeout(() => {
      if (total) {
        total.value = undefined
      }
    }, SWITCH_TIME_MS)

    const endUploadRequest: StorageEndUploadRequest = {
      hash: hasher.digest("hex"),
    }

    try {
      await postJSON(
        router.apiResolve({
          name: "StorageEndUpload",
          params: {
            session: beginUploadResponse.session,
          },
        }).href,
        endUploadRequest,
        abortSignal,
        // We do not mind progress being additionally increased beyond total because
        // progress bar clamps it at 100%.
        progress,
      )
      if (abortSignal.aborted) {
        return ""
      }

      // Poll for completion.
      const uploadStatusURL = router.apiResolve({
        name: "StorageUpload",
        params: {
          session: beginUploadResponse.session,
        },
      }).href

      while (true) {
        await delay(POLL_INTERVAL, abortSignal)
        if (abortSignal.aborted) {
          return ""
        }
        // We do not mind progress being additionally increased beyond total because
        // progress bar clamps it at 100%.
        const res = await getURLDirect<StorageUploadStatus>(uploadStatusURL, abortSignal, progress)
        if (abortSignal.aborted || res === null) {
          return ""
        }
        const status = res.doc
        if (status.id) {
          succeeded = true
          return status.id
        }
        if (status.errored) {
          throw new Error("upload failed")
        }
        if (status.discarded) {
          throw new Error("upload session was discarded")
        }
      }
    } finally {
      clearTimeout(switchTimeout)
    }
  } catch (err) {
    if (abortSignal.aborted) {
      return ""
    }
    throw err
  } finally {
    if (abortListener) {
      abortSignal.removeEventListener("abort", abortListener)
    }
    if (progress) {
      progress.value = initialProgress
    }
    if (total) {
      total.value = undefined
    }
  }
}
