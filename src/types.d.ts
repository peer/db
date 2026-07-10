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
  // childCount is the value's exact number of distinct child values across the whole hierarchy (robust to
  // multiple inheritance), as computed by the backend. It is compared against how many of the value's children
  // are actually loaded to detect children truncated by the server's value cap.
  childCount: number
  paths?: string[][]
}

// TreeNode is one node in a value-hierarchy tree built from a flat, path-carrying result list (see
// buildRefTree). res is the underlying result, key uniquely identifies this placement (res.id for the
// canonical placement; res.id + "|" + ancestorKey for diamond duplicates under additional parents), and
// children are the nodes placed under it.
export type TreeNode<T> = {
  res: T
  key: string
  children: TreeNode<T>[]
}

export type RefFilterTreeNode = TreeNode<RefFilterResult>

// RefValueLike is the minimal shape of a reference filter value the selection logic needs: the
// value id and its hierarchy paths. Each path is an ancestor chain from a root to the value's
// immediate parent; a "direct" entry's path ends with its own value, and the top-level "missing"
// entry has no paths. RefFilterResult satisfies this.
export type RefValueLike = { id: string; paths?: string[][] }

// RefValueWithCounts extends the minimal value shape with the document count and the exact number of
// distinct child values (childCount) the all-children promotion gate needs. RefFilterResult satisfies it.
// When count/childCount are absent (older callers and tests), a value is treated as promotable so the
// prior promotion behavior is preserved.
export type RefValueWithCounts = RefValueLike & { count?: number; childCount?: number }

// RefCheckState is the tri-state a reference filter value renders as.
export type RefCheckState = { checked: boolean; indeterminate: boolean }

// RefFilterValueToken is one rendered entry of a reference filter's selection: a selected value (its id,
// with direct marking the "most-specific only" variant the facet tree labels "direct"), or the synthetic
// missing entry. A flat display iterates these to list the whole selection uniformly.
export type RefFilterValueToken = { kind: "value"; id: string; direct: boolean } | { kind: "missing" }

// ClassCreateResult is one class returned by the DocumentCreateOptions endpoint. paths are the SUBCLASS_OF
// ancestor chains (root to immediate parent), one per parent, so the class renders once under each parent.
// canCreate is true when a document can be created for the class; non-creatable classes appear only as
// structural ancestors of creatable ones.
export type ClassCreateResult = {
  id: string
  paths?: string[][]
  canCreate: boolean
}

export type ClassCreateTreeNode = TreeNode<ClassCreateResult>

// CreateOptionsResponse is the body of the DocumentCreateOptions endpoint: the classes for the create
// view, already ordered for tree rendering. It is an object so it can grow more fields later.
export type CreateOptionsResponse = {
  classes: ClassCreateResult[]
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

// A numeric or temporal filter selection: a range with both bounds set, the documents missing the property (missing), the documents
// having any value (exists), or an empty selection. The variants are mutually exclusive, so a range always carries both bounds and an
// empty selection (the payload that removes the filter from the session) carries none.
type RangeSelection = { gte: number; lte: number; missing?: never; exists?: never }
type MissingSelection = { missing: true; gte?: never; lte?: never; exists?: never }
type ExistsSelection = { exists: true; gte?: never; lte?: never; missing?: never }
type EmptySelection = { gte?: never; lte?: never; missing?: never; exists?: never }
type RangeFilterSelection = RangeSelection | MissingSelection | ExistsSelection | EmptySelection

export type AmountFilter = { unit?: string } & RangeFilterSelection

export type TimeFilter = RangeFilterSelection

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
  ids?: string[]
  filters?: { prop: string[]; ref: { to?: { id: string }[]; direct?: { id: string }[]; missing?: boolean } }[]
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
// When ids is non-empty, the session is scoped to documents whose own ID is one of the listed values.
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
  ids?: string[]
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
  // Maps a minimum viewport width (a CSS length, e.g. "0", "48rem") to the logo path used from that
  // width up; the largest matching entry wins, the smallest is the fallback, and the largest is also
  // used as the full logo (e.g. the home page hero). See logoVariants in @/context.
  logo?: {
    [minWidth: string]: string
  }
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
    // The navbar positioning mode: "fixed" keeps the navbar at the viewport top, "static" leaves
    // it in the document flow at the page top, and unset means the default auto-hide behavior.
    navbarPosition?: "fixed" | "static"
    disableSearchSort?: boolean
    disablePrintView?: boolean
    // Hides the session's prefilters from the UI (the "results limited to" notice in the filters
    // sidebar and the prefilter entries in the print layout).
    hidePrefilters?: boolean
    // Hides the built-in edit and delete buttons from the document view (they render at the top of the side
    // links), for sites that render these actions inside the page themselves (through the document actions
    // provided to document components).
    hideDocumentActions?: boolean
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

// LastOperationResponse is the response shape of the APIs which return the sequence number
// of the latest operation in a coordinator session. lastOperation is 0 when there are none.
// Operations are numbered sequentially without gaps starting at 1, so the session's operations
// are exactly 1 through lastOperation.
export type LastOperationResponse = {
  lastOperation: number
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
// options.final marks the run as a final pass (see ValidatorFn's options.final); a
// form-wide submit (Save) passes it, everything else (blur-driven composite
// validations, re-validation watchers) leaves it unset.
export type ValidateFn = (signal?: AbortSignal, options?: { final?: boolean }) => Promise<void>

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
//
// options.final is true when the validate cascade was invoked with final set
// (the form-wide Save pass does that) and false on every other run (blur,
// typing, mount, blur-driven composite validations). Validators gate checks
// on it when a problem should block a submit but not nag during editing
// (e.g. InputRef's "unfinished" check for typed text without a selection).
export type ValidatorFn<T> = (value: T, options: { signal: AbortSignal; eager: boolean; initial: boolean; final: boolean }) => Promise<ValidationError[]>

// What an input registers with a parent so the parent can validate it and
// resolve focus targets. inputEl returns the focusable control to move
// keyboard focus to; it is also used by useValidationRegistry to decorate
// errors that lack their own el before they are returned to the caller (so
// the resulting ValidationError[] is self-contained for focus resolution).
// mainEl returns the wrapper element that owns the input, used for identity
// and containment checks. For a leaf input the two are the same element; for
// a composite input (e.g. ClaimCardinality, ClaimInput) inputEl descends to
// the first focusable control among its children while mainEl is the
// container spanning all of them (ClaimCardinality tests mainEl against
// document.activeElement to decide whether the slot the user is in may be
// shrunk). reset restores the input to its initial (empty/default) state;
// revert restores the input to its recorded checkpoint. useValidationRegistry
// exposes resetAll / revertAll.
// One top-level grid column an input renders, declared via ValidatedInput.columns.
export type InputColumn = {
  // Translated plain-text label; "" for a column with no visible label.
  label: string
  // The focusable control in this column. May return null before it mounts.
  el: () => HTMLElement | null
  // Optional CSS width for the column. The first column grows to fill the
  // available width and this caps it (the enclosing grid uses minmax(0,width)
  // instead of minmax(0,1fr)): an input whose values are inherently short
  // (amounts, times) does not stretch absurdly wide. Later columns use the
  // value VERBATIM as the track size (a fixed width, or a minmax()/min()
  // expression) instead of the content-sized auto. Declaring it makes every
  // track deterministic, so a repeated field's hoisted label row (a separate
  // grid from the entries, see ClaimCardinality) resolves to exactly the
  // entries' tracks at every container width; a content-sized track would
  // resolve differently for a text label than for a control.
  width?: string
}

export type ValidatedInput = {
  validate: ValidateFn
  reset: () => void
  // Restores the input to whatever checkpoint last captured, leaving
  // isDirty false. Used by the per-field "changed" badge so the user can
  // undo their changes without affecting other fields.
  revert: () => void
  // The focusable control to move keyboard focus to.
  inputEl: () => HTMLElement | null
  // The wrapper element that owns this input. Same element as inputEl for a
  // leaf input; the container spanning all children for a composite input.
  mainEl: () => HTMLElement | null
  // Reactive flag: true when the input's current value differs from its
  // recorded checkpoint.
  isDirty: Readonly<Ref<boolean>>
  // Reactive flag: true when the input holds no meaningful value (e.g.
  // empty string for text inputs, unchecked for checkboxes).
  isEmpty: Readonly<Ref<boolean>>
  // The input's current errors.
  errors: Readonly<Ref<ValidationError[]>>
  // One entry per top-level grid column the input renders, each carrying its
  // label and focusable el. Absent means a single unlabeled column. The number
  // of entries signals how many columns the input wants.
  columns?: Readonly<Ref<InputColumn[]>>
  // Translated hint lines (e.g. an input-format example). Empty
  // or absent means no hints.
  hints?: Readonly<Ref<string[]>>
  // Snapshots the input's current value as the checkpoint against which
  // isDirty is compared. Called when an input's controls are shown or
  // when they are reset so subsequent edits show up as dirty.
  checkpoint: () => void
}

// SaveChangeSpec describes a claim change to commit to the edit session. The queue in
// DocumentEdit assigns the change number at post time and renumbers on conflicts with
// concurrent editors. An add's claim id derives from the change number, so it is known
// only once the change is committed and is returned in SaveChangeResult.
export type SaveChangeSpec =
  | { type: "add"; patch: object; under?: string }
  | { type: "set"; id: string; patch: object }
  | { type: "cast"; id: string; patch: object }
  | { type: "remove"; id: string }

// SaveChangeResult reports a committed change. For an add, id is the claim's final id
// (derived from the final change number); for other change types it echoes the target id.
export type SaveChangeResult = {
  id: string
}

// FieldsFormFlush is registered by every slot input so DocumentEdit can flush pending
// local edits before Save and warn before tab close while local edits are not yet
// committed to the server.
export type FieldsFormFlush = {
  // flush commits the slot's current local edit, like its blur would.
  flush: () => Promise<void>
  // hasUncommitted reports whether the slot holds local edits which have not been
  // committed to its claim (typed but not yet blurred).
  hasUncommitted: () => boolean
}

// ListFormatPart is one piece of a locale-formatted list: a literal separator to print verbatim, or a
// reference (by index) to the element the caller renders at that position.
export type ListFormatPart = { type: "literal"; value: string } | { type: "element"; index: number }

// ParseUrlOptions are the options accepted by parseUrl (and forwarded by normalizeUrl).
export type ParseUrlOptions = {
  // Defaults to true.
  allowContact?: boolean
}
