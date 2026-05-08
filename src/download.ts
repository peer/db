import type { Ref } from "vue"

import type { DownloadFile, DownloadFilesWorkerOutput, DownloadZipWorkerOutput } from "@/types"

import { ref } from "vue"

export type DownloadMode = "zip" | "files"

export function useDownload(abortController: AbortController, updateSearchSessionProgress: Ref<number>) {
  const isDownloading = ref(false)
  const downloadMode = ref<DownloadMode>("zip")
  const completed = ref(0)
  const total = ref(0)
  const currentFile = ref("")
  const error = ref<string | null>(null)

  // File handle obtained from showSaveFilePicker (null when using Blob fallback).
  let zipFileHandle: FileSystemFileHandle | null = null
  // Set while a worker is running; lets cancelDownload tear it down.
  let cancelCurrent: (() => void) | null = null

  async function handleZipBlob(data: Uint8Array) {
    const blob = new Blob([data.buffer as ArrayBuffer], { type: "application/zip" })

    if (zipFileHandle) {
      // Write to the file handle obtained from showSaveFilePicker.
      const writable = await zipFileHandle.createWritable()
      await writable.write(blob)
      await writable.close()
    } else {
      // Blob fallback: trigger download via <a> element.
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = "download.zip"
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    }
  }

  // Wrap a worker in a promise that resolves when the worker finishes, errors, is cancelled, or the owner aborts.
  function runWorker(worker: Worker, message: unknown): Promise<void> {
    return new Promise((resolve) => {
      let resolved = false

      function finish() {
        if (resolved) {
          return
        }
        resolved = true
        abortController.signal.removeEventListener("abort", abortHandler)
        worker.onmessage = null
        worker.onerror = null
        cancelCurrent = null
        resolve()
      }

      function abortHandler() {
        worker.terminate()
        finish()
      }

      cancelCurrent = abortHandler

      abortController.signal.addEventListener("abort", abortHandler, { once: true })

      worker.onmessage = async (e: MessageEvent<DownloadZipWorkerOutput | DownloadFilesWorkerOutput>) => {
        const msg = e.data
        if (msg.type === "progress") {
          completed.value = msg.completed
          total.value = msg.total
          currentFile.value = msg.currentFile
        } else if (msg.type === "blob") {
          try {
            await handleZipBlob(msg.data)
          } catch (err) {
            error.value = err instanceof Error ? err.message : String(err)
          }
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
    if (isDownloading.value || abortController.signal.aborted) {
      return
    }
    // Set the flag immediately after the check so a re-entrant call cannot pass the guard while we await below.
    isDownloading.value = true
    updateSearchSessionProgress.value += 1
    try {
      // Try to use showSaveFilePicker if available.
      // Falls back to Blob download otherwise.
      zipFileHandle = null
      if (window.showSaveFilePicker) {
        try {
          zipFileHandle = await window.showSaveFilePicker({
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
      await runWorker(worker, { type: "start", files })
    } finally {
      isDownloading.value = false
      zipFileHandle = null
      updateSearchSessionProgress.value -= 1
    }
  }

  async function startBulkDownload(files: DownloadFile[]) {
    if (!window.showDirectoryPicker) {
      throw new Error("showDirectoryPicker is not available")
    }
    if (isDownloading.value || abortController.signal.aborted) {
      return
    }
    // Set the flag immediately after the check so a re-entrant call cannot pass the guard while we await below.
    isDownloading.value = true
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
      isDownloading.value = false
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
    isDownloading,
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
