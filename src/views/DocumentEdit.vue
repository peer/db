<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"
import type { ComponentExposed } from "vue-component-type-helpers"

import type { Claim, ClaimTypes, TimePrecision } from "@/document"
import type {
  DocumentEditStatus,
  DocumentEndEditResponse,
  FieldsFormFlush,
  LastOperationResponse,
  SaveChangeResult,
  SaveChangeSpec,
  ValidatedInput,
  ValidateFn,
} from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { computed, nextTick, onBeforeUnmount, provide, readonly, ref, toRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, FetchError, getURL, getURLDirect, postJSON } from "@/api"
import { CAN_EDIT_DOCUMENT, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import { INSTANCE_OF, PROPERTY } from "@/core"
import {
  AmountClaim,
  AmountIntervalClaim,
  D,
  HasClaim,
  HighConfidence,
  HTMLClaim,
  IdentifierClaim,
  LinkClaim,
  NoneClaim,
  ReferenceClaim,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UnknownClaim,
} from "@/document"
import { changeFrom } from "@/document/patch"
import {
  ChangeDroppedError,
  getCommittedClaimKey,
  registerForFlushKey,
  registerRemoteAddsKey,
  registerRemoteConflictKey,
  saveChangeKey,
  unregisterForFlushKey,
  unregisterRemoteAddsKey,
  unregisterRemoteConflictKey,
} from "@/fields"
import { classifyLink, LINK_CLASS_FILE } from "@/internal-links"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import DocumentDuplicates from "@/partials/DocumentDuplicates.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import FieldsForm from "@/partials/FieldsForm.vue"
import Footer from "@/partials/Footer.vue"
import InputAmount from "@/partials/input/InputAmount.vue"
import InputFile from "@/partials/input/InputFile.vue"
import InputHTML from "@/partials/input/InputHTML.vue"
import InputIdentifier from "@/partials/input/InputIdentifier.vue"
import InputLink from "@/partials/input/InputLink.vue"
import InputRef from "@/partials/input/InputRef.vue"
import InputString from "@/partials/input/InputString.vue"
import InputTime from "@/partials/input/InputTime.vue"
import InputField from "@/partials/InputField.vue"
import InputMissing from "@/partials/InputMissing.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { localCounter, pairCounters, useLock, useProgress } from "@/progress"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { delay, encodeQuery, equals } from "@/utils"
import { focusFirstInput, focusFirstInvalid, useValidationRegistry } from "@/validation"
import { Identifier } from "@tozd/identifier"

const props = defineProps<{
  id: string
  session: string
}>()

type ClaimType = "id" | "string" | "html" | "amount" | "amountInterval" | "time" | "timeInterval" | "link" | "file" | "ref" | "has" | "none" | "unknown"
const claimTypes: ClaimType[] = ["id", "string", "html", "amount", "amountInterval", "time", "timeInterval", "link", "file", "ref", "has", "none", "unknown"]

// Restricts the property-picker InputRef to documents that are instances of PROPERTY.
const PROPERTY_FILTER = `${INSTANCE_OF}=${PROPERTY}`

const claimType = ref<ClaimType>("id")
const claimProp = ref("")
const claimValue = ref("")
const claimAmountPrecision = ref("")
const claimTimePrecision = ref<TimePrecision>("y")
const claimFrom = ref("")
const claimFromAmountPrecision = ref("")
const claimFromTimePrecision = ref<TimePrecision>("y")
const claimFromUnknown = ref(false)
const claimFromNone = ref(false)
const claimTo = ref("")
const claimToAmountPrecision = ref("")
const claimToTimePrecision = ref<TimePrecision>("y")
const claimToUnknown = ref(false)
const claimToNone = ref(false)

const claimFormError = ref("")
const sessionError = ref("")
// Null in add mode; the claim's ID in edit mode. Drives the form title,
// the primary button label, and the onSubmit branch (set vs add).
const editingClaimId = ref<string | null>(null)
// Null when no parent is selected; otherwise the claim's ID under which
// the new claim will be added. Mutually exclusive with editingClaimId.
const subClaimParentId = ref<string | null>(null)
// Locks the type tabs to a single type while editing. Decoupled from
// editingClaimId so onEditClaim can briefly unlock the tabs (without
// flipping the title back to "Add value") during the transition between
// two edits of different types - see onEditClaim for the rationale.
const lockedClaimType = ref<ClaimType | null>(null)
// Controlled selected-index for the claim type TabGroup so onEditClaim
// can switch to the tab matching the edited claim's type.
const selectedClaimTab = computed(() => {
  const idx = claimTypes.indexOf(claimType.value)
  return idx >= 0 ? idx : 0
})

// A tab is locked-out when an edit is in progress and its type does not
// match the type being edited.
function claimTypeDisabled(type: ClaimType): boolean {
  return lockedClaimType.value !== null && lockedClaimType.value !== type
}

function claimTypeLabel(type: ClaimType): string {
  switch (type) {
    case "id":
      return t("views.DocumentEdit.claimTypes.identifier")
    case "string":
      return t("views.DocumentEdit.claimTypes.string")
    case "html":
      return t("views.DocumentEdit.claimTypes.html")
    case "amount":
      return t("views.DocumentEdit.claimTypes.amount")
    case "amountInterval":
      return t("views.DocumentEdit.claimTypes.amountInterval")
    case "time":
      return t("views.DocumentEdit.claimTypes.time")
    case "timeInterval":
      return t("views.DocumentEdit.claimTypes.timeInterval")
    case "link":
      return t("views.DocumentEdit.claimTypes.link")
    case "file":
      return t("views.DocumentEdit.claimTypes.file")
    case "ref":
      return t("views.DocumentEdit.claimTypes.reference")
    case "has":
      return t("views.DocumentEdit.claimTypes.has")
    case "none":
      return t("views.DocumentEdit.claimTypes.none")
    case "unknown":
      return t("views.DocumentEdit.claimTypes.unknown")
    default:
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`unsupported claim type: ${type}`)
  }
}

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// We use separate lock for data modification and controls.
const lock = useLock()
// And used together with progress for data loading.
const busy = pairCounters(useProgress(), lock)
// saveBusy is the writable handle for the Save buttons: a local count
// drives the :progress visual (so it lights only during save, not during
// initial data load which also writes to lock via busy), and writes
// propagate into lock for descendant cascade.
const saveBusy = localCounter(lock)

const el = useTemplateRef<HTMLElement>("el")
const displayLabelComponent = useTemplateRef<ComponentExposed<typeof DisplayLabel>>("displayLabelComponent")
const claimFormRef = useTemplateRef<HTMLFormElement>("claimFormRef")
// Explicit handle type for the FieldsForm ref. ComponentExposed<typeof FieldsForm>
// loses the parameter types of its exposed methods, which trips the unsafe-call
// lint rule. Spelling the shape out matches FieldsForm's defineExpose.
const fieldsFormRef = useTemplateRef<{
  validateAll: ValidateFn
  inputs: ReadonlySet<ValidatedInput>
  // anyDirty is exposed by FieldsForm as a ref but defineExpose's proxy
  // unwraps refs on access from the parent's template ref - so we read it
  // as a plain boolean here. Used by canSave to also enable Save when the
  // only pending changes are deferred deletes (emptied existing-claim rows
  // that have not produced a saveChange yet).
  anyDirty: boolean
}>("fieldsFormRef")

const { resetAll, firstInputEl, allEmpty, anyDirty, anyError, inputs, validateAll, checkpointAll } = useValidationRegistry(() => {
  // Any registered-input interaction clears stale form-level errors so the
  // user is not staring at an error message after they have moved on.
  sessionError.value = ""
  claimFormError.value = ""
})

let abortController = new AbortController()

// Armed on every (re)load; cleared once initial focus has moved into the
// FieldsForm. One-shot so later cardinality grow/shrink or tab switches do
// not steal focus back.
let pendingInitialFocus = false

function cleanup() {
  abortController.abort()
}

onBeforeUnmount(() => {
  cleanup()
})

const _doc = ref<D | null>(null)
const doc = process.env.NODE_ENV !== "production" ? readonly(_doc) : _doc
// Pristine snapshot of the doc as it was BEFORE any session changes
// were applied (i.e. the doc fetched at beginMetadata.version, untouched).
// FieldsForm uses this as the baseline against which the per-property
// "changed" badge and the Revert action diff: a reload mid-session must
// still see existing committed-in-session changes as "changed" relative
// to the session's starting point, not relative to the freshly-reloaded
// (already-changes-applied) doc.
const _initialDoc = ref<D | null>(null)
const initialDoc = process.env.NODE_ENV !== "production" ? readonly(_initialDoc) : _initialDoc

// True for create sessions (document not yet materialized), false for
// edit sessions on an existing document. Drives the primary button label
// (Create vs. Update). Null until loadAndSubscribe has fetched the
// session's edit status.
const isCreating = ref<boolean | null>(null)

// Potential-duplicates panel, only mounted for create sessions in field form mode.
const duplicatesRef = useTemplateRef<{ refresh: () => Promise<void> }>("duplicatesRef")

// Debounce the duplicate search so it runs once a field's blur has committed into the doc (a
// blur fires a saveChange that the subscription applies into doc.claims shortly after), and so rapid
// tabbing between fields does not fire a search per field.
let duplicatesTimer: ReturnType<typeof setTimeout> | null = null
function onFieldsBlur() {
  if (!isCreating.value) {
    return
  }
  if (duplicatesTimer !== null) {
    clearTimeout(duplicatesTimer)
  }
  duplicatesTimer = setTimeout(() => {
    duplicatesTimer = null
    duplicatesRef.value?.refresh().catch((err: unknown) => {
      console.error("DocumentEdit.onFieldsBlur", err)
    })
  }, 400)
}

onBeforeUnmount(() => {
  if (duplicatesTimer !== null) {
    clearTimeout(duplicatesTimer)
  }
})

// Tracks the change number which was committed in the backend.
// A ref so canSave reactively follows whether anything has been added to
// the session yet (Save is disabled when the session has no changes).
const committedChange = ref(0)
// Highest change number known to exist on the server for this session: our own successful
// posts and anything observed in the changes list. The next change is posted at this + 1.
let lastServerChange = 0
// Change numbers this client successfully posted. The subscription uses it to tell our own
// applied changes apart from remote ones (remote ones drive conflict handling).
const ownChangeNumbers = new Set<number>()
// Per claim id, the number of our last committed change targeting it. A remote touch of a
// claim is not notified while we have a newer committed change the subscription has not applied
// yet - resyncing from the doc at that moment would regress the slot to the older remote
// state, and no later notification would correct it (our own changes are not "remote").
const ownClaimChanges = new Map<string, number>()
// Number of changes queued or in flight. Drives the pre-endEdit drain on Save and the
// warning when the tab is closed with unsaved data.
const pendingChangeCount = ref(0)

const fieldsFormInvalid = ref(false)

// Flush registry: all slot inputs register here so we can flush them before save and know
// whether uncommitted local edits exist when the tab is being closed.
const flushRegistry = new Set<FieldsFormFlush>()

// Handlers notified with the set of claim ids touched by remote changes the subscription
// applied (including ancestors of every touched claim). Conflict handlers (slots resyncing
// their claims) run first; add handlers (cardinalities adding slots for remotely added claims)
// run after the render flush - see loadChanges.
const remoteConflictHandlers = new Set<(claimIds: ReadonlySet<string>) => void>()
const remoteAddHandlers = new Set<(claimIds: ReadonlySet<string>) => boolean>()

// How long to wait before retrying a change POST after a transient failure.
const saveRetryInterval = 1000 // In milliseconds.

// materializeChange builds the raw change object for a spec at the given change number. An
// add's base and id derive from the number, so they are (re)computed here for every attempt.
async function materializeChange(spec: SaveChangeSpec, changeNumber: number): Promise<{ change: object; id: string }> {
  if (spec.type === "add") {
    const changeBase = [...doc.value!.base, "SESSION", props.session, String(changeNumber)]
    const id = (await Identifier.from(...changeBase)).toString()
    const change: { type: string; id: string; base: string[]; patch: object; under?: string } = { type: "add", id, base: changeBase, patch: spec.patch }
    if (spec.under !== undefined) {
      change.under = spec.under
    }
    return { change, id }
  }
  if (spec.type === "remove") {
    return { change: { type: "remove", id: spec.id }, id: spec.id }
  }
  return { change: { type: spec.type, id: spec.id, patch: spec.patch }, id: spec.id }
}

// changeApplies runs the same validation the backend runs on append: the change has to
// be valid on its own at the given change number (Change.Validate, which for an add also
// binds its base and id to the session and the number) and has to test-apply cleanly to
// a clone of the doc (its target claim exists, a cast still changes the claim type, an
// add's parent claim exists, and so on). The caller is responsible for the doc being at
// the state after the change's preceding change (see postChange).
async function changeApplies(change: object, changeNumber: number): Promise<boolean> {
  if (!_doc.value) {
    return false
  }
  const changesetBase = [...doc.value!.base, "SESSION", props.session]
  const target = _doc.value.Clone()
  try {
    const c = changeFrom(change)
    await c.Validate(changesetBase, changeNumber)
    await c.Apply(target)
    return true
  } catch {
    return false
  }
}

// Serializes every application of committed changes into the live doc (the poll's
// loadChanges and postChange's own-change application), so no change is ever applied to
// the doc twice.
let docApplyChain: Promise<unknown> = Promise.resolve()
function runDocApplySerialized<T>(fn: () => Promise<T>): Promise<T> {
  const run = docApplyChain.then(fn)
  // Keep the chain alive even if this task fails (a then on a rejected promise skips its
  // callback, so every later task would be skipped forever); the caller still observes
  // the failure through the returned promise.
  docApplyChain = run.catch(() => undefined)
  return run
}

// applyOwnChange applies a change this client just committed into the live doc, so the
// doc reaches the state after the change without refetching it. Skipped when the doc is
// not exactly at the state before the change (then the poll path applies it instead).
async function applyOwnChange(changeNumber: number, change: object): Promise<void> {
  await runDocApplySerialized(async () => {
    if (abortController.signal.aborted) {
      return
    }
    if (!_doc.value || committedChange.value !== changeNumber - 1) {
      return
    }
    try {
      await changeFrom(change).Apply(_doc.value)
      committedChange.value = changeNumber
    } catch (error) {
      // The change was validated against this exact state before it was posted, so this
      // is unreachable in practice. The doc stays at the previous change; the poll
      // refetches and applies from there.
      console.error("DocumentEdit.applyOwnChange", error)
    }
  })
}

// postChange posts a change spec, assigning the change number at post time and retrying.
//
// Every attempt is fully validated first, mirroring the backend's apply-on-append
// validation: the doc is brought to the state after the preceding change (the state the
// backend validates against) and the change has to apply cleanly to it, else it is
// dropped with ChangeDroppedError. Then:
//   - On a conflict (another editor claimed the number): if the stored operation matches
//     what we posted, an earlier attempt of ours reached the server despite a network
//     error and the change is committed. Otherwise the change is renumbered and the loop
//     revalidates it against the synced doc, dropping it when it no longer applies
//     (e.g. its claim was removed concurrently).
//   - On an invalid change (server-side validation failed): dropped with
//     ChangeDroppedError. Client-side validation above mirrors the backend, so this is a
//     safety net rather than an expected path.
//   - On transient failures (network or server errors): retried at the same number after
//     a pause. If the failed POST actually reached the server, the retry conflicts with
//     it and the comparison above resolves it as committed.
async function postChange(spec: SaveChangeSpec): Promise<SaveChangeResult> {
  while (true) {
    abortController.signal.throwIfAborted()
    // Bring the doc to the state after the last known committed change. In the common
    // case the doc is already there: our own posts apply through applyOwnChange, and the
    // poll keeps committedChange at lastServerChange otherwise.
    if (committedChange.value < lastServerChange) {
      try {
        await syncChanges()
      } catch (err) {
        abortController.signal.throwIfAborted()
        console.error("DocumentEdit.postChange sync", err)
        await delay(saveRetryInterval, abortController.signal)
        continue
      }
      abortController.signal.throwIfAborted()
    }
    const changeNumber = lastServerChange + 1
    const { change, id } = await materializeChange(spec, changeNumber)
    if (!(await changeApplies(change, changeNumber))) {
      // TODO: Implement better conflict handling instead of just dropping it.
      throw new ChangeDroppedError(`change does not apply: ${JSON.stringify(change)}`)
    }
    try {
      await postJSON(
        router.apiResolve({
          name: "DocumentSaveChange",
          params: { session: props.session },
          query: encodeQuery({ change: String(changeNumber) }),
        }).href,
        change,
        abortController.signal,
        null,
      )
      lastServerChange = changeNumber
      ownChangeNumbers.add(changeNumber)
      ownClaimChanges.set(id, changeNumber)
      await applyOwnChange(changeNumber, change)
      return { id }
    } catch (err) {
      if (abortController.signal.aborted) {
        throw err
      }
      if (err instanceof FetchError && err.status === 409) {
        const { doc: existing } = await getURLDirect<object>(
          router.apiResolve({
            name: "DocumentGetChange",
            params: { session: props.session, change: changeNumber },
          }).href,
          abortController.signal,
          null,
        )
        if (abortController.signal.aborted) {
          throw err
        }
        // The comparison relies on the change serializing exactly as it was posted: no
        // key is ever set to undefined (which JSON.stringify would drop) - the patch
        // builders assign concrete values in every branch and materializeChange adds
        // optional keys conditionally.
        if (equals<object>(existing, change)) {
          lastServerChange = changeNumber
          ownChangeNumbers.add(changeNumber)
          ownClaimChanges.set(id, changeNumber)
          await applyOwnChange(changeNumber, change)
          return { id }
        }
        // Lost the number to a concurrent editor: the loop syncs the doc, renumbers, and
        // revalidates, dropping the change when it no longer applies.
        lastServerChange = changeNumber
        continue
      }
      if (err instanceof FetchError && err.status === 400) {
        throw new ChangeDroppedError(`change rejected by the server: ${JSON.stringify(change)}`, { cause: err })
      }
      await delay(saveRetryInterval, abortController.signal)
      if (abortController.signal.aborted) {
        throw err
      }
    }
  }
}

// saveChange queues a change spec. Changes are posted strictly one after another: the
// server requires change numbers to arrive in sequence and numbers are assigned only at
// post time, so a conflict retry renumbers just the change currently being posted.
let saveChainTail: Promise<unknown> = Promise.resolve()
function saveChange(spec: SaveChangeSpec): Promise<SaveChangeResult> {
  pendingChangeCount.value += 1
  const run = saveChainTail.then(() => postChange(spec))
  // Keep the chain alive even if this change is dropped or fails, so a later change is
  // not blocked forever.
  saveChainTail = run
    .catch(() => undefined)
    .then(() => {
      pendingChangeCount.value -= 1
    })
  return run
}
provide(saveChangeKey, saveChange)

// drainSaveChanges waits until every queued change has settled, including changes queued
// while waiting (e.g. by the focusout commit fired by the Save click itself).
async function drainSaveChanges(): Promise<void> {
  let tail: Promise<unknown>
  do {
    tail = saveChainTail
    await tail
  } while (tail !== saveChainTail)
}

provide(getCommittedClaimKey, (id: string) => (doc.value?.claims.GetByID(id) ?? null) as DeepReadonly<Claim> | null)
provide(registerForFlushKey, (instance: FieldsFormFlush) => {
  flushRegistry.add(instance)
})
provide(unregisterForFlushKey, (instance: FieldsFormFlush) => {
  flushRegistry.delete(instance)
})
provide(registerRemoteConflictKey, (handler: (claimIds: ReadonlySet<string>) => void) => {
  remoteConflictHandlers.add(handler)
})
provide(unregisterRemoteConflictKey, (handler: (claimIds: ReadonlySet<string>) => void) => {
  remoteConflictHandlers.delete(handler)
})
provide(registerRemoteAddsKey, (handler: (claimIds: ReadonlySet<string>) => boolean) => {
  remoteAddHandlers.add(handler)
})
provide(unregisterRemoteAddsKey, (handler: (claimIds: ReadonlySet<string>) => boolean) => {
  remoteAddHandlers.delete(handler)
})

// Warn before the tab closes while changes are still queued or a slot holds local edits
// which have not been committed. We cannot reliably flush and post during unload, so the
// user is prompted to keep the tab open until the data is on the server.
function onBeforeUnload(event: BeforeUnloadEvent): void {
  let unsaved = pendingChangeCount.value > 0
  if (!unsaved) {
    for (const instance of flushRegistry) {
      if (instance.hasUncommitted()) {
        unsaved = true
        break
      }
    }
  }
  if (unsaved) {
    event.preventDefault()
  }
}
window.addEventListener("beforeunload", onBeforeUnload)
onBeforeUnmount(() => {
  window.removeEventListener("beforeunload", onBeforeUnload)
})

// Poll interval.
const pollInterval = 100 // In milliseconds.

// Resolve field definitions for the document's class(es).
const docRef = toRef(() => doc.value ?? null)
const { classDocs, instanceOfClassIds, initialized: classesInitialized } = useParentClasses(docRef, el, busy)
const { fieldsData: mergedFieldsData, classTabId } = useDocumentFields(classDocs, instanceOfClassIds)

// claimAncestry returns the ids of the claims on the path from a top-level claim down to
// (and including) the claim with the given id, or null when the container does not hold it.
function claimAncestry(claims: ClaimTypes | undefined, id: string): string[] | null {
  if (!claims) {
    return null
  }
  for (const claim of claims.AllClaims()) {
    if (claim.id === id) {
      return [claim.id]
    }
    const below = claimAncestry(claim.sub, id)
    if (below) {
      return [claim.id, ...below]
    }
  }
  return null
}

// Applies session changes [fromChange+1 .. latest] to target. Returns the new highest
// applied change number and the ids of claims touched by remote changes (changes this
// client did not post itself), together with each touched claim's ancestors, so a change
// deep in a claim tree also resyncs the slots holding the tree. Caller owns publishing
// target into reactive state - this helper deliberately does not touch _doc.value or
// committedChange, so the initial load can build the doc off-tree and publish it
// atomically (see loadAndSubscribe).
async function applyPendingChanges(target: D, fromChange: number): Promise<{ next: number; remoteTouched: Set<string> }> {
  const remoteTouched = new Set<string>()
  const { doc: lastChangeResponse } = await getURLDirect<LastOperationResponse>(
    router.apiResolve({
      name: "DocumentLastChange",
      params: {
        session: props.session,
      },
    }).href,
    abortController.signal,
    null,
  )
  if (abortController.signal.aborted) {
    return { next: fromChange, remoteTouched }
  }
  const lastChange = lastChangeResponse.lastOperation
  if (lastChange > lastServerChange) {
    lastServerChange = lastChange
  }
  let current = fromChange
  for (; current < lastChange; current++) {
    const { doc: changeDoc } = await getURL<object>(
      router.apiResolve({
        name: "DocumentGetChange",
        params: {
          session: props.session,
          change: current + 1,
        },
      }).href,
      null,
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return { next: current, remoteTouched }
    }
    // A change from another editor: its target claim id, or null for our own changes.
    const remoteId = !ownChangeNumbers.has(current + 1) && "id" in changeDoc && typeof changeDoc.id === "string" ? changeDoc.id : null
    const isAdd = "type" in changeDoc && changeDoc.type === "add"
    // The ancestor chain of a remove/set/cast target has to be resolved BEFORE the change
    // is applied (a removed claim is no longer in the doc); an add's chain only exists
    // after.
    let chain: string[] | null = null
    if (remoteId !== null && !isAdd) {
      chain = claimAncestry(target.claims, remoteId)
    }
    const change = changeFrom(changeDoc)
    await change.Apply(target)
    if (remoteId !== null && isAdd) {
      chain = claimAncestry(target.claims, remoteId)
    }
    if (remoteId !== null) {
      for (const claimId of chain ?? [remoteId]) {
        remoteTouched.add(claimId)
      }
    }
  }
  // Do not notify about claims we have a newer committed change for which is not applied
  // yet (the changes list was fetched before it landed) - resyncing from the doc now
  // would regress the slot to the older remote state, and no later notification would
  // correct it because our own changes are not "remote". The next poll applies our
  // change and brings the doc up to date.
  const applied = lastChange
  for (const claimId of [...remoteTouched]) {
    if ((ownClaimChanges.get(claimId) ?? 0) > applied) {
      remoteTouched.delete(claimId)
    }
  }
  return { next: current, remoteTouched }
}

// loadChanges applies newly committed changes into the live doc and notifies slots about
// claims touched by remote changes. A shared in-flight promise deduplicates overlapping
// calls (the subscription and conflict retries both call it), and the body runs under
// docApplyChain so it cannot interleave with applyOwnChange.
let loadChangesRunning: Promise<void> | null = null
function loadChanges(): Promise<void> {
  if (loadChangesRunning) {
    return loadChangesRunning
  }
  loadChangesRunning = runDocApplySerialized(async () => {
    try {
      const { next, remoteTouched } = await applyPendingChanges(_doc.value!, committedChange.value)
      if (abortController.signal.aborted) {
        return
      }
      committedChange.value = next
      if (remoteTouched.size > 0) {
        // Phase one: existing slots resync their claims. Their handlers read committed
        // state through call-time lookups (getCommittedClaim goes to the doc directly),
        // so this is correct at any nesting depth regardless of handler order. Phase
        // two, after the render flush has propagated the resynced claims into every
        // cardinality's modelValue: cardinalities add slots for remotely added claims.
        // The adds run in rounds: a slot filled in one round feeds its
        // sub-cardinalities' modelValue only after the next render flush, so each round
        // can reveal claims one nesting level deeper. Rounds stop when no cardinality
        // adds anything (the cap is a runaway backstop far above any real claim depth).
        for (const handler of remoteConflictHandlers) {
          handler(remoteTouched)
        }
        for (let round = 0; round < 10; round++) {
          await nextTick()
          if (abortController.signal.aborted) {
            return
          }
          let added = false
          for (const handler of remoteAddHandlers) {
            added = handler(remoteTouched) || added
          }
          if (!added) {
            break
          }
        }
      }
    } finally {
      loadChangesRunning = null
    }
  })
  return loadChangesRunning
}

// syncChanges observes at least everything committed to the session before the call: an
// in-flight loadChanges may have fetched the changes list before, so it is awaited first
// and a fresh run started after.
async function syncChanges(): Promise<void> {
  if (loadChangesRunning) {
    await loadChangesRunning.catch(() => undefined)
  }
  await loadChanges()
}

async function loadAndSubscribe() {
  const { doc: editStatus } = await getURL<DocumentEditStatus>(
    router.apiResolve({
      name: "DocumentEdit",
      params: {
        id: props.id,
        session: props.session,
      },
    }).href,
    null,
    abortController.signal,
    null,
  )
  if (abortController.signal.aborted) {
    return
  }

  // For edit sessions the API returns a parent version we fetch the document
  // at. For create sessions there is no parent yet (the document is materialized
  // on Save), so we start with an empty document built from the session-allocated
  // id/base. Pending session changes (instance_of plus anything the user added
  // in this or a prior load of the same session) are then applied locally below.
  let initialDoc: object
  isCreating.value = !editStatus.version
  if (editStatus.version) {
    const fetched = await getURL<object>(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
        query: encodeQuery({ version: editStatus.version }),
      }).href,
      null,
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return
    }
    initialDoc = fetched.doc
  } else {
    initialDoc = { id: props.id, base: editStatus.base ?? [], claims: {} }
  }

  // Build the doc locally and apply the session's pending changes
  // before publishing _doc. Otherwise reactive consumers
  // (useParentClasses, useDocumentFields) observe a transient state
  // where the doc exists but its claims have not been applied yet -
  // for fresh sessions the instance_of ref arrives via the first
  // change, so the empty-classIds branch in useParentClasses would
  // briefly mark classes as initialized with no class tab, causing
  // a tab-mount race where the class tab is registered late and
  // ends up at a non-selected index.
  //
  // We also keep a pristine deep copy of the just-constructed document
  // (so applyPendingChanges below cannot mutate it through shared
  // object references) as _initialDoc - that one remains the baseline
  // for the per-property "changed" badge and Revert in FieldsForm,
  // regardless of how many session changes the user has already
  // accumulated on a previous load of this same session.
  const localDoc = new D(initialDoc)
  const pristine = localDoc.Clone()
  const { next: initialChange } = await applyPendingChanges(localDoc, 0)
  if (abortController.signal.aborted) {
    return
  }
  _doc.value = localDoc
  _initialDoc.value = pristine
  committedChange.value = initialChange

  // TODO: Use websocket to watch for new changes.
  const timer = setInterval(() => {
    loadChanges().catch((error) => {
      // TODO: Show error state to the user.
      console.error("loadAndSubscribe interval", error)
    })
  }, pollInterval)
  abortController.signal.addEventListener("abort", () => {
    clearInterval(timer)
  })
}
// Re-initialize when route params change.
watch(
  () => ({ id: props.id, session: props.session }),
  () => {
    // Abort previous session's work.
    cleanup()
    abortController = new AbortController()

    // Reset state.
    _doc.value = null
    _initialDoc.value = null
    isCreating.value = null
    committedChange.value = 0
    lastServerChange = 0
    ownChangeNumbers.clear()
    ownClaimChanges.clear()
    // The previous session's queued changes have been aborted above. They settle on their
    // own (decrementing pendingChangeCount as they do), the chain just starts fresh.
    saveChainTail = Promise.resolve()
    fieldsFormInvalid.value = false
    pendingInitialFocus = true

    loadAndSubscribe().catch((error) => {
      // TODO: Show error state to the user.
      console.error("loadAndSubscribe", error)
    })
  },
  // Initialize the first time.
  {
    immediate: true,
  },
)

// Once the FieldsForm's per-field inputs have mounted and registered, move
// keyboard focus into the form. Create sessions land on the first still-
// empty field (fields pre-filled by claims already in the session are
// skipped); edit sessions land on the first field regardless of its value.
watch(
  () => fieldsFormRef.value?.inputs.size ?? 0,
  async (size) => {
    const creating = isCreating.value
    if (!pendingInitialFocus || size === 0 || creating === null) {
      return
    }
    pendingInitialFocus = false
    // Let the just-registered inputs finish rendering before focusing.
    await nextTick()
    if (abortController.signal.aborted || !fieldsFormRef.value) {
      return
    }
    focusFirstInput(fieldsFormRef.value.inputs, creating)
  },
)

async function onSave() {
  if (abortController.signal.aborted) {
    return
  }

  sessionError.value = ""

  // Validate the FieldsForm tab before saving (only mounted when that tab is
  // active - the All-properties tab does not need this gate because its claim
  // form is submitted independently). If validation finds errors, focus the
  // first invalid input and abort the save - no changes flushed, the session
  // stays open for the user to fix the field.
  if (fieldsFormRef.value) {
    await fieldsFormRef.value.validateAll(abortController.signal, { final: true })
    if (abortController.signal.aborted) {
      return
    }
    if (fieldsFormInvalid.value) {
      focusFirstInvalid(fieldsFormRef.value.inputs)
      return
    }
  }

  // Flush any pending edits from all slot inputs before saving (each flush commits like
  // the slot's blur would; invalid values stay in the form and set fieldsFormInvalid),
  // then wait for every queued change to settle on the server - including changes queued
  // outside the flush, e.g. by the focusout commit fired by the Save click itself.
  for (const instance of flushRegistry) {
    await instance.flush()
  }
  await drainSaveChanges()
  if (abortController.signal.aborted) {
    return
  }

  // Re-check after flush: validateAll above clears stale state, but flush itself
  // might surface new invalidity if mutation watchers fired. Abort save but keep
  // the valid changes posted above.
  if (fieldsFormInvalid.value) {
    return
  }

  // Stop polling for changes before ending the session by aborting and creating a fresh controller.
  // The fresh controller is needed for the save request itself.
  abortController.abort()
  abortController = new AbortController()

  saveBusy.value += 1
  try {
    await postJSON<DocumentEndEditResponse>(
      router.apiResolve({
        name: "DocumentEndEdit",
        params: {
          session: props.session,
        },
      }).href,
      {},
      abortController.signal,
      saveBusy,
    )
    if (abortController.signal.aborted) {
      return
    }

    // Poll until the session is fully completed (document committed).
    const editStatusURL = router.apiResolve({
      name: "DocumentEdit",
      params: {
        id: props.id,
        session: props.session,
      },
    }).href
    while (true) {
      await delay(pollInterval, abortController.signal)
      if (abortController.signal.aborted) {
        return
      }
      const { doc: status } = await getURLDirect<DocumentEditStatus>(editStatusURL, abortController.signal, saveBusy)
      if (abortController.signal.aborted) {
        return
      }
      if (status.changeset || status.discarded) {
        break
      }
    }

    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
    await router.push({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentEdit.onSave", err)
    // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
    sessionError.value = `${err}`
  } finally {
    saveBusy.value -= 1
  }
}

async function onDiscard() {
  if (abortController.signal.aborted) {
    return
  }

  sessionError.value = ""

  // Stop polling for changes before discarding the session by aborting and creating a fresh controller.
  // The fresh controller is needed for the discard request itself.
  abortController.abort()
  abortController = new AbortController()

  saveBusy.value += 1
  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentDiscardEdit",
        params: {
          session: props.session,
        },
      }).href,
      {},
      abortController.signal,
      saveBusy,
    )
    if (abortController.signal.aborted) {
      return
    }

    await router.push({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentEdit.onDiscard", err)
    // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
    sessionError.value = `${err}`
  } finally {
    saveBusy.value -= 1
  }
}

function makePatch(): object {
  // The "file" value type produces a "link" claim with an IRI obtained from the file upload.
  const backendType = claimType.value === "file" ? "link" : claimType.value
  const shared = { type: backendType, confidence: HighConfidence, prop: claimProp.value }
  switch (claimType.value) {
    case "id":
      return { ...shared, value: claimValue.value }
    case "string":
      return { ...shared, string: claimValue.value }
    case "html":
      return { ...shared, html: claimValue.value }
    case "amount":
      return { ...shared, amount: claimValue.value, precision: parseFloat(claimAmountPrecision.value) }
    case "amountInterval": {
      // Each bound is one of: explicit amount+precision pair, fromIsNone/
      // toIsNone (explicitly no value), or fromIsUnknown/toIsUnknown
      // (value exists but is unknown). The InputMissing wrapper guarantees
      // unknown/none are mutually exclusive per bound.
      const fromPart = claimFromUnknown.value
        ? { fromIsUnknown: true }
        : claimFromNone.value
          ? { fromIsNone: true }
          : claimFrom.value
            ? { from: claimFrom.value, fromPrecision: parseFloat(claimFromAmountPrecision.value) }
            : {}
      const toPart = claimToUnknown.value
        ? { toIsUnknown: true }
        : claimToNone.value
          ? { toIsNone: true }
          : claimTo.value
            ? { to: claimTo.value, toPrecision: parseFloat(claimToAmountPrecision.value) }
            : {}
      return { ...shared, ...fromPart, ...toPart }
    }
    case "time":
      return { ...shared, time: claimValue.value, precision: claimTimePrecision.value }
    case "timeInterval": {
      // Each bound is one of: explicit time+precision pair, fromIsNone/
      // toIsNone (explicitly no value), or fromIsUnknown/toIsUnknown
      // (value exists but is unknown). The InputMissing wrapper guarantees
      // unknown/none are mutually exclusive per bound.
      const fromPart = claimFromUnknown.value
        ? { fromIsUnknown: true }
        : claimFromNone.value
          ? { fromIsNone: true }
          : claimFrom.value
            ? { from: claimFrom.value, fromPrecision: claimFromTimePrecision.value }
            : {}
      const toPart = claimToUnknown.value
        ? { toIsUnknown: true }
        : claimToNone.value
          ? { toIsNone: true }
          : claimTo.value
            ? { to: claimTo.value, toPrecision: claimToTimePrecision.value }
            : {}
      return { ...shared, ...fromPart, ...toPart }
    }
    case "link":
      return { ...shared, iri: claimValue.value }
    case "file":
      return { ...shared, iri: claimValue.value }
    case "ref":
      return { ...shared, to: claimValue.value }
    case "has":
    case "none":
    case "unknown":
      return shared
    default:
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      throw new Error(`unsupported claim type: ${claimType.value}`)
  }
}

async function onSubmit() {
  if (abortController.signal.aborted) {
    return
  }

  // Run validation across every registered input in the claim form.
  // If any field surfaces an error, focus the first one and abort the
  // submit so the user can fix it before we hit the backend.
  await validateAll(abortController.signal, { final: true })
  if (abortController.signal.aborted) {
    return
  }
  if (anyError.value) {
    focusFirstInvalid(inputs)
    return
  }

  try {
    const patch = makePatch()
    const spec: SaveChangeSpec = editingClaimId.value
      ? { type: "set", id: editingClaimId.value, patch }
      : subClaimParentId.value
        ? { type: "add", patch, under: subClaimParentId.value }
        : { type: "add", patch }
    await saveChange(spec)
    if (abortController.signal.aborted) {
      return
    }
    // Dispatches the reset event, which runs onReset (resetAll) and clears
    // any native form controls so the form is ready for the next claim.
    claimFormRef.value?.reset()
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentEdit.onSubmit", err)
    // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
    claimFormError.value = `${err}`
  }
}

async function onReset() {
  // We do not use .prevent so the browser also resets plain inputs.
  // Here we reset registered input components.
  resetAll()
  claimFormError.value = ""
  editingClaimId.value = null
  subClaimParentId.value = null
  lockedClaimType.value = null
  // Re-checkpoint so the now-empty inputs are not considered dirty
  // (which would keep the submit button enabled after a Cancel from edit
  // mode). Wait one tick before checkpointing for reset to propagate.
  await nextTick()
  checkpointAll()
}

async function onEditClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }
  if (!doc.value) {
    return
  }

  const claim = doc.value.claims.GetByID(id)
  if (!claim) {
    return
  }

  // Unlock the tabs for one tick so every tab becomes enabled before the
  // new selected-index change lands. Headless UI's TabGroup resolves its
  // selected-index against each tab's DOM disabled attribute via a pre-flush
  // watcher, so if both the disabled state and selected-index change in the
  // same render (transitioning between two different-typed edits), it sees
  // the previous render's disabled state and pins the panel to the prior
  // type. lockedClaimType is decoupled from editingClaimId so the title and
  // primary-button label do not flip back to "Add value" during this gap.
  lockedClaimType.value = null
  await nextTick()

  // Start from a clean slate so stale fields from a prior add/edit do not
  // leak into the patch sent on save.
  resetAll()
  claimFormError.value = ""

  if (claim instanceof IdentifierClaim) {
    claimType.value = "id"
    claimProp.value = claim.prop.id
    claimValue.value = claim.value
  } else if (claim instanceof StringClaim) {
    claimType.value = "string"
    claimProp.value = claim.prop.id
    claimValue.value = claim.string
  } else if (claim instanceof HTMLClaim) {
    claimType.value = "html"
    claimProp.value = claim.prop.id
    claimValue.value = claim.html
  } else if (claim instanceof AmountClaim) {
    claimType.value = "amount"
    claimProp.value = claim.prop.id
    claimValue.value = claim.amount
    claimAmountPrecision.value = String(claim.precision)
  } else if (claim instanceof AmountIntervalClaim) {
    claimType.value = "amountInterval"
    claimProp.value = claim.prop.id
    claimFrom.value = claim.from ?? ""
    claimFromAmountPrecision.value = claim.fromPrecision !== undefined ? String(claim.fromPrecision) : ""
    claimFromUnknown.value = !!claim.fromIsUnknown
    claimFromNone.value = !!claim.fromIsNone
    claimTo.value = claim.to ?? ""
    claimToAmountPrecision.value = claim.toPrecision !== undefined ? String(claim.toPrecision) : ""
    claimToUnknown.value = !!claim.toIsUnknown
    claimToNone.value = !!claim.toIsNone
  } else if (claim instanceof TimeClaim) {
    claimType.value = "time"
    claimProp.value = claim.prop.id
    claimValue.value = claim.time
    claimTimePrecision.value = claim.precision
  } else if (claim instanceof TimeIntervalClaim) {
    claimType.value = "timeInterval"
    claimProp.value = claim.prop.id
    claimFrom.value = claim.from ?? ""
    claimFromTimePrecision.value = claim.fromPrecision ?? "y"
    claimFromUnknown.value = !!claim.fromIsUnknown
    claimFromNone.value = !!claim.fromIsNone
    claimTo.value = claim.to ?? ""
    claimToTimePrecision.value = claim.toPrecision ?? "y"
    claimToUnknown.value = !!claim.toIsUnknown
    claimToNone.value = !!claim.toIsNone
  } else if (claim instanceof LinkClaim) {
    // A LinkClaim pointing at a StorageGet URL is what the "file" tab
    // produces on add; route it back to that tab so editing matches the
    // affordance the user originally used.
    claimType.value = classifyLink(claim.iri, router).includes(LINK_CLASS_FILE) ? "file" : "link"
    claimProp.value = claim.prop.id
    claimValue.value = claim.iri
  } else if (claim instanceof ReferenceClaim) {
    claimType.value = "ref"
    claimProp.value = claim.prop.id
    claimValue.value = claim.to.id
  } else if (claim instanceof HasClaim) {
    claimType.value = "has"
    claimProp.value = claim.prop.id
  } else if (claim instanceof NoneClaim) {
    claimType.value = "none"
    claimProp.value = claim.prop.id
  } else if (claim instanceof UnknownClaim) {
    claimType.value = "unknown"
    claimProp.value = claim.prop.id
  } else {
    throw new Error("unsupported claim type")
  }

  editingClaimId.value = id
  subClaimParentId.value = null
  lockedClaimType.value = claimType.value

  // Wait for the new panel's inputs to mount and register, then move focus
  // to the first focusable one so the user can start editing immediately.
  await nextTick()
  firstInputEl()?.focus()
  // Record the populated values as the checkpoint so the form is not
  // dirty until the user actually changes something.
  checkpointAll()
}

// Configures the claim form to add a new claim as a sub-claim of the row
// the user clicked Sub-value on. Mirrors onEditClaim's reset-then-focus
// dance but leaves the form blank (we are adding, not editing). Tabs stay
// unlocked because the user picks the type for the new sub-claim.
async function onSubClaimAdd(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  resetAll()
  claimFormError.value = ""
  editingClaimId.value = null
  subClaimParentId.value = id
  lockedClaimType.value = null

  await nextTick()
  firstInputEl()?.focus()
  checkpointAll()
}

async function onRemoveClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  try {
    await saveChange({ type: "remove", id })
    if (abortController.signal.aborted) {
      return
    }
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentEdit.onRemoveClaim", err)
  }
}

function onChangeClaimTab(index: number) {
  if (abortController.signal.aborted) {
    return
  }

  claimType.value = claimTypes[index]
}

function canSave(): boolean {
  // Save commits the edit session's changes - nothing to commit, nothing to save.
  // We do enable button even when inputs are invalid because we want the user to
  // attempt a save and force validation (and focus to first invalid input).
  // FieldsForm defers existing-claim deletes to flush, so a session whose only
  // pending change is an emptied claim row has committedChange === 0 but
  // anyDirty === true; treat that as savable too.
  return committedChange.value > 0 || (fieldsFormRef.value?.anyDirty ?? false)
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <template #start>
        <NavBarSearch />
      </template>
    </NavBar>
  </Teleport>
  <div ref="el" class="pd-documentedit mt-[var(--pd-navbar-height)] flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:gap-y-4 sm:p-4">
    <div class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="hasPermission(CAN_EDIT_DOCUMENT) && doc && classesInitialized">
        <!--
          TODO: Fix how hover interacts with focused tab.
          See: https://github.com/tailwindlabs/tailwindcss/discussions/10123
        -->
        <TabGroup manual>
          <TabList class="-m-4 mb-4 flex border-collapse flex-row rounded-t border-b border-gray-200 bg-slate-100 contain-inline-size">
            <Tab
              v-if="classTabId && mergedFieldsData"
              :key="classTabId"
              class="min-w-0 overflow-hidden border-r border-gray-200 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
              ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%_-_--spacing(4)),transparent)] px-4 py-3 whitespace-nowrap"
                ><DocumentRefInline :id="classTabId" :link="false" title /></span
            ></Tab>
            <Tab
              :title="t('views.DocumentEdit.tabs.allProperties')"
              class="min-w-0 overflow-hidden border-r border-gray-200 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
              ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%_-_--spacing(4)),transparent)] px-4 py-3 whitespace-nowrap">{{
                t("views.DocumentEdit.tabs.allProperties")
              }}</span></Tab
            >
          </TabList>
          <h1 v-show="displayLabelComponent?.displayLabel" class="mb-4 text-3xl font-bold drop-shadow-xs"><DisplayLabel ref="displayLabelComponent" :doc="doc" /></h1>
          <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
          <TabPanels as="template">
            <!-- Class-specific tab. -->
            <TabPanel v-if="classTabId && mergedFieldsData" :key="classTabId" tabindex="-1" class="outline-none">
              <div @focusout="onFieldsBlur">
                <FieldsForm
                  ref="fieldsFormRef"
                  v-model:invalid="fieldsFormInvalid"
                  :fields-data="mergedFieldsData"
                  :claims="doc.claims"
                  :initial-claims="initialDoc?.claims ?? doc.claims"
                />
                <!-- Potential duplicates of the document being created, refreshed on every field blur. -->
                <DocumentDuplicates v-if="isCreating" ref="duplicatesRef" :doc="doc" />
              </div>
            </TabPanel>
            <!-- "All properties" tab panel. -->
            <TabPanel tabindex="-1" class="outline-none">
              <table class="w-full table-auto border-collapse">
                <thead>
                  <tr>
                    <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.property") }}</th>
                    <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.value") }}</th>
                    <th class="w-px"></th>
                    <th class="w-px"></th>
                    <th class="w-px"></th>
                  </tr>
                </thead>
                <tbody>
                  <PropertiesRows
                    :claims="doc.claims"
                    editable
                    :editing-claim-id="editingClaimId"
                    :sub-claim-parent-id="subClaimParentId"
                    @edit-claim="onEditClaim"
                    @remove-claim="onRemoveClaim"
                    @sub-claim="onSubClaimAdd"
                  />
                </tbody>
              </table>
              <form ref="claimFormRef" @submit.prevent="onSubmit" @reset="onReset">
                <h2 class="mt-4 text-xl font-medium">{{
                  editingClaimId ? t("views.DocumentEdit.editClaim") : subClaimParentId ? t("views.DocumentEdit.addSubClaim") : t("views.DocumentEdit.addClaim")
                }}</h2>
                <TabGroup :selected-index="selectedClaimTab" @change="onChangeClaimTab">
                  <TabList class="mt-4 flex border-collapse flex-row border border-gray-200 bg-slate-100 contain-inline-size">
                    <Tab
                      v-for="type in claimTypes"
                      :key="type"
                      :disabled="claimTypeDisabled(type)"
                      :title="claimTypeLabel(type)"
                      class="min-w-0 overflow-hidden border-r border-gray-200 leading-tight font-medium uppercase outline-none select-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
                      :class="claimTypeDisabled(type) ? 'cursor-not-allowed opacity-50' : 'not-aria-selected:hover:bg-slate-50'"
                      ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%_-_--spacing(4)),transparent)] px-4 py-3 whitespace-nowrap">{{
                        claimTypeLabel(type)
                      }}</span></Tab
                    >
                  </TabList>
                  <TabPanels as="template">
                    <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.identifier')" class="mt-4">
                        <template #input="inputProps">
                          <InputIdentifier v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.string')" class="mt-4">
                        <template #input="inputProps">
                          <InputString v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.html')" class="mt-4">
                        <template #input="inputProps">
                          <InputHTML v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #input="inputProps">
                          <InputAmount v-bind="inputProps" v-model="claimValue" v-model:precision="claimAmountPrecision" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.from')" class="mt-4">
                        <template #input="inputProps">
                          <InputMissing v-bind="inputProps" v-model:unknown="claimFromUnknown" v-model:none="claimFromNone">
                            <template #default="missingProps">
                              <InputAmount v-bind="missingProps" v-model="claimFrom" v-model:precision="claimFromAmountPrecision" />
                            </template>
                          </InputMissing>
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.to')" class="mt-4">
                        <template #input="inputProps">
                          <InputMissing v-bind="inputProps" v-model:unknown="claimToUnknown" v-model:none="claimToNone">
                            <template #default="missingProps">
                              <InputAmount v-bind="missingProps" v-model="claimTo" v-model:precision="claimToAmountPrecision" />
                            </template>
                          </InputMissing>
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #input="inputProps">
                          <InputTime v-bind="inputProps" v-model="claimValue" v-model:precision="claimTimePrecision" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.from')" class="mt-4">
                        <template #input="inputProps">
                          <InputMissing v-bind="inputProps" v-model:unknown="claimFromUnknown" v-model:none="claimFromNone">
                            <template #default="missingProps">
                              <InputTime v-bind="missingProps" v-model="claimFrom" v-model:precision="claimFromTimePrecision" />
                            </template>
                          </InputMissing>
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.to')" class="mt-4">
                        <template #input="inputProps">
                          <InputMissing v-bind="inputProps" v-model:unknown="claimToUnknown" v-model:none="claimToNone">
                            <template #default="missingProps">
                              <InputTime v-bind="missingProps" v-model="claimTo" v-model:precision="claimToTimePrecision" />
                            </template>
                          </InputMissing>
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.iri')" class="mt-4">
                        <template #input="inputProps">
                          <InputLink v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.file')" class="mt-4">
                        <template #input="inputProps">
                          <InputFile v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                      <InputField required :label="t('views.DocumentEdit.labels.to')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required :label="t('common.labels.property')" class="mt-4">
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" :filter="PROPERTY_FILTER" />
                        </template>
                      </InputField>
                    </TabPanel>
                  </TabPanels>
                </TabGroup>
                <div v-if="claimFormError" class="mt-4 text-error-600">{{ t("common.errors.unexpected") }}</div>
                <div class="mt-4 flex flex-row justify-end gap-4">
                  <Button type="reset" :disabled="allEmpty && !anyError">{{ t("common.buttons.cancel") }}</Button>
                  <!--
                    We do enable button even when inputs are invalid because we want the user to
                    attempt a add/update and force validation (and focus to first invalid input).
                  -->
                  <Button type="submit" :disabled="!anyDirty">{{ editingClaimId ? t("common.buttons.update") : t("common.buttons.add") }}</Button>
                </div>
              </form>
            </TabPanel>
          </TabPanels>
        </TabGroup>
        <div v-if="sessionError" class="mt-4 text-error-600">{{ t("common.errors.unexpected") }}</div>
        <div class="mt-4 flex flex-row justify-between gap-4">
          <Button id="documentedit-button-discard" type="button" :progress="saveBusy" @click.prevent="onDiscard">{{ t("common.buttons.discard") }}</Button>
          <Button id="documentedit-button-save" type="submit" primary :disabled="!canSave()" :progress="saveBusy" @click.prevent="onSave">{{
            isCreating ? t("common.buttons.create") : t("common.buttons.update")
          }}</Button>
        </div>
      </template>
      <div v-else-if="!hasPermission(CAN_EDIT_DOCUMENT)" class="my-1 text-center sm:my-4">{{ t("common.status.editingNotAllowed") }}</div>
      <div v-else-if="!classesInitialized" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
      <div v-else class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
