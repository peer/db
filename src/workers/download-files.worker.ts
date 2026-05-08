// Web worker for downloading files individually to a directory.
// Receives a list of files and a FileSystemDirectoryHandle, then streams each response body
// directly into the target file via ReadableStream.pipeTo() so nothing is buffered in memory.
//
// Supports a graceful cancel: the main thread can post { type: "cancel" } and the worker
// aborts pipeTo (which aborts the writable too), then posts a final "done".

import type { DownloadFile, DownloadFilesWorkerInput, DownloadFilesWorkerOutput } from "@/types"

let cancelController: AbortController | null = null

self.onmessage = (e: MessageEvent<DownloadFilesWorkerInput>) => {
  const msg = e.data
  if (msg.type === "cancel") {
    cancelController?.abort()
    return
  }
  if (cancelController !== null) {
    // Already started; ignore duplicate "start".
    return
  }
  cancelController = new AbortController()
  void run(msg.files, msg.directoryHandle, cancelController.signal)
}

async function run(files: DownloadFile[], directoryHandle: FileSystemDirectoryHandle, signal: AbortSignal) {
  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      self.postMessage({ type: "progress", completed: i, total: files.length, currentFile: file.name } satisfies DownloadFilesWorkerOutput)

      const response = await fetch(file.url, { signal })
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }
      if (!response.body) {
        throw new Error(`failed to fetch ${file.name}: response has no body`)
      }

      const fileHandle = await directoryHandle.getFileHandle(file.name, { create: true })
      const writable = await fileHandle.createWritable()
      // pipeTo closes the writable on success and aborts it (cleaning up the swap file)
      // when signal fires, so cancellation does not leave a partial file at the target path.
      await response.body.pipeTo(writable, { signal })
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadFilesWorkerOutput)
    self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
  } catch (err) {
    if (signal.aborted) {
      // Cancelled by the main thread: report a clean completion so the overlay closes.
      self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
    } else {
      const message = err instanceof Error ? err.message : String(err)
      self.postMessage({ type: "error", message } satisfies DownloadFilesWorkerOutput)
    }
  }
}
