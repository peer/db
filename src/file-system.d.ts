// File System Access API types (Chromium-only, not in default lib-dom).
interface SaveFilePickerOptions {
  suggestedName?: string
  types?: Array<{
    description?: string
    accept: Record<string, string[]>
  }>
}

interface DirectoryPickerOptions {
  id?: string
  mode?: "read" | "readwrite"
  startIn?: FileSystemHandle | "desktop" | "documents" | "downloads" | "music" | "pictures" | "videos"
}

declare global {
  interface Window {
    showSaveFilePicker?: (options?: SaveFilePickerOptions) => Promise<FileSystemFileHandle>
    showDirectoryPicker?: (options?: DirectoryPickerOptions) => Promise<FileSystemDirectoryHandle>
  }
}

export {}
