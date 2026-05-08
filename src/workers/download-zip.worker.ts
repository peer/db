// Web worker for downloading files and creating a zip archive.
// When the main thread provides a FileSystemFileHandle, the worker streams zip output
// directly to disk via createWritable. Otherwise it assembles a Blob and posts it back
// for the <a download> fallback.
//
// Supports a graceful cancel: the main thread can post { type: "cancel" } and the worker
// aborts any in-flight fetch / writable, then posts a final "done".

import type { DownloadFile, DownloadZipWorkerInput, DownloadZipWorkerOutput } from "@/types"

import { Zip, ZipDeflate, ZipPassThrough } from "fflate"

// Media types that are already compressed and should not be deflated.
const compressedMediaTypes = new Set([
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/webp",
  "image/avif",
  "video/mp4",
  "video/webm",
  "audio/mpeg",
  "audio/ogg",
  "application/zip",
  "application/gzip",
  "application/x-7z-compressed",
  "application/x-rar-compressed",
])

function isCompressedType(contentType: string | null): boolean {
  if (!contentType) {
    return false
  }
  // Extract media type without parameters.
  const mediaType = contentType.split(";")[0].trim().toLowerCase()
  return compressedMediaTypes.has(mediaType)
}

let cancelController: AbortController | null = null

self.onmessage = (e: MessageEvent<DownloadZipWorkerInput>) => {
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
  void run(msg.files, msg.fileHandle, cancelController.signal)
}

async function run(files: DownloadFile[], fileHandle: FileSystemFileHandle | null, signal: AbortSignal) {
  // When fileHandle is provided we stream into the writable; otherwise we accumulate chunks
  // into a Blob and post it back.
  const chunks: Uint8Array<ArrayBuffer>[] = []
  let writable: FileSystemWritableFileStream | null = null
  // Sequential write chain; each ondata appends a write/close to keep on-disk order correct.
  let writePromise: Promise<void> = Promise.resolve()
  let zipErrorMessage: string | null = null

  try {
    if (fileHandle) {
      writable = await fileHandle.createWritable()
    }
    const w = writable

    const zip = new Zip()
    zip.ondata = (err, chunk, final) => {
      // Once we've recorded an error, ignore any further callbacks so we don't queue writes
      // or buffer data that will be discarded anyway, and so we keep the first error message.
      if (zipErrorMessage !== null) {
        return
      }
      if (err) {
        zipErrorMessage = err.message
        return
      }
      // fflate's chunks are backed by a regular ArrayBuffer (not SharedArrayBuffer);
      // narrow the type so write() and Blob() accept it.
      const c = chunk as Uint8Array<ArrayBuffer>
      if (w) {
        writePromise = writePromise.then(() => w.write(c))
        if (final) {
          writePromise = writePromise.then(() => w.close())
        }
      } else {
        chunks.push(c)
      }
    }

    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      self.postMessage({ type: "progress", completed: i, total: files.length, currentFile: file.name } satisfies DownloadZipWorkerOutput)

      const response = await fetch(file.url, { signal })
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }
      if (!response.body) {
        throw new Error(`failed to fetch ${file.name}: response has no body`)
      }

      const contentType = response.headers.get("Content-Type")

      // Use passthrough for already-compressed files, deflate for others.
      let entry: ZipPassThrough | ZipDeflate
      if (isCompressedType(contentType)) {
        entry = new ZipPassThrough(file.name)
      } else {
        entry = new ZipDeflate(file.name, { level: 6 })
      }
      zip.add(entry)

      // Stream the response body into the zip entry so we don't buffer the whole source
      // file in memory. We hold one chunk back so the last push can carry final=true.
      // The body is auto-cancelled when signal aborts (fetch wires the signal through),
      // which makes reader.read() reject and exits the loop via the catch.
      const reader = response.body.getReader()
      let buffered: Uint8Array | null = null
      while (true) {
        const { done, value } = await reader.read()
        if (done) {
          entry.push(buffered ?? new Uint8Array(0), true)
          break
        }
        if (buffered !== null) {
          entry.push(buffered, false)
          if (zipErrorMessage !== null) {
            throw new Error(zipErrorMessage)
          }
        }
        buffered = value
      }

      if (zipErrorMessage !== null) {
        throw new Error(zipErrorMessage)
      }
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadZipWorkerOutput)
    zip.end()

    if (zipErrorMessage !== null) {
      throw new Error(zipErrorMessage)
    }

    if (w) {
      // Drain queued writes (and the close) before declaring the download done.
      await writePromise
      self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
    } else {
      const blob = new Blob(chunks, { type: "application/zip" })
      self.postMessage({ type: "blob", blob } satisfies DownloadZipWorkerOutput)
    }
  } catch (err) {
    if (writable) {
      // Cancel partial output so the swap file is cleaned up and the original target is untouched.
      try {
        await writable.abort()
      } catch {
        // Ignore abort errors.
      }
      // Drain any queued writes that reject from the abort to avoid unhandled rejections.
      await writePromise.catch(() => {})
    }
    if (signal.aborted) {
      // Cancelled by the main thread: report a clean completion so the overlay closes.
      self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
    } else {
      const message = err instanceof Error ? err.message : String(err)
      self.postMessage({ type: "error", message } satisfies DownloadZipWorkerOutput)
    }
  }
}
