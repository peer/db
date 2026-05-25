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
import InputErrors from "@/partials/InputErrors.vue"
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
// do the self-register manually below so we can use firstEl as the el
// without the forward-reference dance. To keep typing or focus inside an
// inner input bubbling up to e.g. InputCardinality.adjustRows, we wire
// our useRegisterForValidation's returned onInteraction into the
// sub-registry's onInteraction callback.
let forwardInteraction: (() => void) | null = null
const { validateAll, resetAll, revertAll, checkpointAll, anyDirty, allEmpty, inputs, firstEl } = useValidationRegistry(() => {
  forwardInteraction?.()
})

const validatedInput: ValidatedInput = {
  validate: validateAll,
  reset: resetAll,
  revert: revertAll,
  // Focus target: the first focusable inner input. firstEl is the
  // sub-registry's earliest focusable; for an interval row that lands on
  // the "from" side, for a scalar row on the single input.
  el: firstEl,
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
  <!-- id -->
  <InputErrors v-if="claimType === 'id'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputIdentifier v-bind="errorProps" v-model="value" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>

  <!-- string -->
  <InputErrors v-else-if="claimType === 'string'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputString v-bind="errorProps" v-model="value" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>

  <!-- html -->
  <InputErrors v-else-if="claimType === 'html'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputHTML v-bind="errorProps" v-model="value" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>

  <!-- amount -->
  <InputErrors v-else-if="claimType === 'amount'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputAmount
      v-bind="errorProps"
      v-model="value"
      v-model:precision="amountPrecision"
      :required="required"
      :invalid="invalid"
      @update:model-value="onInput"
      @update:precision="onInput"
    />
  </InputErrors>

  <!--
    amountInterval - "from" and "to" stack vertically, one per sub-row.
    The min-w-0/flex-auto/grow on each InputErrors cascades through
    InputMissing's slot down to InputAmount's root, so the amount input
    column grows to fill the row width (otherwise InputAmount's root
    would sit at natural width inside InputMissing's growing slot).
  -->
  <div v-else-if="claimType === 'amountInterval'" class="flex min-w-0 flex-auto grow flex-col gap-y-1">
    <InputErrors v-slot="errorProps" class="min-w-0 flex-auto grow">
      <InputMissing
        v-bind="errorProps"
        v-model:unknown="fromUnknown"
        v-model:none="fromNone"
        :required="required"
        :invalid="invalid"
        @update:unknown="onInput"
        @update:none="onInput"
      >
        <template #default="missingProps">
          <InputAmount v-bind="missingProps" v-model="value" v-model:precision="amountPrecision" @update:model-value="onInput" @update:precision="onInput">
            <template #amount-label>{{ t("partials.FieldsForm.from") }}</template>
          </InputAmount>
        </template>
      </InputMissing>
    </InputErrors>
    <InputErrors v-slot="errorProps" class="min-w-0 flex-auto grow">
      <InputMissing
        v-bind="errorProps"
        v-model:unknown="toUnknown"
        v-model:none="toNone"
        :required="required"
        :invalid="invalid"
        @update:unknown="onInput"
        @update:none="onInput"
      >
        <template #default="missingProps">
          <InputAmount v-bind="missingProps" v-model="valueTo" v-model:precision="amountPrecisionTo" @update:model-value="onInput" @update:precision="onInput">
            <template #amount-label>{{ t("partials.FieldsForm.to") }}</template>
          </InputAmount>
        </template>
      </InputMissing>
    </InputErrors>
  </div>

  <!-- time -->
  <InputErrors v-else-if="claimType === 'time'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputTime
      v-bind="errorProps"
      v-model="value"
      v-model:precision="timePrecision"
      :required="required"
      :invalid="invalid"
      @update:model-value="onInput"
      @update:precision="onInput"
    />
  </InputErrors>

  <!--
    timeInterval - "from" and "to" stack vertically, one per sub-row.
    See amountInterval above for why min-w-0/flex-auto/grow on
    InputErrors is needed.
  -->
  <div v-else-if="claimType === 'timeInterval'" class="flex min-w-0 flex-auto grow flex-col gap-y-1">
    <InputErrors v-slot="errorProps" class="min-w-0 flex-auto grow">
      <InputMissing
        v-bind="errorProps"
        v-model:unknown="fromUnknown"
        v-model:none="fromNone"
        :required="required"
        :invalid="invalid"
        @update:unknown="onInput"
        @update:none="onInput"
      >
        <template #default="missingProps">
          <InputTime v-bind="missingProps" v-model="value" v-model:precision="timePrecision" @update:model-value="onInput" @update:precision="onInput">
            <template #time-label>{{ t("partials.FieldsForm.from") }}</template>
          </InputTime>
        </template>
      </InputMissing>
    </InputErrors>
    <InputErrors v-slot="errorProps" class="min-w-0 flex-auto grow">
      <InputMissing
        v-bind="errorProps"
        v-model:unknown="toUnknown"
        v-model:none="toNone"
        :required="required"
        :invalid="invalid"
        @update:unknown="onInput"
        @update:none="onInput"
      >
        <template #default="missingProps">
          <InputTime v-bind="missingProps" v-model="valueTo" v-model:precision="timePrecisionTo" @update:model-value="onInput" @update:precision="onInput">
            <template #time-label>{{ t("partials.FieldsForm.to") }}</template>
          </InputTime>
        </template>
      </InputMissing>
    </InputErrors>
  </div>

  <!-- link (no file affordance) -->
  <InputErrors v-else-if="claimType === 'link' && !isFile" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputLink v-bind="errorProps" v-model="value" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>

  <!-- link with file value type: render the file-upload affordance instead. -->
  <InputErrors v-else-if="claimType === 'link' && isFile" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputFile v-bind="errorProps" v-model="value" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>

  <!-- ref -->
  <InputErrors v-else-if="claimType === 'ref'" v-slot="errorProps" class="min-w-0 flex-auto grow">
    <InputRef v-bind="errorProps" v-model="value" :filter="field.values" :required="required" :invalid="invalid" @update:model-value="onInput" />
  </InputErrors>
</template>
