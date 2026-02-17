// Web worker for downloading files individually to a directory.
// Receives a list of files and a FileSystemDirectoryHandle,
// downloads each file and saves it to the directory.

export type DownloadFilesWorkerInput = {
  type: "start"
  files: { name: string; url: string }[]
  directoryHandle: FileSystemDirectoryHandle
}

export type DownloadFilesWorkerOutput =
  | { type: "progress"; completed: number; total: number; currentFile: string }
  | { type: "done" }
  | { type: "error"; message: string }

self.onmessage = async (e: MessageEvent<DownloadFilesWorkerInput>) => {
  const { files, directoryHandle } = e.data

  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      self.postMessage({ type: "progress", completed: i, total: files.length, currentFile: file.name } satisfies DownloadFilesWorkerOutput)

      const response = await fetch(file.url)
      if (!response.ok) {
        throw new Error(`failed to fetch ${file.name}: ${response.status} ${response.statusText}`)
      }

      const data = await response.arrayBuffer()
      const fileHandle = await directoryHandle.getFileHandle(file.name, { create: true })
      const writable = await fileHandle.createWritable()
      await writable.write(data)
      await writable.close()
    }

    self.postMessage({ type: "progress", completed: files.length, total: files.length, currentFile: "" } satisfies DownloadFilesWorkerOutput)
    self.postMessage({ type: "done" } satisfies DownloadFilesWorkerOutput)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    self.postMessage({ type: "error", message } satisfies DownloadFilesWorkerOutput)
  }
}
