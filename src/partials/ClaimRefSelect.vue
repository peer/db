<!--
ClaimRefSelect renders a reference field whose filtered set of candidate documents
is small as a list of all options: deselectable radio buttons when the field holds
at most one value, checkboxes when it is repeated. Unlike the combobox slots
(InputRef inside ClaimInput), one ClaimRefSelect manages ALL of the field's claims
itself: toggling an option adds/removes (or, for the single-value radio, sets) the
claim through the change queue right away, so there is no uncommitted local state
and no per-entry rows.

It registers itself as one ValidatedInput with the enclosing registry: dirty
compares the selected target ids against the session baseline, revert reconciles
the selection back to it (posting the reverting changes), and validation flags a
selection short of the field's min cardinality.

There is no optimistic UI: a toggle shows only once its change committed, and the
whole list is disabled while a change is queued or in flight, like the slots are.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim, ClaimTypes, D } from "@/document"
import type { FieldData } from "@/fields"
import type { Result, SaveChangeResult, SaveChangeSpec, ValidatedInput, ValidationError } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, inject, nextTick, ref, shallowRef, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import CheckBox from "@/components/CheckBox.vue"
import RadioButton from "@/components/RadioButton.vue"
import WithDocument from "@/components/WithDocument.vue"
import { claimPatchFrom } from "@/document"
import { ChangeDroppedError, emptyFieldEntryValue, getClaimValues, getCommittedClaimKey, makePatchForField, saveChangeKey } from "@/fields"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"
import { pickErrorMessage, useRegisterForValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    // The field's committed claims (the doc's view). The selection state is kept
    // locally and updated at every commit; this prop is only adopted when it
    // diverges while no own change is in flight (a remote change, server wins).
    modelValue: DeepReadonly<readonly Claim[]>
    // Pre-session claims: the baseline for the changed state and revert.
    initialClaims: DeepReadonly<readonly Claim[]>
    field: DeepReadonly<FieldData>
    // The complete set of candidate documents (the enclosing cardinality only
    // mounts this component when the filtered search returned all of them).
    options: readonly Result[]
    // Checkboxes (repeated field) instead of deselectable radio buttons.
    multiple: boolean
    // See ClaimInput's prop of the same name.
    parentClaimId?: () => Promise<string>
    // parentCleanup asks the enclosing slot to remove its lazily-created base claim
    // once a removal leaves this field without claims, mirroring the slots'
    // updateSlotClaim; see ClaimCardinality's prop of the same name.
    parentCleanup?: () => Promise<void>
    invalid?: boolean
    readonly?: boolean
    // Id of the field's label element, naming the fieldset via aria-labelledby.
    labelId?: string
  }>(),
  {
    parentClaimId: undefined,
    parentCleanup: undefined,
    invalid: false,
    readonly: false,
    labelId: undefined,
  },
)

const saveChange = inject(saveChangeKey, (spec: SaveChangeSpec) => Promise.resolve({ id: "id" in spec ? spec.id : "" }))
const getCommittedClaim = inject(getCommittedClaimKey, () => null)

const { t } = useI18n({ useScope: "global" })

const baseId = useId()
const fieldsetRef = useTemplateRef<HTMLFieldSetElement>("fieldsetRef")

// claimTarget returns the document id a reference claim points to.
function claimTarget(claim: DeepReadonly<Claim>): string {
  return getClaimValues(claim).value
}

// The committed claims, locally owned like ClaimInput's currentClaim: updated
// synchronously at every commit so consecutive toggles observe the state left
// behind by the previous one instead of the lagging prop.
const current = shallowRef<readonly DeepReadonly<Claim>[]>(props.modelValue)

const selectedTargets = computed<Set<string>>(() => new Set(current.value.map(claimTarget)))

// The revert/changed baseline; moved forward by checkpoint() after Save.
const checkpointClaims = ref<readonly DeepReadonly<Claim>[]>(props.initialClaims)
watch(
  () => props.initialClaims,
  (v) => {
    checkpointClaims.value = v
  },
  { flush: "sync" },
)

// Number of this field's changes queued or in flight; the list is disabled while
// non-zero so no further toggles pile up on an unsettled committed state.
const pendingCount = ref(0)
const inactive = computed<boolean>(() => props.readonly || pendingCount.value > 0)

// fingerprint identifies a claim list by ids and targets, for the external-write
// comparison below (an own commit echoes back with the same ids and targets).
function fingerprint(claims: DeepReadonly<readonly Claim[]>): string {
  return claims
    .map((claim) => `${claim.id}=${claimTarget(claim)}`)
    .sort()
    .join(",")
}

// A prop value diverging from the local claims is an external write (a remote
// change; the doc echo of own commits matches by ids and targets). Server wins,
// unless an own change is in flight - it either overrides the remote one or is
// dropped by the queue's conflict handling, which resyncs below.
watch(
  () => props.modelValue,
  (v) => {
    if (pendingCount.value > 0) {
      return
    }
    if (fingerprint(v) === fingerprint(current.value)) {
      return
    }
    current.value = v
  },
)

// Operations are serialized so a second toggle (or Save's revert) observes the
// state left behind by the first instead of racing it.
let operationChain: Promise<unknown> = Promise.resolve()
function runSerialized<T>(fn: () => Promise<T>): Promise<T> {
  const run = operationChain.then(fn)
  operationChain = run.catch(() => undefined)
  return run
}

// submitChange queues one change and tracks it as pending. On a dropped change
// (lost to a concurrent change, see ChangeDroppedError) the local claims resync
// to the committed state and the caller stops its flow.
async function submitChange(spec: SaveChangeSpec): Promise<SaveChangeResult | null> {
  pendingCount.value += 1
  try {
    return await saveChange(spec)
  } catch (err) {
    if (err instanceof ChangeDroppedError) {
      current.value = current.value.map((claim) => getCommittedClaim(claim.id)).filter((claim): claim is DeepReadonly<Claim> => claim !== null)
      return null
    }
    throw err
  } finally {
    pendingCount.value -= 1
  }
}

function patchFor(target: string): object {
  return makePatchForField(props.field, { ...emptyFieldEntryValue(), value: target })
}

// doAdd commits an AddClaimChange for the given target, resolving the (possibly
// lazily-created) parent first, like ClaimInput.addClaimWithParent.
async function doAdd(target: string): Promise<void> {
  const patch = patchFor(target)
  const under = props.parentClaimId ? await props.parentClaimId() : undefined
  const result = await submitChange(under === undefined ? { type: "add", patch } : { type: "add", patch, under })
  if (!result) {
    return
  }
  current.value = [...current.value, claimPatchFrom(patch).New(result.id)]
}

async function doRemove(claim: DeepReadonly<Claim>): Promise<void> {
  if (!(await submitChange({ type: "remove", id: claim.id }))) {
    return
  }
  current.value = current.value.filter((c) => c !== claim)
  // A removal which left the field claim-less may have emptied the enclosing
  // slot's lazily-created base claim; ask it to clean up (it has its own guards).
  if (current.value.length === 0 && props.parentCleanup) {
    void props.parentCleanup()
  }
}

// doSet points an existing claim at a new target, preserving its id (and any
// sub-claims it carries outside the schema).
async function doSet(claim: DeepReadonly<Claim>, target: string): Promise<void> {
  const patch = patchFor(target)
  if (!(await submitChange({ type: "set", id: claim.id, patch }))) {
    return
  }
  const updated = claimPatchFrom(patch).New(claim.id)
  if (claim.sub) {
    updated.sub = claim.sub as unknown as ClaimTypes
  }
  current.value = current.value.map((c) => (c === claim ? updated : c))
}

// toggle handles a checkbox (repeated field).
function toggle(target: string, checked: boolean): void {
  clearErrors()
  void runSerialized(async () => {
    const existing = current.value.find((claim) => claimTarget(claim) === target)
    if (checked && !existing) {
      await doAdd(target)
    } else if (!checked && existing) {
      await doRemove(existing)
    }
  })
}

// select handles the radio (single-value field); undefined is a deselect.
function select(target: string | undefined): void {
  clearErrors()
  void runSerialized(async () => {
    const existing = current.value.length > 0 ? current.value[0] : null
    if (target === undefined) {
      if (existing) {
        await doRemove(existing)
      }
    } else if (!existing) {
      await doAdd(target)
    } else if (claimTarget(existing) !== target) {
      await doSet(existing, target)
    }
  })
}

// The radio's model. The getter reflects the committed claim, so the selection
// visibly moves only once the change committed (no optimistic UI).
const selectedSingle = computed<string | undefined>({
  get: () => (current.value.length > 0 ? claimTarget(current.value[0]) : undefined),
  set: (v) => {
    select(v)
  },
})

// Options are rendered in the given order; committed claims whose target is not
// among them (a stale claim no longer matching the field's filter) are appended
// as extra rows so the user can still see and unselect them.
const rows = computed<string[]>(() => {
  const listed = props.options.map((option) => option.id)
  const extra = current.value.map(claimTarget).filter((target) => !listed.includes(target))
  return [...listed, ...extra]
})

// Validation: a selection short of the field's min cardinality is flagged on the
// validate() cascade (Save) and on leaving the fieldset, and cleared again on the
// first interaction, like other inputs do.
const errors = ref<ValidationError[]>([])

function clearErrors(): void {
  errors.value = []
  onInteraction()
}

// eslint-disable-next-line @typescript-eslint/require-await
async function validate(): Promise<void> {
  if (current.value.length < props.field.minCardinality) {
    // TODO: Use standard codes.
    errors.value = [{ code: "required" }]
    return
  }
  errors.value = []
}

const errorMessage = computed<string | null>(() => pickErrorMessage(errors.value, t))

// Focus has actually left the fieldset (not just moved between its controls). Run
// validation so the required error appears as soon as the user tabs/clicks away
// from a selection which is still too small. The nextTick is needed because
// focusout fires while document.activeElement is still in transition.
async function onFocusout(): Promise<void> {
  await nextTick()
  if (fieldsetRef.value?.contains(document.activeElement)) {
    return
  }
  await validate()
}

// revert reconciles the selection back to the checkpoint, posting the reverting
// changes right away like the slots' revert does. Serialized with the toggles so
// an in-flight commit cannot race it. Re-added baseline claims get fresh
// content-addressed ids; dirty compares targets, so that still counts as pristine.
function revert(): Promise<void> {
  return runSerialized(async () => {
    const baseline = checkpointClaims.value
    if (!props.multiple) {
      const want = baseline.length > 0 ? claimTarget(baseline[0]) : undefined
      const existing = current.value.length > 0 ? current.value[0] : null
      if (want === undefined) {
        if (existing) {
          await doRemove(existing)
        }
      } else if (!existing) {
        await doAdd(want)
      } else if (claimTarget(existing) !== want) {
        await doSet(existing, want)
      }
      return
    }
    const baselineTargets = new Set(baseline.map(claimTarget))
    for (const claim of [...current.value]) {
      if (!baselineTargets.has(claimTarget(claim))) {
        await doRemove(claim)
      }
    }
    const currentTargets = new Set(current.value.map(claimTarget))
    for (const target of baselineTargets) {
      if (!currentTargets.has(target)) {
        await doAdd(target)
      }
    }
  })
}

function targetsEqual(a: DeepReadonly<readonly Claim[]>, b: DeepReadonly<readonly Claim[]>): boolean {
  const setA = new Set(a.map(claimTarget))
  const setB = new Set(b.map(claimTarget))
  return setA.size === setB.size && [...setA].every((target) => setB.has(target))
}

const validatedInput: ValidatedInput = {
  validate,
  reset: () => {
    errors.value = []
  },
  revert: () => {
    void revert()
  },
  inputEl: () => fieldsetRef.value?.querySelector<HTMLInputElement>("input") ?? null,
  mainEl: () => fieldsetRef.value,
  isDirty: computed(() => !targetsEqual(current.value, checkpointClaims.value)),
  isEmpty: computed(() => current.value.length === 0),
  errors,
  checkpoint: () => {
    checkpointClaims.value = current.value
  },
}

const { onInteraction } = useRegisterForValidation(validatedInput)

// Restores the focus which the prevented mousedown suppressed, so focus lands (and
// stays) inside the fieldset and leaving it later triggers the focusout validation.
function focusControl(row: string): void {
  document.getElementById(`${baseId}-${row}`)?.focus()
}

defineExpose({
  ...validatedInput,
  // Override the sync wrapper with the actual async function so the enclosing
  // cardinality's revertField can await it.
  revert,
})

const WithPeerDBDocument = WithDocument<D>
</script>

<template>
  <fieldset ref="fieldsetRef" class="pd-claimrefselect" :aria-labelledby="labelId || undefined" @focusout="onFocusout">
    <ul class="grid grid-cols-[max-content_auto] gap-x-1">
      <!--
        The controls and labels prevent mousedown so clicking them does not blur the
        previously focused element first: that blur's commit would flash the enclosing
        slot read-only and the then-disabled control would swallow the click the user
        is in the middle of (the toggle itself commits through the parent's serialized
        chain, so the pending parent value still gets committed first, see
        ensureClaimId). The click itself still toggles; focus is restored onto the
        control afterwards (a control disabled by the immediate commit refuses focus,
        which is harmless - nothing was blurred either).
      -->
      <li v-for="row in rows" :key="row" class="contents">
        <RadioButton
          v-if="!multiple"
          :id="`${baseId}-${row}`"
          v-model="selectedSingle"
          :name="baseId"
          :value="row"
          :disabled="inactive"
          :invalid="invalid"
          @mousedown.prevent
          @click="focusControl(row)"
        />
        <CheckBox
          v-else
          :id="`${baseId}-${row}`"
          :model-value="selectedTargets.has(row)"
          :disabled="inactive"
          :invalid="invalid"
          @mousedown.prevent
          @click="focusControl(row)"
          @update:model-value="(v) => toggle(row, !!v)"
        />
        <div class="flex items-baseline gap-x-1">
          <WithPeerDBDocument :id="row" name="DocumentGet">
            <template #default="{ doc, url }">
              <label
                :for="`${baseId}-${row}`"
                :class="inactive ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                :data-url="url"
                @mousedown.prevent
                @click="focusControl(row)"
                ><DisplayLabel :doc="doc"
              /></label>
            </template>
            <template #loading="{ url }">
              <span
                class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                :data-url="url"
                :class="[loadingWidth(row)]"
                aria-hidden="true"
              />
            </template>
            <template #error="{ message, accessDenied, url }">
              <i :class="['pd-withdocument-error', accessDenied ? 'text-gray-500' : 'text-error-600']" :data-url="url">{{ message }}</i>
            </template>
          </WithPeerDBDocument>
          <!--
            tabindex="-1" keeps the open-link icon out of the keyboard tab
            order so Tab jumps between form fields without stopping on each
            row's icon. Mouse users can still click it; the icon is here as
            a "view document" affordance, not a primary action.
          -->
          <RouterLink :to="{ name: 'DocumentGet', params: { id: row } }" class="link" tabindex="-1"
            ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
          /></RouterLink>
        </div>
      </li>
      <li v-if="rows.length === 0" class="col-span-2 p-2"
        ><i>{{ t("partials.ClaimRefSelect.noOptions") }}</i></li
      >
    </ul>
    <p v-if="errorMessage" class="mt-1 text-sm text-error-600">{{ errorMessage }}</p>
  </fieldset>
</template>
