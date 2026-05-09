import type { BareItem, Key } from "structured-field-values"
import type { Component, DeepReadonly, Ref } from "vue"
import type { Composer } from "vue-i18n"
import type { Router } from "vue-router"

import type { ClaimTypes } from "@/document/claims"
import type { NONE } from "@/symbols"

export type RefSearchResult = {
  id: string
  count: number
  type: "ref"
}

export type AmountSearchResult = {
  id: string
  count: number
  type: "amount"
  unit?: string
}

export type TimeSearchResult = {
  id: string
  count: number
  type: "time"
}

export type FilterResult = RefSearchResult | AmountSearchResult | TimeSearchResult

export type Result = {
  id: string
}

export type RefFilterResult = {
  id: string
  count: number
}

export type HistogramAmountResult = {
  from: number
  count: number
}

export type HistogramTimeResult = {
  from: number
  count: number
}

export type RefFilter = {
  prop: string
  value: string
}

export type RefNoneFilter = {
  prop: string
  none: true
}

export type AmountFilter = {
  prop: string
  unit?: string
  gte?: number
  lte?: number
}

export type AmountNoneFilter = {
  prop: string
  unit?: string
  none: true
}

export type TimeFilter = {
  prop: string
  gte?: number
  lte?: number
}

export type TimeNoneFilter = {
  prop: string
  none: true
}

export type Filters =
  | {
      and: Filters[]
    }
  | {
      or: Filters[]
    }
  | {
      not: Filters
    }
  | { ref: RefFilter | RefNoneFilter }
  | { amount: AmountFilter | AmountNoneFilter }
  | { time: TimeFilter | TimeNoneFilter }

export type RefFilterState = (string | typeof NONE)[]

export type AmountFilterState = null | typeof NONE | { gte?: number; lte?: number }

export type TimeFilterState = null | typeof NONE | { gte?: number; lte?: number }

export type FiltersState = {
  ref: Record<string, RefFilterState>
  amount: Record<string, AmountFilterState>
  time: Record<string, TimeFilterState>
}

export type RefFilterStateChange = {
  type: "ref"
  id: string
  value: RefFilterState
}

export type AmountFilterStateChange = {
  type: "amount"
  id: string
  unit?: string
  value: AmountFilterState
}

export type TimeFilterStateChange = {
  type: "time"
  id: string
  value: TimeFilterState
}

export type FilterStateChange = RefFilterStateChange | AmountFilterStateChange | TimeFilterStateChange

export type ServerSearchSession = {
  id: string
  version: number
  view: ViewType
  query: string
  filters?: Filters
}

export type ClientSearchSession = {
  id: string
  version: number
  view: ViewType
  query: string
  filters?: FiltersState
}

export type CreateSearchSessionRequest = {
  view?: ViewType
  query: string
  filters?: FiltersState
}

export type SearchSessionRef = {
  id: string
  version: number
}

export type SiteContext = {
  domain: string
  build?: {
    version?: string
    buildTimestamp?: string
    revision?: string
  }
  title?: string
  logo?: string
  languagePriority?: {
    [language: string]: string[]
  }
  defaultLanguage?: string
  languageCodes?: {
    [documentId: string]: string
  }
  features: {
    searchResultsTable?: boolean
    editButtons?: boolean
    downloadButtons?: boolean
  }
}

export type RouteOptions = {
  handlers?: Record<string, true>
}

export type Route = RouteOptions & {
  path: string
  api?: RouteOptions
}

export type Routes = {
  [name: string]: Route
}

type ItemTypes = BareItem | BareItem[]

export type Metadata = Record<Key, ItemTypes>

export type QueryValues = Record<string, string | string[]>

export type QueryValuesWithOptional = Record<string, string | (string | null)[] | undefined | null>

export type StorageBeginUploadRequest = {
  size: number
  mediaType: string
  filename: string
}

export type StorageBeginUploadResponse = {
  session: string
}

export type StorageUploadStatus = {
  active: boolean
  id?: string
  discarded?: boolean
}

export type DocumentEditStatus = {
  active: boolean
  changeset?: string
  discarded?: boolean
}

export type DocumentCreateResponse = {
  id: string
  base: string[]
}

export type DocumentBeginEditResponse = {
  session: string
  version: string
}

export type DocumentEndEditResponse = {
  changeset: string
}

export type DocumentBeginMetadata = {
  at: string
  id: string
  version: string
}

export type ViewType = "table" | "feed"

export type SelectButtonOption<T> = {
  name: string
  value: T
  icon?: {
    component: string | Component
    alt: string
  }
  disabled?: boolean
  progress?: number
}

export type GetDisplayLabel = (
  claims: DeepReadonly<ClaimTypes> | null | undefined,
  router: Router,
  i18n: Composer,
  el: Ref<Element | null> | null,
  abortSignal: AbortSignal,
  progress: Ref<number> | null,
) => Promise<string | null>

// It is recursive.
export type Mutable<T> = {
  -readonly [k in keyof T]: Mutable<T[k]>
}

// It is not recursive.
type Required<T> = {
  [k in keyof T]-?: T[k]
}

// It is not recursive.
type Optional<T> = {
  [k in keyof T]+?: T[k]
}

export type Constructor<T> = new (json: object) => T
export type Constructee<C> = C extends Constructor<infer R> ? R : never

export type DownloadFile = {
  name: string
  url: string
}

export type DownloadingPhase = "picking" | "preparing" | "downloading" | "empty"

export type DownloadZipWorkerInput =
  // When fileHandle is non-null, the worker streams the zip directly to this handle. When null,
  // the worker assembles a Blob and posts it back to the main thread for the <a download> fallback.
  | { type: "start"; files: DownloadFile[]; fileHandle: FileSystemFileHandle | null }
  // Asks the worker to abort cleanly: cancel pending I/O, abort the writable so the swap file is
  // cleaned up, then post a final "done" so the main thread can close out the run.
  | { type: "cancel" }

export type DownloadZipWorkerOutput =
  | { type: "progress"; completed: number; total: number; currentFile: string }
  | { type: "blob"; blob: Blob }
  | { type: "done" }
  | { type: "error"; message: string }

export type DownloadFilesWorkerInput = { type: "start"; files: DownloadFile[]; directoryHandle: FileSystemDirectoryHandle } | { type: "cancel" }

export type DownloadFilesWorkerOutput =
  | { type: "progress"; completed: number; total: number; currentFile: string }
  | { type: "done" }
  | { type: "error"; message: string }
