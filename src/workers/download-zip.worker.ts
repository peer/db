// Web worker for downloading files and creating a zip archive.
// Receives a list of files and a WritableStream, downloads each file,
// compresses them into a zip archive using fflate, and writes to the stream.

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
  const { files, writable } = e.data
  const writer = writable.getWriter()

  try {
    const zip = new Zip()
    zip.ondata = (err, chunk, final) => {
      if (err) {
        writer.abort(err.message).catch(() => {})
        self.postMessage({ type: "error", message: err.message } satisfies DownloadZipWorkerOutput)
        return
      }
      writer.write(chunk).catch(() => {})
      if (final) {
        writer.close().catch(() => {})
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
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadZipWorkerOutput)
    zip.end()
    self.postMessage({ type: "done" } satisfies DownloadZipWorkerOutput)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    try {
      writer.abort(message).catch(() => {})
    } catch {
      // Writer may already be closed.
    }
    self.postMessage({ type: "error", message } satisfies DownloadZipWorkerOutput)
  }
}
