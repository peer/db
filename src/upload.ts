import type { Ref } from "vue"
import type { Router } from "vue-router"

import type { StorageBeginUploadRequest, StorageBeginUploadResponse } from "@/types"

import { postBlob, postJSON } from "@/api"
import { encodeQuery } from "@/utils"

// 10 MB.
const maxPayloadSize = 10 << 20

export async function uploadFile(router: Router, file: File, abortSignal: AbortSignal, progress: Ref<number>): Promise<string> {
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
  for (let start = 0; start < file.size; start += maxPayloadSize) {
    await postBlob(
      router.apiResolve({
        name: "StorageUploadChunk",
        params: {
          session: beginUploadResponse.session,
        },
        // Because start is less than maxPayloadSize, toString() never uses scientific notation.
        query: encodeQuery({ start: start.toString() }),
      }).href,
      file.slice(start, Math.min(start + maxPayloadSize, file.size)),
      abortSignal,
      progress,
    )
    if (abortSignal.aborted) {
      return ""
    }
  }
  await postJSON(
    router.apiResolve({
      name: "StorageEndUpload",
      params: {
        session: beginUploadResponse.session,
      },
    }).href,
    {},
    abortSignal,
    progress,
  )
  if (abortSignal.aborted) {
    return ""
  }
  return beginUploadResponse.session
}
