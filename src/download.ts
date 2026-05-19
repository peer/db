import type { DeepReadonly, Ref } from "vue"
import type { Router } from "vue-router"

import type {
  DownloadFile,
  DownloadFilesWorkerInput,
  DownloadFilesWorkerOutput,
  DownloadingPhase,
  DownloadZipWorkerInput,
  DownloadZipWorkerOutput,
  Result,
} from "@/types"

import { ref } from "vue"

import { getURL, headURLDirect } from "@/api"
import { D, HTMLClaim, LinkClaim } from "@/document"
import { delay, parseUrl } from "@/utils"

// RFC 5987 extended form: filename*=<charset>'<lang>'<percent-encoded value>.
// Capture group 2 holds the percent-encoded value, ending at a ; or end of string.
const CONTENT_DISPOSITION_FILENAME_EXT = /filename\*=([^']*)'[^']*'([^;]+)/i
// Plain form: filename="quoted" or filename=token. Capture group 2 is the quoted body
// (without surrounding quotes), capture group 3 is the unquoted token.
const CONTENT_DISPOSITION_FILENAME_PLAIN = /filename=("([^"]*)"|([^;]*))/i

export function useDownload(abortController: AbortController, router: Router, results: Ref<DeepReadonly<Result[]>>) {
  // Drives the overlay's message and the dialog's open state. null means idle.
  const downloadingPhase = ref<DownloadingPhase | null>(null)
  const completed = ref(0)
  const total = ref(0)
  const currentFile = ref("")
  const error = ref<string | null>(null)

  // Set while a download lifecycle is active; lets cancelDownload abort preparation, terminate
  // the worker, dismiss an empty notice, etc. Repointed as we move through phases.
  let cancelCurrent: (() => void) | null = null

  // Filename for the zip currently being assembled. Set by startZipDownload and read by
  // handleZipBlob when the worker posts the Blob fallback back.
  let zipFilename = ""

  // Blob fallback only: when the worker had no FileSystemFileHandle, it posts the assembled
  // Blob back here and we trigger a download via <a download>.
  function handleZipBlob(blob: Blob) {
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = zipFilename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  // Minimum time the overlay stays at 100% before the we return from runWorker and close
  // the dialog. Padded only when the natural completion path is faster than this; if the
  // worker takes longer to wrap up after 100% (e.g. zip writePromise drain), no extra wait.
  const MIN_HOLD_MS = 400 // 300ms (progress bar transition duration) + 100ms extra.

  // Wrap a worker in a promise that resolves when the worker finishes successfully or is cancelled,
  // and rejects when the worker reports an error.
  function runWorker(worker: Worker, message: Extract<DownloadZipWorkerInput | DownloadFilesWorkerInput, { type: "start" }>): Promise<void> {
    return new Promise((resolve, reject) => {
      let settled = false
      let terminateTimer: ReturnType<typeof setTimeout> | null = null
      // Timestamp of the first progress message that reported completed === total. Used in
      // finish() to ensure the overlay shows the final 100% state for at least MIN_HOLD_MS.
      let completedAt: number | null = null

      function finish(err?: Error) {
        if (settled) {
          return
        }
        settled = true
        if (terminateTimer !== null) {
          clearTimeout(terminateTimer)
          terminateTimer = null
        }
        abortController.signal.removeEventListener("abort", abortHandler)
        worker.onmessage = null
        worker.onerror = null
        // Terminate on every path (success, error, graceful cancel, hard timeout) so the
        // worker thread does not linger waiting for GC.
        worker.terminate()

        const settle = err !== undefined ? () => reject(err) : resolve
        // Skip the hold if we never reached 100% or the owner is being torn down (component unmount).
        const remaining = completedAt === null || abortController.signal.aborted ? 0 : MIN_HOLD_MS - (Date.now() - completedAt)
        if (remaining <= 0) {
          cancelCurrent = null
          settle()
          return
        }
        // Hold the overlay at 100% for the remaining time. Point cancelCurrent at a shortcut
        // so a cancel-button click during the hold closes the dialog immediately instead of
        // waiting out the timer.
        const holdTimer = setTimeout(() => {
          cancelCurrent = null
          settle()
        }, remaining)
        cancelCurrent = () => {
          clearTimeout(holdTimer)
          cancelCurrent = null
          settle()
        }
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
          if (completedAt === null && msg.total > 0 && msg.completed >= msg.total) {
            completedAt = Date.now()
          }
        } else if (msg.type === "blob") {
          try {
            handleZipBlob(msg.blob)
          } catch (err) {
            finish(err instanceof Error ? err : new Error(String(err)))
            return
          }
          finish()
        } else if (msg.type === "done") {
          finish()
        } else if (msg.type === "error") {
          finish(new Error(msg.message))
        }
      }

      worker.onerror = (e) => {
        finish(new Error(e.message || "worker error"))
      }

      worker.postMessage(message)
    })
  }

  // Test if an iri targets the StorageGet route (/f/:id) of this site. Returns the file id
  // on match, null otherwise. Uses vue-router's resolve() so the match stays in lockstep
  // with the actual route definition.
  //
  // classifyLink function is similar. Keep in sync as needed.
  function matchStorageRoute(iri: string): string | null {
    if (!iri) return null

    let url: URL
    try {
      url = parseUrl(iri)
    } catch {
      return null
    }

    if (url.origin !== window.location.origin) {
      return null
    }

    const resolved = router.resolve(url.pathname)
    const matched = resolved.matched.length > 0

    if (!matched || resolved.name !== "StorageGet") {
      return null
    }

    const id = resolved.params.id
    return typeof id === "string" ? id : null
  }

  // Parse a Content-Disposition header value and return the filename, preferring the
  // RFC 5987 filename* form (which carries an explicit charset and percent-encoding) over
  // the plain filename= form.
  function parseContentDispositionFilename(header: string | null): string | null {
    if (!header) {
      return null
    }
    const ext = CONTENT_DISPOSITION_FILENAME_EXT.exec(header)
    if (ext) {
      try {
        return decodeURIComponent(ext[2].trim())
      } catch {
        // Fall through to the plain form.
      }
    }
    const plain = CONTENT_DISPOSITION_FILENAME_PLAIN.exec(header)
    if (plain) {
      return (plain[2] ?? plain[3] ?? "").trim() || null
    }
    return null
  }

  // HEAD a file to read its Content-Disposition filename and build a DownloadFile.
  // Falls back to the file id as the name when no usable filename is advertised.
  async function fetchFileMetadata(id: string, signal: AbortSignal): Promise<DownloadFile> {
    const url = router.resolve({ name: "StorageGet", params: { id } }).href
    const headers = await headURLDirect(url, signal, null)
    const name = parseContentDispositionFilename(headers.get("content-disposition")) ?? id
    completed.value += 1
    return { name, url }
  }

  // Walk every search result document, collect all LinkClaim iris that target our StorageGet
  // route, dedupe by file id, and resolve a DownloadFile (with HEAD-derived filename) for each.
  // Progress reflects per-document completion; HEAD requests run in parallel with doc fetches
  // and are awaited at the end.
  async function prepareFiles(signal: AbortSignal): Promise<DownloadFile[]> {
    // Snapshot once so a search update mid-preparation cannot shift the document set under us.
    const snapshot = results.value
    downloadingPhase.value = "preparing"
    currentFile.value = ""
    error.value = null
    completed.value = 0
    total.value = snapshot.length

    // Dedupe by file id, and reuse the same in-flight HEAD promise across documents that
    // reference the same file.
    const files = new Map<string, Promise<DownloadFile>>()

    function recordFile(iri: string) {
      const id = matchStorageRoute(iri)
      if (id !== null && !files.has(id)) {
        total.value += 1
        files.set(id, fetchFileMetadata(id, signal))
      }
    }

    const docPromises = snapshot.map(async (r) => {
      const url = router.apiResolve({ name: "DocumentGet", params: { id: r.id } }).href
      const { doc: rawDoc } = await getURL<object>(url, null, signal, null)
      const d = new D(rawDoc)
      for (const claim of d.claims.AllClaimsWithSub()) {
        if (claim instanceof LinkClaim) {
          // Extract every link claim that points at our StorageGet route.
          recordFile(claim.iri)
        } else if (claim instanceof HTMLClaim) {
          // Parse the HTML and extract every <a href="..."> that points at our StorageGet route.
          const parsed = new DOMParser().parseFromString(claim.html, "text/html")
          for (const a of parsed.querySelectorAll("a[href]")) {
            const href = a.getAttribute("href")
            if (href !== null) {
              recordFile(href)
            }
          }
        }
      }
      completed.value += 1
    })

    await Promise.all(docPromises)
    const resolved = await Promise.all(files.values())
    // Sort by url for a deterministic order (the url embeds the file id).
    resolved.sort((a, b) => (a.url < b.url ? -1 : a.url > b.url ? 1 : 0))

    // Hold the progress bar at 100% for MIN_HOLD_MS so the user perceives preparation
    // completing before the phase transitions to "downloading" or "empty". delay() rejects
    // with the signal's abort reason if cancel hits during the hold.
    await delay(MIN_HOLD_MS, signal)

    return resolved
  }

  async function startZipDownload(filename: string = "download.zip") {
    if (abortController.signal.aborted) {
      return
    }
    if (downloadingPhase.value !== null) {
      throw new Error("download already in progress")
    }
    // Claim the slot immediately so a concurrent call sees us as busy without waiting
    // for the picker / prepareFiles to flip the phase. The overlay stays closed for
    // "picking" (the destination picker is its own modal); it opens once prepareFiles
    // transitions to "preparing".
    downloadingPhase.value = "picking"
    // Clear any leftover error from a previous run.
    error.value = null

    const preparationController = new AbortController()
    // Owner abort propagates to preparation too.
    const onOwnerAbort = () => preparationController.abort()
    abortController.signal.addEventListener("abort", onOwnerAbort, { once: true })

    try {
      zipFilename = filename

      // Try to use showSaveFilePicker if available; otherwise the worker assembles a Blob
      // and we fall back to a <a download> click.
      let fileHandle: FileSystemFileHandle | null = null
      if (window.showSaveFilePicker) {
        try {
          fileHandle = await window.showSaveFilePicker({
            suggestedName: zipFilename,
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
      if (abortController.signal.aborted) {
        return
      }

      // During preparation, Cancel aborts preparation; once the worker starts,
      // runWorker overwrites cancelCurrent with its own abort/terminate handler.
      cancelCurrent = () => preparationController.abort()

      const files = await prepareFiles(preparationController.signal)

      if (files.length === 0) {
        downloadingPhase.value = "empty"
        // Close button dismisses the notice.
        cancelCurrent = () => {
          cancelCurrent = null
          downloadingPhase.value = null
        }
        return
      }

      downloadingPhase.value = "downloading"
      completed.value = 0
      total.value = files.length
      currentFile.value = ""

      const worker = new Worker(new URL("@/workers/download-zip.worker.ts", import.meta.url), { type: "module" })
      await runWorker(worker, { type: "start", files, fileHandle })
    } catch (err) {
      // Cancellation (owner abort or user cancel during preparation) is a clean close, not an error.
      if (abortController.signal.aborted || preparationController.signal.aborted) {
        return
      }
      console.error("download.startZipDownload", err)
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      error.value = `${err}`
    } finally {
      abortController.signal.removeEventListener("abort", onOwnerAbort)
      // Do not clobber the empty-notice state.
      if (downloadingPhase.value !== "empty") {
        downloadingPhase.value = null
      }
    }
  }

  async function startBulkDownload() {
    if (abortController.signal.aborted) {
      return
    }
    if (!window.showDirectoryPicker) {
      throw new Error("showDirectoryPicker is not available")
    }
    if (downloadingPhase.value !== null) {
      throw new Error("download already in progress")
    }
    // Claim the slot immediately so a concurrent call sees us as busy without waiting
    // for the picker / prepareFiles to flip the phase. The overlay stays closed for
    // "picking" (the directory picker is its own modal); it opens once prepareFiles
    // transitions to "preparing".
    downloadingPhase.value = "picking"
    // Clear any leftover error from a previous run.
    error.value = null

    const preparationController = new AbortController()
    const onOwnerAbort = () => preparationController.abort()
    abortController.signal.addEventListener("abort", onOwnerAbort, { once: true })

    try {
      let directoryHandle: FileSystemDirectoryHandle
      try {
        directoryHandle = await window.showDirectoryPicker({ mode: "readwrite" })
      } catch {
        // User cancelled the dialog.
        return
      }
      if (abortController.signal.aborted) {
        return
      }

      // During preparation, Cancel aborts preparation; once the worker starts,
      // runWorker overwrites cancelCurrent with its own abort/terminate handler.
      cancelCurrent = () => preparationController.abort()

      const files = await prepareFiles(preparationController.signal)

      if (files.length === 0) {
        downloadingPhase.value = "empty"
        cancelCurrent = () => {
          cancelCurrent = null
          downloadingPhase.value = null
        }
        return
      }

      downloadingPhase.value = "downloading"
      completed.value = 0
      total.value = files.length
      currentFile.value = ""

      const worker = new Worker(new URL("@/workers/download-files.worker.ts", import.meta.url), { type: "module" })
      await runWorker(worker, { type: "start", files, directoryHandle })
    } catch (err) {
      if (abortController.signal.aborted || preparationController.signal.aborted) {
        return
      }
      console.error("download.startBulkDownload", err)
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      error.value = `${err}`
    } finally {
      abortController.signal.removeEventListener("abort", onOwnerAbort)
      // Do not clobber the empty-notice state.
      if (downloadingPhase.value !== "empty") {
        downloadingPhase.value = null
      }
    }
  }

  function cancelDownload() {
    if (cancelCurrent) {
      // Active phase. Preparation abort, worker terminate, or empty-notice dismiss.
      cancelCurrent()
      return
    }
    // No active phase. Clear any displayed error and downloading phase so the dialog closes.
    error.value = null
    downloadingPhase.value = null
  }

  return {
    downloadingPhase,
    completed,
    total,
    currentFile,
    error,
    startZipDownload,
    startBulkDownload,
    cancelDownload,
  }
}
