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

// A search result. When results are grouped, a node with group set is a group heading: id is the
// referenced value's document ID, count is the number of documents in the group, and group holds the
// nested sub-groups or the documents. A node without group is a plain result document (a leaf).
// A group heading whose id is "__MISSING__" is the synthetic "missing" group: it holds the documents
// that are missing this level's grouping property (same sentinel the reference filter uses).
export type Result = {
  id: string
  count?: number
  group?: Result[]
}

// SortColumn identifies a sortable column: a built-in column ("score", "time", "label") with no prop, or
// a filter column ("ref", "amount", "time") carrying its property path (and unit for amount). A built-in
// "time" column is distinguished from a "time" filter column by the absence of prop.
export type SortColumn = {
  type: "score" | "time" | "label" | "ref" | "amount"
  prop?: string[]
  unit?: string
}

// SortKey is one column in the effective sort order. descending sorts high-to-low. group (ref columns
// only, in a leading run) groups results by that column's value. expand (grouped columns only) renders
// each group value as a full result card instead of a one-line heading.
export type SortKey = SortColumn & {
  descending?: boolean
  group?: boolean
  expand?: boolean
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
  direct?: ToValue[]
  missing?: boolean
}

export type AmountFilter = {
  unit?: string
  gte?: number
  lte?: number
  missing?: boolean
  exists?: boolean
}

export type TimeFilter = {
  gte?: number
  lte?: number
  missing?: boolean
  exists?: boolean
}

export type HasFilter = {
  props?: HasValue[]
}

export type FilterBase = {
  // On frontend, ID and base are always set, except when we send payload to the SearchJustResults API
  // endpoint (e.g., in DocumentCreate.vue) where we use payload without them (and without this type).
  id: string
  base: string[]
  prop: string[]
}

export type RefFilterEntry = FilterBase & { ref: RefFilter }
export type AmountFilterEntry = FilterBase & { amount: AmountFilter }
export type TimeFilterEntry = FilterBase & { time: TimeFilter }
export type HasFilterEntry = FilterBase & { has: HasFilter }

export type Filter = RefFilterEntry | AmountFilterEntry | TimeFilterEntry | HasFilterEntry

// A single parsed key/value pair from a search shortcut string. Nested keys
// keep the raw "parent:prop" form; callers resolve each side individually.
export type ShortcutPair = { key: string; value: string }

// Payload shape for the SearchJustResults POST endpoint built.
export type JustResultsFilters = {
  reverse?: string
  filters?: { prop: string[]; ref: { to: { id: string }[] } }[]
}

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
  // prefilters constrain results like filters but do not contribute to ranking. They are populated by search shortcuts.
  prefilters?: Filter[]
  reverse?: string
  // reverseExpand, valid only when reverse is set, is presentational: in the print view it renders the
  // referenced target as its full result card instead of a one-line "results referencing" heading.
  reverseExpand?: boolean
  // language is the session's UI language. The backend resolves an empty value to the site default and stores it on the session.
  language?: string
  // sort is the effective sort order. Empty means the default order (relevance, time, display label). A
  // leading run of group=true ref columns groups the feed results.
  sort?: SortKey[]
}

// Request body for creating a new search session. The optional query sets the initial full-text query.
export type CreateSearchSessionRequest = {
  query?: string
  // language sets the session's UI language. The backend resolves an empty value to the site default.
  language?: string
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

// Request body for creating a search session from the search shortcut.
// query is a URL query string.
export type SearchShortcutRequest = {
  query: string
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
  logoCompact?: string
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

export type StorageEndUploadRequest = {
  // Lowercase hex SHA-256 of the file contents, computed by the client while uploading. The upload
  // fails if the assembled file does not hash to it.
  hash: string
}

export type StorageUploadStatus = {
  active: boolean
  id?: string
  discarded?: boolean
  errored?: boolean
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

// A user who contributed to a document version. id is the auth subject string.
export type HistoryUser = {
  id: string
}

// One entry in a document's changeset history, as returned by the DocumentHistory API.
// at is an RFC3339 timestamp and version is the "changeset-revision" string used to link
// to the document at that revision.
export type DocumentHistoryItem = {
  changeset: string
  version: string
  at: string
  authors?: HistoryUser[]
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
