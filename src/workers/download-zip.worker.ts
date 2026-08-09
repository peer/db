// Web worker for downloading files and creating a zip archive.
// When the main thread provides a FileSystemFileHandle, the worker streams zip output
// directly to disk via createWritable. Otherwise it assembles a Blob and posts it back
// for the <a download> fallback.
//
// Supports a graceful cancel: the main thread can post { type: "cancel" } and the worker
// aborts any in-flight fetch / writable, then posts a final "done".

import type { DownloadFile, DownloadZipWorkerInput, DownloadZipWorkerOutput } from "@/types"

import { Zip, ZipDeflate, ZipPassThrough } from "fflate"

import { safeFilename } from "@/path"

// Media types worth deflating: text/*, JSON, XML, and structured-syntax suffixes
// like application/ld+json or application/atom+xml. Everything else (images, video,
// audio, archives, office documents, PDFs, ...) is assumed to already be compressed.
function isCompressibleType(contentType: string | null): boolean {
  if (!contentType) {
    return false
  }
  // Extract media type without parameters.
  const mediaType = contentType.split(";")[0].trim().toLowerCase()
  if (mediaType.startsWith("text/")) {
    return true
  }
  if (mediaType === "application/json" || mediaType === "application/xml") {
    return true
  }
  if (mediaType.startsWith("application/") && (mediaType.endsWith("+json") || mediaType.endsWith("+xml"))) {
    return true
  }
  return false
}

let cancelController: AbortController | null = null

self.onmessage = async (e: MessageEvent<DownloadZipWorkerInput>) => {
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
  await run(msg.files, msg.fileHandle, cancelController.signal)
}

async function run(files: DownloadFile[], fileHandle: FileSystemFileHandle | null, signal: AbortSignal) {
  // When fileHandle is provided we stream into the writable; otherwise we accumulate chunks
  // into a Blob and post it back.
  const chunks: Uint8Array<ArrayBuffer>[] = []
  let writable: FileSystemWritableFileStream | null = null
  // Sequential write chain; each ondata appends a write/close to keep on-disk order correct.
  let writePromise: Promise<void> = Promise.resolve()
  let zipErrorMessage: string | null = null

  // Throws away any partial output: aborts the writable so its swap file is cleaned up and the
  // original target is left untouched, then drains the queued writes whose rejections from that
  // abort would otherwise be unhandled. Does nothing in the Blob fallback, where there is no writable.
  async function discardOutput() {
    if (!writable) {
      return
    }
    try {
      await writable.abort()
    } catch {
      // Ignore abort errors.
    }
    try {
      await writePromise
    } catch {
      // Ignore errors.
    }
  }

  try {
    if (fileHandle) {
      writable = await fileHandle.createWritable()
      if (signal.aborted) {
        await discardOutput()
        self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
        return
      }
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
      if (w) {
        writePromise = writePromise.then(() => w.write(chunk))
        if (final) {
          writePromise = writePromise.then(() => w.close())
        }
      } else {
        chunks.push(chunk)
      }
    }

    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      self.postMessage({ type: "progress", completed: i, total: files.length, currentFile: file.name } satisfies DownloadZipWorkerOutput)

      const response = await fetch(file.url, { signal })
      if (signal.aborted) {
        break
      }
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }
      if (!response.body) {
        throw new Error(`failed to fetch ${file.name}: response has no body`)
      }

      const contentType = response.headers.get("Content-Type")

      // Sanitize the entry name so the archive is portable across OS extractors.
      const entryName = safeFilename(file.name)

      // Deflate text-like payloads; pass everything else through since it is generally
      // already compressed (images, video, audio, archives, office documents, ...).
      let entry: ZipPassThrough | ZipDeflate
      if (isCompressibleType(contentType)) {
        entry = new ZipDeflate(entryName, { level: 6 })
      } else {
        entry = new ZipPassThrough(entryName)
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
        if (signal.aborted) {
          // The whole archive is discarded below, so this entry does not need its final push.
          break
        }
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

      if (signal.aborted) {
        break
      }

      if (zipErrorMessage !== null) {
        throw new Error(zipErrorMessage)
      }
    }

    if (signal.aborted) {
      // Cancelled by the main thread while a step was in flight. Drop the partial archive so
      // neither a truncated file on disk nor a Blob for the <a download> fallback survives the
      // cancel, and report a clean completion so the overlay closes.
      await discardOutput()
      self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
      return
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
    await discardOutput()
    if (signal.aborted) {
      // Cancelled by the main thread: report a clean completion so the overlay closes.
      self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
    } else {
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      const message = `${err}`
      self.postMessage({ type: "error", message } satisfies DownloadZipWorkerOutput)
    }
  }
}
