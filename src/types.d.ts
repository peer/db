import type { BareItem, Key } from "structured-field-values"
import type { Component, DeepReadonly, Ref } from "vue"
import type { Composer } from "vue-i18n"
import type { Router } from "vue-router"

import type { ClaimTypes } from "@/document/claims"

export type RefSearchResult = {
  props: readonly string[]
  count: number
  type: "ref"
  filterId?: string
}

export type AmountSearchResult = {
  props: readonly string[]
  count: number
  type: "amount"
  unit?: string
  filterId?: string
}

export type TimeSearchResult = {
  props: readonly string[]
  count: number
  type: "time"
  filterId?: string
}

export type HasSearchResult = {
  props?: readonly string[]
  count: number
  type: "has"
  filterId?: string
}

export type FilterResult = RefSearchResult | AmountSearchResult | TimeSearchResult | HasSearchResult

export type Result = {
  id: string
}

export type RefFilterResult = {
  id: string
  count: number
  paths?: string[][]
}

export type RefFilterTreeNode = {
  res: RefFilterResult
  // res.id for the canonical placement; res.id + "|" + ancestorKey for diamond duplicates.
  key: string
  children: RefFilterTreeNode[]
}

export type HasFilterResult = {
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

export type ToValue = {
  id: string
}

export type HasValue = {
  id: string
}

export type RefFilter = {
  to?: ToValue[]
  missing?: boolean
}

export type AmountFilter = {
  unit?: string
  gte?: number
  lte?: number
  missing?: boolean
}

export type TimeFilter = {
  gte?: number
  lte?: number
  missing?: boolean
}

export type HasFilter = {
  props?: HasValue[]
}

export type FilterBase = {
  // On frontend, ID and base are always set, except when we send payload to the SearchShortcut API
  // endpoint (e.g., in CreateDropdown.vue) where we use payload without them (and without this type).
  id: string
  base: string[]
  prop: string[]
}

export type RefFilterEntry = FilterBase & { ref: RefFilter }
export type AmountFilterEntry = FilterBase & { amount: AmountFilter }
export type TimeFilterEntry = FilterBase & { time: TimeFilter }
export type HasFilterEntry = FilterBase & { has: HasFilter }

export type Filter = RefFilterEntry | AmountFilterEntry | TimeFilterEntry | HasFilterEntry

export type SearchSession = {
  id: string
  base: string[]
  version: number
  // View is always set by the backend.
  view: ViewType
} & SearchSessionData

// What the client sends when creating or updating a search session.
// When reverse is set, the session is scoped to documents referencing that ID via any property.
export type SearchSessionData = {
  view?: ViewType
  query?: string
  filters?: Filter[]
  reverse?: string
}

// Response from creating a new search session.
export type CreateSearchSessionResponse = {
  id: string
  base: string[]
  version: number
}

// Response from updating an existing search session.
export type UpdateSearchSessionResponse = {
  version: number
}

// Client-side reference to a search session for reactive tracking.
export type SearchSessionRef = {
  id: string
  version: number
}

// UserInfo carries the profile fields the backend exposes for the
// currently signed-in user. Optional fields are present only when the
// upstream userinfo lookup succeeded; subject is always set.
export type UserInfo = {
  subject: string
  username?: string
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
    downloadButtons?: boolean
  }
  roles?: {
    [roleName: string]: string[]
  }
  metadataHeaderPrefix?: string
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

// DocumentEditStatus is the response shape of GET /d/edit/:id/:session (DocumentEdit API).
// For active sessions, base is always set; version is absent for create sessions and
// present for edit sessions.
export type DocumentEditStatus = {
  active: boolean
  base?: string[]
  version?: string
  changeset?: string
  discarded?: boolean
}

export type DocumentCreateResponse = {
  id: string
  base: string[]
  session: string
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
  base: string[]
  // Absent for create sessions (no parent version yet).
  version?: string
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

// A single validation failure. Codes (not messages) keep i18n in the
// presentation layer. Path is a hierarchical address into a composite input
// (e.g. ["from"] for the lower bound of an interval input). The optional el
// is the focus target for this specific error (used by composite inputs to
// point at a particular sub-element); when absent the input's own el getter
// is used as the fallback.
export type ValidationError = {
  path?: string[]
  code: string
  el?: HTMLElement
  // It should be localized.
  userMessage?: string

  // Optional debug info.
  debugMessage?: string
  debugError?: Error
}

// Triggers (re-)validation of an input. Implementations write the result
// into the input's reactive errors field. Callers read it from there after
// the promise resolves. The optional signal lets callers abort in-flight
// async validation.
export type ValidateFn = (signal?: AbortSignal) => Promise<void>

// A user-supplied rule plugged into an input via its :validator prop. It
// receives the value directly (instead of reading it off the input's
// model) and returns the resulting errors. The input's useValidation
// wrapper writes them into the input's reactive errors field.
//
// options.eager is true on re-validation triggered by the model watcher
// (e.g. when the validator is being called during typing) and false on
// validateAll (and underlying validate), but can be overridden when calling
// runValidation directly. Validators with side effects on the model (e.g.
// trimming whitespace) should gate those effects on !options.eager so the
// user is not fighting the input while typing.
//
// options.initial is true on the very first validator invocation (triggered
// by the immediate model watcher on mount) and false on every subsequent
// call. On initial, validators should report structural errors (e.g. URL
// parse failure) so a pre-populated invalid value is surfaced immediately,
// but should skip the required check (the user has not interacted yet) and
// skip any model-mutating side effects.
export type ValidatorFn<T> = (value: T, options: { signal: AbortSignal; eager: boolean; initial: boolean }) => Promise<ValidationError[]>

// What an input registers with a parent so the parent can validate it and
// resolve focus targets. el returns the input's default focus target, used
// by useValidationRegistry to decorate errors that lack their own el before
// they are returned to the caller (so the resulting ValidationError[] is
// self-contained for focus resolution). reset restores the input to its
// initial (empty/default) state; revert restores the input to its recorded
// checkpoint. useValidationRegistry exposes resetAll / revertAll.
export type ValidatedInput = {
  validate: ValidateFn
  reset: () => void
  // Restores the input to whatever checkpoint last captured, leaving
  // isDirty false. Used by the per-field "changed" badge so the user can
  // undo their changes without affecting other fields.
  revert: () => void
  el: () => HTMLElement | null
  // Reactive flag: true when the input's current value differs from its
  // recorded checkpoint.
  isDirty: Readonly<Ref<boolean>>
  // Reactive flag: true when the input holds no meaningful value (e.g.
  // empty string for text inputs, unchecked for checkboxes).
  isEmpty: Readonly<Ref<boolean>>
  // The input's current errors.
  errors: Readonly<Ref<ValidationError[]>>
  // Snapshots the input's current value as the checkpoint against which
  // isDirty is compared. Called when an input's controls are shown or
  // when they are reset so subsequent edits show up as dirty.
  checkpoint: () => void
}
