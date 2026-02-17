import type { DownloadFile, DownloadFilesWorkerOutput, DownloadZipWorkerOutput } from "@/types"

import { ref } from "vue"

export type DownloadMode = "zip" | "files"

export function useDownload() {
  const isDownloading = ref(false)
  const downloadMode = ref<DownloadMode>("zip")
  const completed = ref(0)
  const total = ref(0)
  const currentFile = ref("")
  const error = ref<string | null>(null)

  let activeWorker: Worker | null = null
  // File handle obtained from showSaveFilePicker (null when using Blob fallback).
  let zipFileHandle: FileSystemFileHandle | null = null

  function reset() {
    isDownloading.value = false
    completed.value = 0
    total.value = 0
    currentFile.value = ""
    error.value = null
    activeWorker = null
    zipFileHandle = null
  }

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

    reset()
  }

  function handleWorkerMessage(e: MessageEvent<DownloadZipWorkerOutput | DownloadFilesWorkerOutput>) {
    const msg = e.data
    if (msg.type === "progress") {
      completed.value = msg.completed
      total.value = msg.total
      currentFile.value = msg.currentFile
    } else if (msg.type === "blob") {
      handleZipBlob(msg.data).catch((err) => {
        error.value = err instanceof Error ? err.message : String(err)
        isDownloading.value = false
        activeWorker = null
        zipFileHandle = null
      })
    } else if (msg.type === "done") {
      reset()
    } else if (msg.type === "error") {
      error.value = msg.message
      isDownloading.value = false
      activeWorker = null
      zipFileHandle = null
    }
  }

  async function startZipDownload(files: DownloadFile[]) {
    if (isDownloading.value) {
      return
    }

    // Try to use showSaveFilePicker if available (Chrome/Edge).
    // Falls back to Blob download otherwise (Brave/Firefox/Safari).
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

    isDownloading.value = true
    downloadMode.value = "zip"
    completed.value = 0
    total.value = files.length
    currentFile.value = ""
    error.value = null

    const worker = new Worker(new URL("@/workers/download-zip.worker.ts", import.meta.url), { type: "module" })
    activeWorker = worker
    worker.onmessage = handleWorkerMessage
    worker.onerror = (e) => {
      error.value = e.message || "Worker error."
      isDownloading.value = false
      activeWorker = null
      zipFileHandle = null
    }
    worker.postMessage({ type: "start", files })
  }

  async function startBulkDownload(files: DownloadFile[]) {
    if (isDownloading.value) {
      return
    }
    if (!window.showDirectoryPicker) {
      return
    }

    let directoryHandle: FileSystemDirectoryHandle
    try {
      directoryHandle = await window.showDirectoryPicker({ mode: "readwrite" })
    } catch {
      // User cancelled the dialog.
      return
    }

    isDownloading.value = true
    downloadMode.value = "files"
    completed.value = 0
    total.value = files.length
    currentFile.value = ""
    error.value = null

    const worker = new Worker(new URL("@/workers/download-files.worker.ts", import.meta.url), { type: "module" })
    activeWorker = worker
    worker.onmessage = handleWorkerMessage
    worker.onerror = (e) => {
      error.value = e.message || "Worker error."
      isDownloading.value = false
      activeWorker = null
    }
    worker.postMessage({ type: "start", files, directoryHandle })
  }

  function cancelDownload() {
    if (activeWorker) {
      activeWorker.terminate()
      activeWorker = null
    }
    reset()
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
