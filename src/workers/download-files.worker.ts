// Web worker for downloading files individually to a directory.
// Receives a list of files and a FileSystemDirectoryHandle, then streams each response body
// directly into the target file via ReadableStream.pipeTo() so nothing is buffered in memory.

import type { DownloadFilesWorkerInput, DownloadFilesWorkerOutput } from "@/types"

self.onmessage = async (e: MessageEvent<DownloadFilesWorkerInput>) => {
  const { files, directoryHandle } = e.data

  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      self.postMessage({ type: "progress", completed: i, total: files.length, currentFile: file.name } satisfies DownloadFilesWorkerOutput)

      const response = await fetch(file.url)
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }
      if (!response.body) {
        throw new Error(`failed to fetch ${file.name}: response has no body`)
      }

      const fileHandle = await directoryHandle.getFileHandle(file.name, { create: true })
      const writable = await fileHandle.createWritable()
      // pipeTo closes the writable on success and aborts it if the source errors mid-stream.
      await response.body.pipeTo(writable)
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadFilesWorkerOutput)
    self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    self.postMessage({ type: "error", message } satisfies DownloadFilesWorkerOutput)
  }
}
