<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { TimePrecision } from "@/document"
import type { InputColumn, ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from "@headlessui/vue"
import { CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { computed, ref, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import InputStyled from "@/components/InputStyled.vue"
import InputText from "@/components/InputText.vue"
import { TIME_PRECISIONS_ORDERED } from "@/document/time"
import {
  applyPrecision,
  getStructuredTime,
  inferPrecisionFromNormalized,
  normalizeForParsing,
  progressiveValidate,
  toCanonicalString,
} from "@/partials/input/InputTime.format"
import { useLocked } from "@/progress"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
    maxPrecision?: "G" | "100M" | "10M" | "M" | "100k" | "10k" | "k" | "100y" | "10y" | "y"
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
    maxPrecision: "G",
  },
)

// Two v-models: the time string and the precision value. Both are kept in
// sync by the inner watchers (time -> precision derivation, precision ->
// time reformat). Time is stored in canonical form once the validator
// canonicalizes on blur. Precision is one of the TIME_PRECISIONS_ORDERED
// values accepted by the backend.
const model = defineModel<string>({ default: "" })
const precision = defineModel<TimePrecision>("precision", { default: "y" })

defineOptions({
  inheritAttrs: false,
})

const emit = defineEmits<{ errors: [ValidationError[]] }>()

const { t } = useI18n({ useScope: "global" })

const locked = useLocked()
const inactive = computed(() => locked.value || props.readonly)

const timeInputId = useId()
// Id on the precision listbox button (the column's focusable control) so
// clicking the "Precision" label focuses it.
const precisionButtonId = useId()

const precisionLabels: Record<TimePrecision, string> = {
  G: t("partials.input.InputTime.precision.G"),
  "100M": t("partials.input.InputTime.precision.100M"),
  "10M": t("partials.input.InputTime.precision.10M"),
  M: t("partials.input.InputTime.precision.M"),
  "100k": t("partials.input.InputTime.precision.100k"),
  "10k": t("partials.input.InputTime.precision.10k"),
  k: t("partials.input.InputTime.precision.k"),
  "100y": t("partials.input.InputTime.precision.100y"),
  "10y": t("partials.input.InputTime.precision.10y"),
  y: t("partials.input.InputTime.precision.y"),
  m: t("partials.input.InputTime.precision.m"),
  d: t("partials.input.InputTime.precision.d"),
  h: t("partials.input.InputTime.precision.h"),
  min: t("partials.input.InputTime.precision.min"),
  s: t("partials.input.InputTime.precision.s"),
  ms: t("partials.input.InputTime.precision.ms"),
  us: t("partials.input.InputTime.precision.us"),
  ns: t("partials.input.InputTime.precision.ns"),
}

function precisionLabel(p: TimePrecision): string {
  return precisionLabels[p]
}

const timePrecisionWithMax = computed(() => {
  const reversed = TIME_PRECISIONS_ORDERED.toReversed()
  const maxIdx = reversed.indexOf(props.maxPrecision)

  if (maxIdx < 0) return reversed

  return reversed.slice(0, maxIdx + 1)
})

// timeAuthoritative tracks who edited last so the cross-field auto-sync
// (time -> precision via watch(model), precision -> time via watch(precision))
// only runs in one direction per change. Flips to true on a time keystroke,
// false when the user selects a precision from the dropdown.
const timeAuthoritative = ref(true)
const timeAuthoritativeCheckpoint = ref(timeAuthoritative.value)

// Gate: the time validator surfaces "required" only after the user has
// left the whole widget at least once (focusout to outside, or an
// external validate via parent submit). Tabbing between the time input
// and the precision dropdown does NOT trip this, so the pair only goes
// red once the user is really done editing the field. Reset clears the
// gate so a cancelled form starts fresh.
const triggered = ref(false)

// One-shot suppression for watch(model)'s precision re-inference. Holds
// the canonical value the validator just wrote so the watcher can
// recognize and skip that specific update (e.g. the validator rewriting
// "1995-01-15 10" to "1995-01-15 10:00" for hour precision. Re-inferring
// would clobber the user's "h" pick).
let suppressedCanonical: string | null = null

// Time validator: parses, validates, canonicalizes on blur (!eager).
// While typing (eager) only structural errors surface; canonicalization
// is gated on blur to avoid fighting the user mid-type. The required
// check is also gated on triggered so the field only goes red once the
// user has left the widget.
// eslint-disable-next-line @typescript-eslint/require-await
const timeValidator: ValidatorFn<string> = async (value, options) => {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && !options.initial && trimmed !== model.value) {
      model.value = trimmed
    }
    if (!props.required || options.initial || !triggered.value) return []
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  const normalized = normalizeForParsing(trimmed)
  const validationErrorMessage = progressiveValidate(normalized, t)
  if (validationErrorMessage) {
    // TODO: Use standard codes.
    return [{ code: "invalid", userMessage: validationErrorMessage }]
  }
  if (!options.eager && !options.initial) {
    const struct = getStructuredTime(normalized)
    const inferredPrecision = inferPrecisionFromNormalized(normalized, struct, props.maxPrecision, precision.value)
    const canonical = toCanonicalString(struct, inferredPrecision)
    if (canonical && canonical !== model.value) {
      // Mark the upcoming model change as canonicalization, not user input,
      // so watch(model) does not re-infer precision from it. Without this,
      // hour-precision input "1995-01-15 10" gets rewritten to
      // "1995-01-15 10:00", which the watcher would then re-infer as
      // minute precision, clobbering the user's intent.
      suppressedCanonical = canonical
      model.value = canonical
    }
  }
  return []
}

// Sub-registry: the inner InputText registers here instead of bubbling up
// to the ancestor form. We proxy it upward as a single ValidatedInput
// that combines its dirty/validate state with the precision Listbox's.
// The Listbox is managed manually (it is not a component that registers
// itself, so it sits outside the sub-registry) and composed into the
// outer validatedInput's isDirty / revert / checkpoint paths.
let forwardInteraction: (() => void) | null = null
const {
  validateAll: validateChildAll,
  resetAll: resetChildAll,
  revertAll: revertChildAll,
  inputs: childInputs,
  anyDirty: anyChildDirty,
  checkpointAll: checkpointChildAll,
} = useValidationRegistry(() => {
  forwardInteraction?.()
})

const childErrors = allErrors(childInputs)

watch(childErrors, (v) => emit("errors", v), { flush: "sync" })

// Auto-disarm: once every gated error has cleared (typically because the
// user has fixed the time), drop back into lazy mode so the next round
// behaves like the initial one - no flashes while the user edits.
watch(
  () => childErrors.value.length === 0,
  (cleared) => {
    if (cleared) triggered.value = false
  },
)

// precisionChanged tracks the precision dropdown via its own local checkpoint
// ref. The time and precision are exposed to the parent as a single
// ValidatedInput, so the "changed" badge and revert are whole-input, not per-column.
const precisionCheckpointRef = ref<TimePrecision>(precision.value)
const precisionChanged = computed<boolean>(() => precision.value !== precisionCheckpointRef.value)

// Auto-derive precision from the time whenever the time changes AND the
// time is the authoritative side. The timeAuthoritative guard prevents
// looping with the precision watcher below. The suppressedCanonical
// guard skips the one tick where the change came from the validator's
// canonicalization (which would otherwise re-infer e.g. "min" from
// "1995-01-15 10:00" and clobber the user's "h" pick). The marker is
// cleared unconditionally so it can never linger. If the value the
// watcher fires with does not match (because Vue coalesced this update
// with an unrelated write), the suppression is treated as stale and
// inference runs as usual.
watch(model, (value) => {
  const expected = suppressedCanonical
  suppressedCanonical = null
  if (expected !== null && value === expected) return
  if (!timeAuthoritative.value) return
  if (!value) {
    // The value was cleared: a time with no value is not a valid claim, so the
    // slot must be removed. Reset the leftover precision (auto-inferred, or one
    // the user deliberately picked - it does not matter) to the entry's empty
    // default ("y") so it does not keep the value-less entry comparing non-empty.
    // Otherwise commit() takes the Set/Add path and pushes an invalid, value-less
    // claim to the server instead of the Remove path.
    if (precision.value !== "y") {
      precision.value = "y"
    }
    return
  }
  const normalized = normalizeForParsing(value)
  if (progressiveValidate(normalized, t)) return
  const struct = getStructuredTime(normalized)
  const inferred = inferPrecisionFromNormalized(normalized, struct, props.maxPrecision, precision.value)
  if (inferred !== precision.value) {
    precision.value = inferred
  }
})

// Reformat the time to match the new precision whenever the user is the
// authoritative side for precision. Mirrors NewTime on the backend:
// truncate/pad components to the precision window.
watch(precision, (value) => {
  if (timeAuthoritative.value) return
  if (!model.value) return
  const normalized = normalizeForParsing(model.value)
  if (progressiveValidate(normalized, t)) return
  const struct = getStructuredTime(normalized)
  const next = applyPrecision(struct, value)
  if (next !== model.value) {
    model.value = next
  }
})

// Route user updates through these handlers so we can flip the
// authoritative side. Programmatic mutations (validator canonicalization,
// watcher-driven reformat) bypass these and set the model/precision
// directly, leaving timeAuthoritative as the user last set it.
function onTimeUpdate(v: string) {
  timeAuthoritative.value = true
  model.value = v
}

function onPrecisionSelected(p: TimePrecision) {
  timeAuthoritative.value = false
  precision.value = p
}

// Two columns: the time (which grows to fill, capped since timestamps are
// never that long) and the precision.
const columns = computed<InputColumn[]>(() => [
  { label: t("common.labels.time"), el: () => document.getElementById(timeInputId), width: "24rem" },
  // At full width the track matches the Listbox's natural 12rem; under pressure its
  // cap falls with the container (roughly a 2:1 time:precision split) down to half,
  // with the label inside truncating: the precision prefix alone mostly tells which
  // one it is, so giving the time input the space is the better trade.
  { label: t("common.labels.precision"), el: () => document.getElementById(precisionButtonId), width: "minmax(6rem,min(12rem,33%))" },
])

// Input-format hint.
const hints = computed<string[]>(() => [t("partials.input.InputTime.format")])

// The contents root spanning both columns, used as mainEl and by onFocusOut.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")

const validatedInput: ValidatedInput = {
  validate: async (signal, options) => {
    triggered.value = true
    await validateChildAll(signal, options)
  },
  reset: () => {
    resetChildAll()
    model.value = ""
    precision.value = "y"
    precisionCheckpointRef.value = "y"
    timeAuthoritative.value = true
    timeAuthoritativeCheckpoint.value = true
    triggered.value = false
    suppressedCanonical = null
  },
  revert: () => {
    revertChildAll()
    precision.value = precisionCheckpointRef.value
    timeAuthoritative.value = timeAuthoritativeCheckpoint.value
  },
  // inputEl is the time input - precision is auto-detected by default, so
  // external focus should land on the primary field.
  inputEl: () => document.getElementById(timeInputId),
  // mainEl is the contents root spanning both the time and precision columns,
  // for containment checks.
  mainEl: () => rootRef.value,
  isDirty: computed<boolean>(() => anyChildDirty.value || precisionChanged.value),
  // Empty iff the canonical time is empty. Precision always has a
  // default value, so it never counts as "empty" on its own.
  isEmpty: computed<boolean>(() => !model.value),
  errors: childErrors,
  columns,
  hints,
  checkpoint: () => {
    checkpointChildAll()
    precisionCheckpointRef.value = precision.value
    timeAuthoritativeCheckpoint.value = timeAuthoritative.value
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose(validatedInput)

// Trip the "triggered" gate (and validate) once focus actually leaves
// the whole widget. focusout bubbles, so a single root handler catches
// every internal blur; if the new focus target is still inside us, it
// is just inter-input navigation (time <-> precision) and we skip. A
// null relatedTarget (focus moved to body or a non-focusable element)
// counts as leaving.
async function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  await validatedInput.validate()
}
</script>

<template>
  <!--
    display:contents so the time and precision inputs become direct grid items
    of the enclosing component, each in its own column.
  -->
  <div ref="rootRef" class="pd-inputtime contents" @focusout="onFocusOut">
    <!-- Fall-through attrs (e.g. aria-describedby pointing at InputField's error) go on the time input, the focusable control. -->
    <InputText
      :id="timeInputId"
      v-bind="$attrs"
      :model-value="model"
      :readonly="readonly"
      :invalid="invalid"
      :validator="timeValidator"
      spellcheck="false"
      autocorrect="off"
      autocapitalize="none"
      @update:model-value="onTimeUpdate"
    />

    <Listbox v-model="precision" :disabled="inactive" as="div" class="w-full" @update:model-value="onPrecisionSelected">
      <div class="relative">
        <!--
          We add additional padding on the right (pr-10) on top of InputStyled's
          default px-3 to make space for the icon.
        -->
        <InputStyled :id="precisionButtonId" :as="ListboxButton" :inactive="inactive" :invalid="invalid" class="relative w-full pr-10">
          <div class="truncate" :title="precisionLabel(precision)">{{ precisionLabel(precision) }}</div>

          <div class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
            <ChevronUpDownIcon class="size-5 text-gray-400" aria-hidden="true" />
          </div>
        </InputStyled>

        <ListboxOptions class="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none">
          <ListboxOption v-for="tp in timePrecisionWithMax" :key="tp" v-slot="{ active, selected }" :value="tp" as="template">
            <li class="cursor-pointer p-1 outline-none select-none">
              <!--
                We have an additional div so that the ring has the space to be shown.
                li element has p-1 for ring space, together with py-1 and px-2 we get the effective padding
                for option content of py-2 and px-3, same what InputText and ListboxButton have.
              -->
              <div class="flex flex-row justify-between gap-x-1 rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                <div class="truncate" :class="selected ? 'font-medium' : ''" :title="precisionLabel(tp)">{{ precisionLabel(tp) }}</div>

                <CheckIcon v-if="selected" class="size-5 text-primary-600" aria-hidden="true" />
              </div>
            </li>
          </ListboxOption>
        </ListboxOptions>
      </div>
    </Listbox>
  </div>
</template>
