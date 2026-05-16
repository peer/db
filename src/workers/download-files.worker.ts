// Web worker for downloading files individually to a directory.
// Receives a list of files and a FileSystemDirectoryHandle, then streams each response body
// directly into the target file via ReadableStream.pipeTo() so nothing is buffered in memory.
//
// Supports a graceful cancel: the main thread can post { type: "cancel" } and the worker
// aborts pipeTo (which aborts the writable too), then posts a final "done".

import type { DownloadFile, DownloadFilesWorkerInput, DownloadFilesWorkerOutput } from "@/types"

import { safeFilename } from "@/path"

// Returns true if the directory already contains an entry with the given name (file or directory).
// The File System Access API does not expose an atomic "create-if-not-exists", so we approximate
// it with a lookup; there is a small benign race window between the lookup and the subsequent create.
async function entryExists(directoryHandle: FileSystemDirectoryHandle, name: string): Promise<boolean> {
  try {
    await directoryHandle.getFileHandle(name)
    return true
  } catch (err) {
    if (err instanceof DOMException) {
      if (err.name === "NotFoundError") {
        return false
      }
      if (err.name === "TypeMismatchError") {
        // Something with this name exists (e.g. a directory); treat as taken.
        return true
      }
    }
    throw err
  }
}

// Returns name (or name_1.ext, name_2.ext, ...) - the first variant that does not collide with
// an existing entry in the directory. Splits on the first dot so compound extensions like
// .tar.gz survive intact (archive.tar.gz -> archive_1.tar.gz). Leading-dot ("hidden") names are
// treated as having no extension so the suffix lands at the end.
async function uniqueFilename(directoryHandle: FileSystemDirectoryHandle, name: string): Promise<string> {
  if (!(await entryExists(directoryHandle, name))) {
    return name
  }
  const dotIndex = name.indexOf(".")
  const prefix = dotIndex > 0 ? name.substring(0, dotIndex) : name
  const suffix = dotIndex > 0 ? name.substring(dotIndex) : ""
  for (let n = 1; ; n++) {
    const candidate = `${prefix}_${n}${suffix}`
    if (!(await entryExists(directoryHandle, candidate))) {
      return candidate
    }
  }
}

let cancelController: AbortController | null = null

self.onmessage = async (e: MessageEvent<DownloadFilesWorkerInput>) => {
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
  await run(msg.files, msg.directoryHandle, cancelController.signal)
}

async function run(files: DownloadFile[], directoryHandle: FileSystemDirectoryHandle, signal: AbortSignal) {
  // Tracks the name of the file currently being written. Set after getFileHandle materializes
  // the 0-byte placeholder, cleared after pipeTo finishes (close has run, content is on disk).
  // On cancel/error, anything still set here is a partial file we should remove.
  let pendingName: string | null = null
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

      // Sanitize the name so OS/filesystem-invalid characters and Windows-reserved device names
      // don't make getFileHandle reject. Then dedupe so we never overwrite an existing entry.
      const targetName = await uniqueFilename(directoryHandle, safeFilename(file.name))
      pendingName = targetName
      const fileHandle = await directoryHandle.getFileHandle(targetName, { create: true })
      const writable = await fileHandle.createWritable()
      // pipeTo closes the writable on success and aborts it (cleaning up the swap file)
      // when signal fires, so cancellation does not leave a partial file at the target path.
      await response.body.pipeTo(writable, { signal })
      // pipeTo resolved => close ran => content is committed. Anything from now on counts as
      // a "fully written" file and should not be removed if a later iteration cancels.
      pendingName = null
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadFilesWorkerOutput)
    self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
  } catch (err) {
    // Remove the placeholder for the file we were in the middle of writing. Earlier iterations
    // already cleared pendingName once their close() succeeded, so they won't be touched.
    if (pendingName !== null) {
      try {
        await directoryHandle.removeEntry(pendingName)
      } catch {
        // Best-effort cleanup; ignore failures (NotFoundError, permission, etc.).
      }
    }
    if (signal.aborted) {
      // Cancelled by the main thread: report a clean completion so the overlay closes.
      self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
    } else {
      const message = err instanceof Error ? err.message : String(err)
      self.postMessage({ type: "error", message } satisfies DownloadFilesWorkerOutput)
    }
  }
}
