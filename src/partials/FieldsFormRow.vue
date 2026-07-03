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
import type { ValidatedInput } from "@/types"

import { computed, onMounted, watch } from "vue"
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
  // Id of the (sub)field's label element, threaded down from the field's
  // ClaimCardinality, so a bare single-column input is named via InputField's
  // labelledby. Undefined when not in a FieldsForm context.
  labelId?: string
}>()

// Notify the parent on any user-driven model change. Emitted by every
// inner input's @update:model-value handler.
const emit = defineEmits<{ input: [] }>()

// One v-model for the whole entry. Parent (ClaimInput) owns a single
// reactive FieldEntryValue; each inner input updates its own slice via
// the computed wrappers below, which spread the entry and emit it back.
const entry = defineModel<FieldEntryValue>("entry", { default: () => emptyFieldEntryValue() })

function fieldRef<K extends keyof FieldEntryValue>(key: K): WritableComputedRef<FieldEntryValue[K]> {
  return computed({
    get: () => entry.value[key],
    set: (v) => {
      entry.value = { ...entry.value, [key]: v }
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
  checkpoint: checkpointAll,
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose(validatedInput)

// The inner inputs' validators (e.g. InputString) close over props.required,
// but useValidation only re-runs on model changes, not on prop changes.
// Watch props.required and re-run validateAll so toggling the prop (from
// ClaimCardinality flipping the missing-min indicator on) immediately
// surfaces or clears each input's "Required value." text - instead of
// waiting for the next model edit or blur.
watch(
  () => props.required,
  () => {
    void validateAll()
  },
)

// Also run validation on mount once the inner inputs have registered.
// Without this, a fresh row mounted with required=true already set (e.g.
// the cardinality dropped the row the user just cleared and grew a new
// trailing-empty in its place) would not surface "Required value." until
// the next user interaction - useValidation's own immediate watch runs
// with options.initial=true, which the validator skips on purpose to
// avoid yelling at form-load.
onMounted(() => {
  if (props.required) {
    void validateAll()
  }
})

function onInput() {
  emit("input")
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
  <InputField v-if="claimType === 'id'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputIdentifier v-bind="inputProps" v-model="value" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- string -->
  <InputField v-else-if="claimType === 'string'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputString v-bind="inputProps" v-model="value" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- html -->
  <InputField v-else-if="claimType === 'html'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputHTML v-bind="inputProps" v-model="value" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- amount -->
  <InputField v-else-if="claimType === 'amount'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputAmount v-bind="inputProps" v-model="value" v-model:precision="amountPrecision" @update:model-value="onInput" @update:precision="onInput" />
    </template>
  </InputField>

  <!-- amountInterval - "from" and "to" stack vertically, one InputField each. -->
  <div v-else-if="claimType === 'amountInterval'" class="flex min-w-0 flex-col gap-y-1">
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.from')">
      <template #input="inputProps">
        <InputMissing v-bind="inputProps" v-model:unknown="fromUnknown" v-model:none="fromNone" @update:unknown="onInput" @update:none="onInput">
          <template #default="missingProps">
            <InputAmount v-bind="missingProps" v-model="value" v-model:precision="amountPrecision" @update:model-value="onInput" @update:precision="onInput" />
          </template>
        </InputMissing>
      </template>
    </InputField>
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.to')">
      <template #input="inputProps">
        <InputMissing v-bind="inputProps" v-model:unknown="toUnknown" v-model:none="toNone" @update:unknown="onInput" @update:none="onInput">
          <template #default="missingProps">
            <InputAmount v-bind="missingProps" v-model="valueTo" v-model:precision="amountPrecisionTo" @update:model-value="onInput" @update:precision="onInput" />
          </template>
        </InputMissing>
      </template>
    </InputField>
  </div>

  <!-- time -->
  <InputField v-else-if="claimType === 'time'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputTime v-bind="inputProps" v-model="value" v-model:precision="timePrecision" @update:model-value="onInput" @update:precision="onInput" />
    </template>
  </InputField>

  <!-- timeInterval - "from" and "to" stack vertically, one InputField each. -->
  <div v-else-if="claimType === 'timeInterval'" class="flex min-w-0 flex-col gap-y-1">
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.from')">
      <template #input="inputProps">
        <InputMissing v-bind="inputProps" v-model:unknown="fromUnknown" v-model:none="fromNone" @update:unknown="onInput" @update:none="onInput">
          <template #default="missingProps">
            <InputTime v-bind="missingProps" v-model="value" v-model:precision="timePrecision" @update:model-value="onInput" @update:precision="onInput" />
          </template>
        </InputMissing>
      </template>
    </InputField>
    <InputField :required="required" :invalid="invalid" :labelledby="labelId" :label="t('partials.FieldsForm.to')">
      <template #input="inputProps">
        <InputMissing v-bind="inputProps" v-model:unknown="toUnknown" v-model:none="toNone" @update:unknown="onInput" @update:none="onInput">
          <template #default="missingProps">
            <InputTime v-bind="missingProps" v-model="valueTo" v-model:precision="timePrecisionTo" @update:model-value="onInput" @update:precision="onInput" />
          </template>
        </InputMissing>
      </template>
    </InputField>
  </div>

  <!-- link (no file affordance) -->
  <InputField v-else-if="claimType === 'link' && !isFile" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputLink v-bind="inputProps" v-model="value" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- link with file value type: render the file-upload affordance instead. -->
  <InputField v-else-if="claimType === 'link' && isFile" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <InputFile v-bind="inputProps" v-model="value" @update:model-value="onInput" />
    </template>
  </InputField>

  <!-- ref -->
  <InputField v-else-if="claimType === 'ref'" :required="required" :invalid="invalid" :labelledby="labelId" :hide-badge="inputIsWholeField">
    <template #input="inputProps">
      <!-- TODO: Pass "self" prop as the current document's ID. -->
      <InputRef v-bind="inputProps" v-model="value" :filter="field.values" @update:model-value="onInput" />
    </template>
  </InputField>
</template>
