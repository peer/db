import type { Ref } from "vue"

import type { DownloadFile, DownloadFilesWorkerInput, DownloadFilesWorkerOutput, DownloadZipWorkerInput, DownloadZipWorkerOutput } from "@/types"

import { ref } from "vue"

export type DownloadMode = "zip" | "files"

export function useDownload(abortController: AbortController, updateSearchSessionProgress: Ref<number>) {
  const downloadMode = ref<DownloadMode>("zip")
  const completed = ref(0)
  const total = ref(0)
  const currentFile = ref("")
  const error = ref<string | null>(null)

  // Set while a worker is running; lets cancelDownload tear it down.
  let cancelCurrent: (() => void) | null = null

  // Blob fallback only: when the worker had no FileSystemFileHandle, it posts the assembled
  // Blob back here and we trigger a download via <a download>.
  function handleZipBlob(blob: Blob) {
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = "download.zip"
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  // Wrap a worker in a promise that resolves when the worker finishes, errors, is cancelled, or the owner aborts.
  function runWorker(worker: Worker, message: Extract<DownloadZipWorkerInput | DownloadFilesWorkerInput, { type: "start" }>): Promise<void> {
    return new Promise((resolve) => {
      let resolved = false
      let terminateTimer: ReturnType<typeof setTimeout> | null = null

      function finish() {
        if (resolved) {
          return
        }
        resolved = true
        if (terminateTimer !== null) {
          clearTimeout(terminateTimer)
          terminateTimer = null
        }
        abortController.signal.removeEventListener("abort", abortHandler)
        cancelCurrent = null
        worker.onmessage = null
        worker.onerror = null
        // Terminate on every path (success, error, graceful cancel, hard timeout) so the
        // worker thread does not linger waiting for GC.
        worker.terminate()
        resolve()
      }

      function abortHandler() {
        // Already shutting down. Do not restart the terminate timer.
        if (terminateTimer !== null) {
          return
        }
        // Ask the worker to abort cleanly. It will post "done" / "error", which routes through
        // onmessage below and calls finish() (clearing the timer and terminating). If the worker
        // does not respond within 1 second, the timer fires finish() to force-terminate.
        worker.postMessage({ type: "cancel" })
        terminateTimer = setTimeout(finish, 1000) // 1 second.
      }

      // Both user-initiated cancel and owner abort go through the same handler: graceful first,
      // hard terminate after a 1-second grace period.
      cancelCurrent = abortHandler

      abortController.signal.addEventListener("abort", abortHandler, { once: true })

      worker.onmessage = (e: MessageEvent<DownloadZipWorkerOutput | DownloadFilesWorkerOutput>) => {
        const msg = e.data
        if (msg.type === "progress") {
          completed.value = msg.completed
          total.value = msg.total
          currentFile.value = msg.currentFile
        } else if (msg.type === "blob") {
          handleZipBlob(msg.blob)
          finish()
        } else if (msg.type === "done") {
          finish()
        } else if (msg.type === "error") {
          error.value = msg.message
          finish()
        }
      }

      worker.onerror = (e) => {
        error.value = e.message || "Worker error."
        finish()
      }

      worker.postMessage(message)
    })
  }

  async function startZipDownload(files: DownloadFile[]) {
    if (abortController.signal.aborted) {
      return
    }
    if (updateSearchSessionProgress.value > 0) {
      if (total.value > 0) {
        throw new Error("download already in progress")
      }
      throw new Error("search session update in progress")
    }
    // Bump the progress immediately after the check so a re-entrant call cannot pass the guard while we await below.
    updateSearchSessionProgress.value += 1
    try {
      // Try to use showSaveFilePicker if available; otherwise the worker assembles a Blob
      // and we fall back to a <a download> click.
      let fileHandle: FileSystemFileHandle | null = null
      if (window.showSaveFilePicker) {
        try {
          fileHandle = await window.showSaveFilePicker({
            suggestedName: "download.zip",
            types: [
              {
                description: "ZIP archive",
                accept: { "application/zip": [".zip"] },
              },
            ],
          })
        } catch {
          // User cancelled the dialog.
          return
        }
      }

      downloadMode.value = "zip"
      completed.value = 0
      total.value = files.length
      currentFile.value = ""
      error.value = null

      const worker = new Worker(new URL("@/workers/download-zip.worker.ts", import.meta.url), { type: "module" })
      await runWorker(worker, { type: "start", files, fileHandle })
    } finally {
      // Reset total so the overlay's "open" condition (total > 0) flips back to closed.
      total.value = 0
      updateSearchSessionProgress.value -= 1
    }
  }

  async function startBulkDownload(files: DownloadFile[]) {
    if (abortController.signal.aborted) {
      return
    }
    if (!window.showDirectoryPicker) {
      throw new Error("showDirectoryPicker is not available")
    }
    if (updateSearchSessionProgress.value > 0) {
      if (total.value > 0) {
        throw new Error("download already in progress")
      }
      throw new Error("search session update in progress")
    }
    // Bump the progress immediately after the check so a re-entrant call cannot pass the guard while we await below.
    updateSearchSessionProgress.value += 1
    try {
      let directoryHandle: FileSystemDirectoryHandle
      try {
        directoryHandle = await window.showDirectoryPicker({ mode: "readwrite" })
      } catch {
        // User cancelled the dialog.
        return
      }

      downloadMode.value = "files"
      completed.value = 0
      total.value = files.length
      currentFile.value = ""
      error.value = null

      const worker = new Worker(new URL("@/workers/download-files.worker.ts", import.meta.url), { type: "module" })
      await runWorker(worker, { type: "start", files, directoryHandle })
    } finally {
      // Reset total so the overlay's "open" condition (total > 0) flips back to closed.
      total.value = 0
      updateSearchSessionProgress.value -= 1
    }
  }

  function cancelDownload() {
    if (cancelCurrent) {
      // Active download: terminate worker and resolve its promise so the start function's finally runs.
      cancelCurrent()
    } else {
      // No active download; clear any displayed error so the dialog closes.
      error.value = null
    }
  }

  return {
    downloadMode,
    completed,
    total,
    currentFile,
    error,
    startZipDownload,
    startBulkDownload,
    cancelDownload,
  }
}
