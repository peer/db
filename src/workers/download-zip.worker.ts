// Web worker for downloading files and creating a zip archive.
// When the main thread provides a FileSystemFileHandle, the worker streams zip output
// directly to disk via createWritable. Otherwise it assembles a Blob and posts it back
// for the <a download> fallback.

import type { DownloadZipWorkerInput, DownloadZipWorkerOutput } from "@/types"

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

self.onmessage = async (e: MessageEvent<DownloadZipWorkerInput>) => {
  const { files, fileHandle } = e.data

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
      if (err) {
        zipErrorMessage ??= err.message
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

      const response = await fetch(file.url)
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }

      const contentType = response.headers.get("Content-Type")
      const data = new Uint8Array(await response.arrayBuffer())

      // Use passthrough for already-compressed files, deflate for others.
      let entry: ZipPassThrough | ZipDeflate
      if (isCompressedType(contentType)) {
        entry = new ZipPassThrough(file.name)
      } else {
        entry = new ZipDeflate(file.name, { level: 6 })
      }
      zip.add(entry)
      entry.push(data, true)

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
      // Cancel partial output so the file is not left half-written.
      try {
        await writable.abort()
      } catch {
        // Ignore abort errors.
      }
      // Drain any queued writes that reject from the abort to avoid unhandled rejections.
      await writePromise.catch(() => {})
    }
    const message = err instanceof Error ? err.message : String(err)
    self.postMessage({ type: "error", message } satisfies DownloadZipWorkerOutput)
  }
}
