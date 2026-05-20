<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { ComponentExposed } from "vue-component-type-helpers"

import type { TimePrecision } from "@/document"
import type { FieldsFormSaveChange, FlushFn } from "@/fields"
import type { DocumentEditStatus, DocumentEndEditResponse, ValidatedInput, ValidateFn } from "@/types"

import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/vue"
import { computed, nextTick, onBeforeUnmount, provide, readonly, ref, toRef, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, getURL, getURLDirect, postJSON } from "@/api"
import { CAN_EDIT, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
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
import { changeFrom, RemoveClaimChange, SetClaimChange } from "@/document/patch"
import { getNextChangeNumberKey, registerForFlushKey, saveChangeKey, unregisterForFlushKey } from "@/fields"
import { classifyLink, LINK_CLASS_FILE } from "@/internal-links"
import DisplayLabel from "@/partials/DisplayLabel.vue"
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
import InputErrors from "@/partials/InputErrors.vue"
import InputField from "@/partials/InputField.vue"
import InputMissing from "@/partials/InputMissing.vue"
import NavBar from "@/partials/NavBar.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { localCounter, pairCounters, useLock, useProgress } from "@/progress"
import { useDocumentFields } from "@/useDocumentFields"
import { useParentClasses } from "@/useParentClasses"
import { delay, encodeQuery, makeAddClaimChange } from "@/utils"
import { focusFirstInvalid, useValidationRegistry } from "@/validation"

const props = defineProps<{
  id: string
  session: string
}>()

type ClaimType = "id" | "string" | "html" | "amount" | "amountInterval" | "time" | "timeInterval" | "link" | "file" | "ref" | "has" | "none" | "unknown"
const claimTypes: ClaimType[] = ["id", "string", "html", "amount", "amountInterval", "time", "timeInterval", "link", "file", "ref", "has", "none", "unknown"]
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
// the primary button label, and the onSubmit branch (SetClaimChange vs
// makeAddClaimChange).
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

const { resetAll, firstEl, allEmpty, anyDirty, anyError, inputs, validateAll, checkpointAll } = useValidationRegistry(() => {
  // Any registered-input interaction clears stale form-level errors so the
  // user is not staring at an error message after they have moved on.
  sessionError.value = ""
  claimFormError.value = ""
})

let abortController = new AbortController()

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

// Tracks the change number which was committed in the backend.
// A ref so canSave reactively follows whether anything has been added to
// the session yet (Save is disabled when the session has no changes).
const committedChange = ref(0)
// Tracks the next change number to submit (may be ahead of committedChange when changes are in-flight).
let nextChangeToSubmit = 1

const fieldsFormInvalid = ref(false)

// Flush registry: all FieldsForm instances register here so we can flush them before save.
const flushRegistry = new Set<FlushFn>()

// Provide shared services for recursive FieldsForm instances.
provide(getNextChangeNumberKey, () => nextChangeToSubmit++)
provide(saveChangeKey, async (change: object, changeNumber: number) => {
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
})
provide(registerForFlushKey, (instance: FlushFn) => {
  flushRegistry.add(instance)
})
provide(unregisterForFlushKey, (instance: FlushFn) => {
  flushRegistry.delete(instance)
})

// Poll interval in milliseconds.
const pollInterval = 1000

// Resolve field definitions for the document's class(es).
const docRef = toRef(() => doc.value ?? null)
const { classDocs, instanceOfClassIds, initialized: classesInitialized } = useParentClasses(docRef, el, busy)
const { fieldsData: mergedFieldsData, classTabId } = useDocumentFields(classDocs, instanceOfClassIds)

// Applies session changes [fromChange+1 .. latest] to target. Returns the
// new highest applied change number. Caller owns publishing target into
// reactive state - this helper deliberately does not touch _doc.value or
// committedChange, so the initial load can build the doc off-tree and
// publish it atomically (see loadAndSubscribe).
async function applyPendingChanges(target: D, fromChange: number): Promise<number> {
  const { doc: changesList } = await getURLDirect<number[]>(
    router.apiResolve({
      name: "DocumentListChanges",
      params: {
        session: props.session,
      },
    }).href,
    abortController.signal,
    null,
  )
  if (abortController.signal.aborted) {
    return fromChange
  }
  let current = fromChange
  for (; changesList.length > 0 && current < changesList[0]; current++) {
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
      return current
    }
    const change = changeFrom(changeDoc)
    await change.Apply(target)
  }
  return current
}

let running = false
async function loadChanges() {
  if (running) {
    return
  }
  running = true
  try {
    const next = await applyPendingChanges(_doc.value!, committedChange.value)
    if (abortController.signal.aborted) {
      return
    }
    committedChange.value = next
  } finally {
    running = false
  }
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
  // We also keep a pristine D instance constructed from the same raw
  // JSON (deep-cloned so applyPendingChanges below cannot mutate it
  // through shared object references) as _initialDoc - that one
  // remains the baseline for the per-property "changed" badge and
  // Revert in FieldsForm, regardless of how many session changes the
  // user has already accumulated on a previous load of this same
  // session.
  const localDoc = new D(initialDoc)
  const pristine = new D(structuredClone(initialDoc))
  const initialChange = await applyPendingChanges(localDoc, 0)
  if (abortController.signal.aborted) {
    return
  }
  _doc.value = localDoc
  _initialDoc.value = pristine
  committedChange.value = initialChange
  nextChangeToSubmit = initialChange + 1

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
    nextChangeToSubmit = 1
    fieldsFormInvalid.value = false

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
    await fieldsFormRef.value.validateAll(abortController.signal)
    if (abortController.signal.aborted) {
      return
    }
    if (fieldsFormInvalid.value) {
      focusFirstInvalid(fieldsFormRef.value.inputs)
      return
    }
  }

  // Flush any pending edits from all FieldsForm instances before saving.
  // Flush returns only valid changes; invalid fields remain and set fieldsFormInvalid.
  const allPendingChanges: FieldsFormSaveChange[] = []
  for (const flush of flushRegistry) {
    const changes = await flush()
    allPendingChanges.push(...changes)
  }

  // Post all flushed changes first (they are valid and have consumed change numbers).
  for (const { change, changeNumber } of allPendingChanges) {
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
    if (abortController.signal.aborted) {
      return
    }
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
  await validateAll(abortController.signal)
  if (abortController.signal.aborted) {
    return
  }
  if (anyError.value) {
    focusFirstInvalid(inputs)
    return
  }

  try {
    const change = editingClaimId.value
      ? new SetClaimChange({ id: editingClaimId.value, patch: makePatch() })
      : await makeAddClaimChange(doc.value!.base, props.session, committedChange.value + 1, makePatch(), subClaimParentId.value ?? undefined)
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: props.session,
        },
        query: encodeQuery({ change: String(committedChange.value + 1) }),
      }).href,
      change,
      abortController.signal,
      null,
    )
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
  firstEl()?.focus()
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
  firstEl()?.focus()
  checkpointAll()
}

async function onRemoveClaim(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: props.session,
        },
        query: encodeQuery({ change: String(committedChange.value + 1) }),
      }).href,
      new RemoveClaimChange({
        id,
      }),
      abortController.signal,
      null,
    )
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
  <div ref="el" class="pd-documentedit mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div class="rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="hasPermission(CAN_EDIT) && doc && classesInitialized">
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
              ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%-1rem),transparent)] px-4 py-3 whitespace-nowrap"
                ><DocumentRefInline :id="classTabId" :link="false" title /></span
            ></Tab>
            <Tab
              :title="t('views.DocumentEdit.tabs.allProperties')"
              class="min-w-0 overflow-hidden border-r border-gray-200 leading-tight font-medium uppercase outline-none select-none first:rounded-tl not-aria-selected:hover:bg-slate-50 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 aria-selected:bg-white"
              ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%-1rem),transparent)] px-4 py-3 whitespace-nowrap">{{
                t("views.DocumentEdit.tabs.allProperties")
              }}</span></Tab
            >
          </TabList>
          <h1 v-show="displayLabelComponent?.displayLabel" class="mb-4 text-4xl font-bold drop-shadow-xs"><DisplayLabel ref="displayLabelComponent" :doc="doc" /></h1>
          <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
          <TabPanels as="template">
            <!-- Class-specific tab. -->
            <TabPanel v-if="classTabId && mergedFieldsData" :key="classTabId" tabindex="-1" class="outline-none">
              <FieldsForm
                ref="fieldsFormRef"
                v-model:invalid="fieldsFormInvalid"
                :fields-data="mergedFieldsData"
                :claims="doc.claims"
                :initial-claims="initialDoc?.claims ?? doc.claims"
                :base="doc.base"
                :session="session"
              />
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
                <h2 class="mt-4 text-xl font-bold drop-shadow-xs">{{
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
                      ><span class="block [mask-image:linear-gradient(to_right,black_calc(100%-1rem),transparent)] px-4 py-3 whitespace-nowrap">{{
                        claimTypeLabel(type)
                      }}</span></Tab
                    >
                  </TabList>
                  <TabPanels as="template">
                    <!-- We explicitly disable tabbing. See: https://github.com/tailwindlabs/headlessui/discussions/1433 -->
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.identifier") }}</template>
                        <template #input="inputProps">
                          <InputIdentifier v-bind="inputProps" v-model="claimValue" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.string") }}</template>
                        <template #input="inputProps">
                          <InputString v-bind="inputProps" v-model="claimValue" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.html") }}</template>
                        <template #input="inputProps">
                          <InputHTML v-bind="inputProps" v-model="claimValue" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputErrors v-slot="errorProps">
                        <InputAmount v-bind="errorProps" v-model="claimValue" v-model:precision="claimAmountPrecision" required class="mt-4 min-w-0 flex-auto grow" />
                      </InputErrors>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputErrors v-slot="errorProps" class="mt-4">
                        <InputMissing v-bind="errorProps" v-model:unknown="claimFromUnknown" v-model:none="claimFromNone" required>
                          <template #default="missingProps">
                            <InputAmount v-bind="missingProps" v-model="claimFrom" v-model:precision="claimFromAmountPrecision" class="min-w-0 flex-auto grow">
                              <template #amount-label>{{ t("views.DocumentEdit.labels.from") }}</template>
                            </InputAmount>
                          </template>
                        </InputMissing>
                      </InputErrors>
                      <InputErrors v-slot="errorProps" class="mt-4">
                        <InputMissing v-bind="errorProps" v-model:unknown="claimToUnknown" v-model:none="claimToNone" required>
                          <template #default="missingProps">
                            <InputAmount v-bind="missingProps" v-model="claimTo" v-model:precision="claimToAmountPrecision" class="min-w-0 flex-auto grow">
                              <template #amount-label>{{ t("views.DocumentEdit.labels.to") }}</template>
                            </InputAmount>
                          </template>
                        </InputMissing>
                      </InputErrors>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputErrors v-slot="errorProps">
                        <InputTime v-bind="errorProps" v-model="claimValue" v-model:precision="claimTimePrecision" required class="mt-4 min-w-0 flex-auto grow" />
                      </InputErrors>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputErrors v-slot="errorProps" class="mt-4">
                        <InputMissing v-bind="errorProps" v-model:unknown="claimFromUnknown" v-model:none="claimFromNone" required>
                          <template #default="missingProps">
                            <InputTime v-bind="missingProps" v-model="claimFrom" v-model:precision="claimFromTimePrecision" class="min-w-0 flex-auto grow">
                              <template #time-label>{{ t("views.DocumentEdit.labels.from") }}</template>
                            </InputTime>
                          </template>
                        </InputMissing>
                      </InputErrors>
                      <InputErrors v-slot="errorProps" class="mt-4">
                        <InputMissing v-bind="errorProps" v-model:unknown="claimToUnknown" v-model:none="claimToNone" required>
                          <template #default="missingProps">
                            <InputTime v-bind="missingProps" v-model="claimTo" v-model:precision="claimToTimePrecision" class="min-w-0 flex-auto grow">
                              <template #time-label>{{ t("views.DocumentEdit.labels.to") }}</template>
                            </InputTime>
                          </template>
                        </InputMissing>
                      </InputErrors>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.iri") }}</template>
                        <template #input="inputProps">
                          <InputLink v-bind="inputProps" v-model="claimValue" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.file") }}</template>
                        <template #input="inputProps">
                          <InputFile v-bind="inputProps" v-model="claimValue" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                      <InputField required class="mt-4">
                        <template #label>{{ t("views.DocumentEdit.labels.to") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimValue" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
                        </template>
                      </InputField>
                    </TabPanel>
                    <TabPanel tabindex="-1" class="flex flex-col outline-none">
                      <InputField required class="mt-4">
                        <template #label>{{ t("common.labels.property") }}</template>
                        <template #input="inputProps">
                          <InputRef v-bind="inputProps" v-model="claimProp" class="min-w-0 flex-auto grow" />
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
      <div v-else-if="!hasPermission(CAN_EDIT)" class="my-1 text-center sm:my-4">{{ t("common.status.editingNotAllowed") }}</div>
      <div v-else-if="!classesInitialized" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
      <div v-else class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
