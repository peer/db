import type { Ref } from "vue"
import type { Router } from "vue-router"

import type { StorageBeginUploadRequest, StorageBeginUploadResponse, StorageUploadStatus } from "@/types"

import { getURLDirect, postBlob, postJSON } from "@/api"
import { delay, encodeQuery } from "@/utils"

// 10 MB.
const MAX_PAYLOAD_SIZE = 10 << 20

// Poll interval in milliseconds.
const POLL_INTERVAL = 1000

// Waiting time to switch to the indeterminate progress bar after the upload finishes.
const SWITCH_TIME_MS = 400 // 300ms (progress bar transition duration) + 100ms extra.

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
  try {
    // Initially, we show the indeterminate progress bar.
    if (progress) {
      progress.value = 1
    }
    if (total) {
      total.value = undefined
    }

    // TODO: If abortSignal is aborted, we should attempt to discard the upload (with fetch's keepalive set).

    // TODO: Pass and store lastModified timestamp for the file (as different timestamp than current uploaded "at" timestamp).
    const beginUploadRequest: StorageBeginUploadRequest = {
      size: file.size,
      mediaType: file.type || "application/octet-stream",
      filename: file.name || "",
    }
    const beginUploadResponse = await postJSON<StorageBeginUploadResponse>(
      router.apiResolve({
        name: "StorageBeginUpload",
      }).href,
      beginUploadRequest,
      abortSignal,
      progress,
    )
    if (abortSignal.aborted) {
      return ""
    }
    for (let chunkStart = 0; chunkStart < file.size; chunkStart += MAX_PAYLOAD_SIZE) {
      const chunkEnd = Math.min(chunkStart + MAX_PAYLOAD_SIZE, file.size)
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
        file.slice(chunkStart, chunkEnd),
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

    try {
      await postJSON(
        router.apiResolve({
          name: "StorageEndUpload",
          params: {
            session: beginUploadResponse.session,
          },
        }).href,
        {},
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
        const { doc: status } = await getURLDirect<StorageUploadStatus>(uploadStatusURL, abortSignal, progress)
        if (abortSignal.aborted) {
          return ""
        }
        if (status.id) {
          return status.id
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
    if (progress) {
      progress.value = initialProgress
    }
    if (total) {
      total.value = undefined
    }
  }
}
