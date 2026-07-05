<!--
FieldsFormRow renders the value-input cells of a single FieldsForm row -
one row in an InputCardinality (or a single direct row for max=1 fields).
The wrapping <tr>/<td> belong to the parent FieldsFormField; this component
only emits the input controls themselves so the parent can stack them
inside whatever table layout it likes.

The row registers itself as one ValidatedInput with the enclosing
registry (the InputCardinality's sub-registry, typically), aggregating
its inner inputs (a single input for scalar types, or two stacked
InputMissing-wrapped inputs - "from" on top, "to" below - for intervals)
into one rowwise unit. Doing so lets the parent track per-row identity
(e.g. claim ID) keyed on this input, and lets useRepeatedInput.modelFor(input)
bind every row model through a single key.

The ten value/precision/missing-state defineModels expose the entry's
FieldEntryValue fields one-by-one so the parent's v-model:value /
v-model:value-to / etc. bind directly into its reactive entry record
without this component having to mutate a single prop object.

Has/None/Unknown are presence-only and have no value input - the parent
renders a sole checkbox in the value cell instead and never mounts this
component for those.
-->

<script setup lang="ts">
import type { DeepReadonly, WritableComputedRef } from "vue"

import type { FieldData, FieldEntryValue } from "@/fields"
import type { InputColumn, ValidatedInput } from "@/types"

import { computed, shallowRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import { VT_FILE } from "@/core"
import { emptyFieldEntryValue, valueTypeToClaimType } from "@/fields"
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
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"

const props = defineProps<{
  field: DeepReadonly<FieldData>
  // Drives the wrapped input's own "Required" check. The parent flips this on
  // when the field's minCardinality is not yet satisfied so the validators
  // light up the empty entry/slot inputs.
  required: boolean
  // Presentational red ring on the input. Used for the cardinality-short
  // visual hint, orthogonal to per-input validation errors.
  invalid: boolean
  // Renders every inner input read-only (grayed and non-interactive, but selectable).
  // Set by ClaimInput while the slot's changes are queued or in flight.
  readonly?: boolean
  // Slot-level revert passed through to every InputField, so the per-input changed
  // badge's revert posts the reverting changes like the field-level revert does (see
  // the revert prop on InputField).
  revert?: () => void
  // Id of the (sub)field's label element, threaded down from the field's
  // ClaimCardinality, so a bare single-column input is named via InputField's
  // labelledby. Undefined when not in a FieldsForm context.
  labelId?: string
}>()

// input notifies the parent on any user-driven model change; it is emitted by every
// inner input's @update:model-value handler. missingChange additionally fires when a
// missing-state checkbox (unknown/none) of the given interval bound is toggled, so the
// parent can commit a newly checked state immediately instead of waiting for blur.
// completeChange additionally fires for model changes which are complete decisions on
// their own (a finished file upload, a cleared file): there is no natural blur after
// them (focus never left the slot), so the parent commits immediately as well.
const emit = defineEmits<{ input: []; missingChange: [side: "from" | "to"]; completeChange: [] }>()

// One v-model for the whole entry. Parent (ClaimInput) owns a single
// reactive FieldEntryValue; each inner input updates its own slice via
// the computed wrappers below, which spread the entry and emit it back.
const entry = defineModel<FieldEntryValue>("entry", { default: () => emptyFieldEntryValue() })

// Writes from the inner inputs can come in bursts within one tick (e.g. the missing-state
// checkboxes keep unknown/none mutually exclusive with two consecutive writes). The entry
// model only emits upward - until the parent re-renders, entry.value keeps returning the
// previous object - so a second write would spread the stale base and silently drop the
// first one. head is the newest written object, used as the base until the parent's next
// entry value arrives (our own write round-tripping back, or an external replacement,
// which then wins).
const head = shallowRef<FieldEntryValue | null>(null)
watch(
  () => entry.value,
  () => {
    head.value = null
  },
  { flush: "sync" },
)

function fieldRef<K extends keyof FieldEntryValue>(key: K): WritableComputedRef<FieldEntryValue[K]> {
  return computed({
    get: () => (head.value ?? entry.value)[key],
    set: (v) => {
      const next = { ...(head.value ?? entry.value), [key]: v }
      head.value = next
      entry.value = next
    },
  })
}

const value = fieldRef("value")
const valueTo = fieldRef("valueTo")
const amountPrecision = fieldRef("amountPrecision")
const amountPrecisionTo = fieldRef("amountPrecisionTo")
const timePrecision = fieldRef("timePrecision")
const timePrecisionTo = fieldRef("timePrecisionTo")
const fromUnknown = fieldRef("fromUnknown")
const fromNone = fieldRef("fromNone")
const toUnknown = fieldRef("toUnknown")
const toNone = fieldRef("toNone")

const { t } = useI18n({ useScope: "global" })

const claimType = computed(() => valueTypeToClaimType(props.field.valueType))
const isFile = computed(() => props.field.valueType === VT_FILE)

// When the value input IS the whole field (a single non-repeated value with no
// sub-fields), its whole-input changed/revert badge duplicates the field-level
// badge next to the field's label, so InputField hides it. Repeated fields keep
// a per-slot badge; intervals keep their distinct From/To badges (set below).
const inputIsWholeField = computed(() => props.field.maxCardinality <= 1 && props.field.subFields.length === 0)

// Sub-registry: every inner input (InputString, InputAmount, InputMissing,
// etc.) registers here instead of bubbling directly to the ancestor
// FieldsForm / InputCardinality. We aggregate them into a single
// ValidatedInput exposed to the enclosing registry. That single input is
// what useRepeatedInput keys its per-row store on, what InputCardinality
// counts toward its min/max, and what FieldsFormField maps to claim IDs.
//
// The forwardInteraction indirection wires inner-input interactions
// through to the parent registry. useValidationRegistry only sets up its
// own notifyUp when it is given an el (and self-registers internally); we
// do the self-register manually below so we can use firstInputEl as the
// focus target without the forward-reference dance. To keep typing or focus
// inside an inner input bubbling up to e.g. InputCardinality.adjustRows, we
// wire our useRegisterForValidation's returned onInteraction into the
// sub-registry's onInteraction callback.
let forwardInteraction: (() => void) | null = null
const { validateAll, resetAll, revertAll, checkpointAll, anyDirty, allEmpty, inputs, firstInputEl } = useValidationRegistry(() => {
  forwardInteraction?.()
})

// The value input's columns, merged across the inner inputs. FieldsFormRow
// stacks its inputs vertically (a single input for a scalar, "from" above "to"
// for an interval), so its columns line up top-to-bottom: the merged result has
// the max column count over the inputs, each column keeping the first non-empty
// label found at that position (empty if none). A column-less input counts as
// one unlabeled column (the same fallback InputField/InputMissing use), so it is
// not skipped.
const columns = computed<InputColumn[]>(() => {
  const merged: InputColumn[] = []
  for (const input of inputs) {
    const cols = input.columns?.value ?? [{ label: "", el: () => input.inputEl() ?? null }]
    for (let i = 0; i < cols.length; i++) {
      if (i >= merged.length) {
        merged.push(cols[i])
      } else if (merged[i].label === "" && cols[i].label !== "") {
        merged[i] = cols[i]
      }
    }
  }
  return merged
})

const validatedInput: ValidatedInput = {
  validate: validateAll,
  reset: resetAll,
  revert: revertAll,
  // Focus target: the first focusable inner input. firstInputEl is the
  // sub-registry's earliest focusable; for an interval row that lands on
  // the "from" side, for a scalar row on the single input. The row has no
  // wrapper of its own, so mainEl is the same element.
  inputEl: firstInputEl,
  mainEl: firstInputEl,
  isDirty: anyDirty,
  // "Row empty" for FieldsFormRow is "every inner input empty" - the
  // sub-registry's allEmpty. This matches what InputCardinality wants
  // for its missing-required check and what useRepeatedInput uses to
  // skip empty rows in values()/entries().
  isEmpty: allEmpty,
  errors: allErrors(inputs),
  columns,
  checkpoint: checkpointAll,
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose(validatedInput)

// The inner inputs' validators (e.g. InputString) close over props.required,
// but useValidation re-runs only on model changes, not on prop changes. When a
// slot stops being designated (required goes false), re-validate so any
// "Required value." it was showing clears at once. We deliberately do NOT
// re-validate when required goes true: the "required" badge is driven by the
// prop alone, while the "Required value." text must wait for the user to leave
// the empty slot (the input's own @blur), so a slot that just became required
// (or a freshly-opened form) does not light up before it is touched.
//
// flush: "post" is essential: props.required propagates DOWN to the inner input
// during render, so at the default "pre" timing the input still sees required=true
// and validateAll would re-assert the very "Required value." we mean to clear
// (InputTime even sets its own triggered=true in validate()). Running after the
// render lets the input observe required=false, so it validates clean and clears.
watch(
  () => props.required,
  (isRequired) => {
    if (!isRequired) {
      void validateAll()
    }
  },
  { flush: "post" },
)

function onInput() {
  emit("input")
}

function onMissingInput(side: "from" | "to") {
  emit("input")
  emit("missingChange", side)
}

function onCompleteInput() {
  emit("input")
  emit("completeChange")
}
</script>

<template>
  <!--
    Each value input is wrapped in InputField. InputField renders the per-input
    labels + whole-input changed/revert badge for multi-column inputs
    (amount/precision, interval bounds), or nothing for single-column inputs
    (their label and whole-field badge live in FieldsFormField's left cell,
    referenced via labelledby). required/invalid flow to the inner input
    through InputField's slot props.
  -->
  <!-- id -->
  <InputField v-if="claimType === 'id'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputIdentifier v-bind="inputProps" v-model="value" :readonly="readonly" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- string -->
  <InputField v-else-if="claimType === 'string'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputString v-bind="inputProps" v-model="value" :readonly="readonly" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- html -->
  <InputField v-else-if="claimType === 'html'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputHTML v-bind="inputProps" v-model="value" :readonly="readonly" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- amount -->
  <InputField v-else-if="claimType === 'amount'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputAmount
        v-bind="inputProps"
        v-model="value"
        v-model:precision="amountPrecision"
        :readonly="readonly"
        @update:model-value="onInput"
        @update:precision="onInput"
      />
    </template>
  </InputField>

  <!-- amountInterval - "from" and "to" stack vertically, one InputField each. -->
  <div v-else-if="claimType === 'amountInterval'" class="flex min-w-0 flex-col gap-y-4">
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.from')" :revert="revert">
      <template #input="inputProps">
        <InputMissing
          v-bind="inputProps"
          v-model:unknown="fromUnknown"
          v-model:none="fromNone"
          :readonly="readonly"
          @update:unknown="onMissingInput('from')"
          @update:none="onMissingInput('from')"
        >
          <template #default="missingProps">
            <InputAmount
              v-bind="missingProps"
              v-model="value"
              v-model:precision="amountPrecision"
              :readonly="readonly"
              @update:model-value="onInput"
              @update:precision="onInput"
            />
          </template>
        </InputMissing>
      </template>
    </InputField>
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.to')" :revert="revert">
      <template #input="inputProps">
        <InputMissing
          v-bind="inputProps"
          v-model:unknown="toUnknown"
          v-model:none="toNone"
          :readonly="readonly"
          @update:unknown="onMissingInput('to')"
          @update:none="onMissingInput('to')"
        >
          <template #default="missingProps">
            <InputAmount
              v-bind="missingProps"
              v-model="valueTo"
              v-model:precision="amountPrecisionTo"
              :readonly="readonly"
              @update:model-value="onInput"
              @update:precision="onInput"
            />
          </template>
        </InputMissing>
      </template>
    </InputField>
  </div>

  <!-- time -->
  <InputField v-else-if="claimType === 'time'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputTime v-bind="inputProps" v-model="value" v-model:precision="timePrecision" :readonly="readonly" @update:model-value="onInput" @update:precision="onInput" />
    </template>
  </InputField>

  <!-- timeInterval - "from" and "to" stack vertically, one InputField each. -->
  <div v-else-if="claimType === 'timeInterval'" class="flex min-w-0 flex-col gap-y-4">
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.from')" :revert="revert">
      <template #input="inputProps">
        <InputMissing
          v-bind="inputProps"
          v-model:unknown="fromUnknown"
          v-model:none="fromNone"
          :readonly="readonly"
          @update:unknown="onMissingInput('from')"
          @update:none="onMissingInput('from')"
        >
          <template #default="missingProps">
            <InputTime
              v-bind="missingProps"
              v-model="value"
              v-model:precision="timePrecision"
              :readonly="readonly"
              @update:model-value="onInput"
              @update:precision="onInput"
            />
          </template>
        </InputMissing>
      </template>
    </InputField>
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.to')" :revert="revert">
      <template #input="inputProps">
        <InputMissing
          v-bind="inputProps"
          v-model:unknown="toUnknown"
          v-model:none="toNone"
          :readonly="readonly"
          @update:unknown="onMissingInput('to')"
          @update:none="onMissingInput('to')"
        >
          <template #default="missingProps">
            <InputTime
              v-bind="missingProps"
              v-model="valueTo"
              v-model:precision="timePrecisionTo"
              :readonly="readonly"
              @update:model-value="onInput"
              @update:precision="onInput"
            />
          </template>
        </InputMissing>
      </template>
    </InputField>
  </div>

  <!-- link (no file affordance) -->
  <InputField v-else-if="claimType === 'link' && !isFile" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputLink v-bind="inputProps" v-model="value" :readonly="readonly" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- link with file value type: render the file-upload affordance instead. -->
  <InputField v-else-if="claimType === 'link' && isFile" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <InputFile v-bind="inputProps" v-model="value" :readonly="readonly" @update:model-value="onCompleteInput" />
    </template>
  </InputField>

  <!-- ref -->
  <InputField v-else-if="claimType === 'ref'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField" :revert="revert">
    <template #input="inputProps">
      <!-- TODO: Pass "self" prop as the current document's ID. -->
      <InputRef v-bind="inputProps" v-model="value" :readonly="readonly" :filter="field.values" @update:model-value="onInput" />
    </template>
  </InputField>
</template>
