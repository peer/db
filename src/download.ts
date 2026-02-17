import type { DownloadFile } from "@/types"
import type { DownloadFilesWorkerOutput } from "@/workers/download-files.worker"
import type { DownloadZipWorkerOutput } from "@/workers/download-zip.worker"

import { ref } from "vue"

import DownloadFilesWorkerFactory from "@/workers/download-files.worker?worker"
import DownloadZipWorkerFactory from "@/workers/download-zip.worker?worker"

export type DownloadMode = "zip" | "files"

export function useDownload() {
  const isDownloading = ref(false)
  const downloadMode = ref<DownloadMode>("zip")
  const completed = ref(0)
  const total = ref(0)
  const currentFile = ref("")
  const error = ref<string | null>(null)

  let activeWorker: Worker | null = null

  function reset() {
    isDownloading.value = false
    completed.value = 0
    total.value = 0
    currentFile.value = ""
    error.value = null
    activeWorker = null
  }

  function handleWorkerMessage(e: MessageEvent<DownloadZipWorkerOutput | DownloadFilesWorkerOutput>) {
    const msg = e.data
    if (msg.type === "progress") {
      completed.value = msg.completed
      total.value = msg.total
      currentFile.value = msg.currentFile
    } else if (msg.type === "done") {
      reset()
    } else if (msg.type === "error") {
      error.value = msg.message
      isDownloading.value = false
      activeWorker = null
    }
  }

  async function startZipDownload(files: DownloadFile[]) {
    if (isDownloading.value) {
      return
    }
    if (!window.showSaveFilePicker) {
      return
    }

    let fileHandle: FileSystemFileHandle
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

    isDownloading.value = true
    downloadMode.value = "zip"
    completed.value = 0
    total.value = files.length
    currentFile.value = ""
    error.value = null

    const writable = await fileHandle.createWritable()

    const worker = new DownloadZipWorkerFactory()
    activeWorker = worker
    worker.onmessage = handleWorkerMessage
    worker.onerror = (e) => {
      error.value = e.message || "Worker error."
      isDownloading.value = false
      activeWorker = null
    }
    worker.postMessage({ type: "start", files, writable: writable as unknown as WritableStream<Uint8Array> }, [writable as unknown as Transferable])
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

    const worker = new DownloadFilesWorkerFactory()
    activeWorker = worker
    worker.onmessage = handleWorkerMessage
    worker.onerror = (e) => {
      error.value = e.message || "Worker error."
      isDownloading.value = false
      activeWorker = null
    }
    worker.postMessage({ type: "start", files, directoryHandle }, [directoryHandle as unknown as Transferable])
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
