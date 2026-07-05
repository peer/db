<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { ShallowUnwrapRef } from "vue"

import type { InputColumn, ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import { computed, ref, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"

import InputText from "@/components/InputText.vue"
import { allErrors, useRegisterForValidation, useValidationRegistry } from "@/validation"

// Mirrors document.go's amountRegex: optional sign, digits, optional
// dot- or comma-separated decimal part. The same shape the backend
// accepts in UnmarshalText, so anything that passes here will pass the
// server-side Validate(0) check too.
const AMOUNT_RE = /^(-?\d+)(?:[.,](\d+))?$/

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
  },
)

// Two v-models: the amount string and the precision string. Both are
// strings on the wire to preserve the user's typed form. Numeric
// parsing happens on the backend (Amount.Float64). The precision
// is stored as a decimal number, e.g. "1", "0.1", "0.01".
const model = defineModel<string>({ default: "" })
const precision = defineModel<string>("precision", { default: "" })

defineOptions({
  inheritAttrs: false,
})

const emit = defineEmits<{ errors: [ValidationError[]] }>()

const { t } = useI18n({ useScope: "global" })

const amountInputId = useId()
const precisionInputId = useId()

// True while the amount drives precision auto-detection. Flips off
// when the user edits precision directly, and back on when the user
// edits the amount, so the most recently touched side is the
// authoritative one and the other follows.
const amountAuthoritative = ref(true)
const amountAuthoritativeCheckpoint = ref(amountAuthoritative.value)

// Gate: the validators surface required / requiredPrecision only after
// the user has left the whole widget at least once (focusout to outside,
// or an external validate via parent submit). Individual input blurs
// while tabbing between amount and precision do NOT trip this, so the
// pair only goes red once the user is really done editing the field.
// Reset clears the gate so a cancelled form starts fresh.
const triggered = ref(false)

// parseAmount accepts dot or comma as the decimal separator (matching
// the backend regex) and returns the canonical dot-separated form plus
// the count of decimal digits. Returns null when the input does not
// parse as an amount.
function parseAmount(raw: string): { canonical: string; decimals: number } | null {
  const m = AMOUNT_RE.exec(raw)
  if (!m) return null
  const decimals = m[2]?.length ?? 0
  const canonical = decimals === 0 ? m[1] : m[1] + "." + m[2]
  return { canonical, decimals }
}

// Number of decimal digits a precision should produce, matching the
// backend: 0 for precision >= 1, else ceil(-log10(precision)).
function expectedDecimals(p: number): number {
  if (p >= 1) return 0
  return Math.ceil(-Math.log10(p))
}

// Canonical precision string for decimals digits in the amount. "1"
// for 0 decimals (whole numbers), "0.1" for 1, "0.01" for 2, etc.
function detectedPrecisionString(decimals: number): string {
  if (decimals === 0) return "1"
  return Math.pow(0.1, decimals).toFixed(decimals)
}

// Parses a precision string into a positive finite number, or null if
// the value cannot be a valid precision.
function parsePrecision(raw: string): number | null {
  const n = parseFloat(raw)
  if (!isFinite(n) || n <= 0) return null
  return n
}

// applyPrecision rounds amount to precision and reformats it with
// the matching number of decimal digits, mirroring NewAmount on the
// backend (including the negative-zero strip).
function applyPrecision(amount: string, p: number): string {
  const parsed = parseAmount(amount)
  if (!parsed) return amount
  const num = parseFloat(parsed.canonical)
  if (!isFinite(num)) return amount
  const rounded = Math.round(num / p) * p
  const decimals = expectedDecimals(p)
  let s = rounded.toFixed(decimals)
  if (rounded === 0 && s.startsWith("-")) s = s.slice(1)
  return s
}

// Amount validator: trim, regex-check, canonicalize on blur. Owns the
// required-when-empty check since the precision side has no separate
// required signal. The "required" return is gated on triggered so it
// only surfaces after the widget has been blurred out once. Structural
// errors ("invalid") fire immediately regardless.
// eslint-disable-next-line @typescript-eslint/require-await
const amountValidator: ValidatorFn<string> = async (value, options) => {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && !options.initial && trimmed !== model.value) {
      model.value = trimmed
    }
    if (!props.required || options.initial || !triggered.value) return []
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  const parsed = parseAmount(trimmed)
  if (!parsed) {
    // TODO: Use standard codes.
    return [{ code: "invalid" }]
  }
  if (!options.eager && !options.initial && parsed.canonical !== model.value) {
    model.value = parsed.canonical
  }
  return []
}

// Precision validator: precision is required whenever amount is in a
// "should have a value" state - either parseably non-empty (then a
// missing precision is a cross-field "requiredPrecision" violation),
// or empty while amount itself is required (then the pair is the
// "missing value" picture and precision should turn red alongside
// amount). The only empty-precision case that does NOT complain is
// when amount holds an unparseable string ("abc"): amount has its own
// "Invalid value." problem and piling a precision error on top would
// just blame the wrong side while the user is fixing amount.
// Non-empty must parse as a positive finite number.
// eslint-disable-next-line @typescript-eslint/require-await
const precisionValidator: ValidatorFn<string> = async (value, options) => {
  const trimmed = value.trim()
  if (trimmed === "") {
    if (!options.eager && !options.initial && trimmed !== precision.value) {
      precision.value = trimmed
    }
    if (options.initial || !triggered.value) return []
    const amountTrimmed = model.value.trim()
    if (amountTrimmed === "") {
      // Both fields empty. When amount is required, precision is
      // part of the same missing pair - return "required" so the
      // input renders red. InputErrors's codeMap order ensures the
      // single message displayed is amount's "Required value."
      // rather than a duplicated precision-specific one.
      if (props.required) {
        // TODO: Use standard codes.
        return [{ code: "required" }]
      }
      return []
    }
    if (AMOUNT_RE.test(amountTrimmed)) {
      // TODO: Use standard codes.
      return [{ code: "requiredPrecision" }]
    }
    return []
  }
  const p = parsePrecision(trimmed)
  if (p === null) {
    // TODO: Use standard codes.
    return [{ code: "invalidPrecision" }]
  }
  // Re-emit the parseFloat form so e.g. ",1" becomes "0.1" and trailing
  // junk past the number is stripped.
  const canonical = String(p)
  if (!options.eager && !options.initial && canonical !== precision.value) {
    precision.value = canonical
  }
  return []
}

// Sub-registry: the two inner InputTexts register here instead of
// bubbling up to the ancestor form. We proxy them upward as a single
// ValidatedInput.
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
// user has fixed amount and precision), drop back into lazy mode so the
// next round behaves like the initial one - no flashes while the user
// edits, validation only resurfaces on the next blur-out of the widget.
watch(
  () => childErrors.value.length === 0,
  (cleared) => {
    if (cleared) triggered.value = false
  },
)

// Per-field template refs back the cross-field validation below. The amount
// and precision are exposed to the parent as a single ValidatedInput, so the
// "changed" badge and revert are whole-input, not per-column.
const amountRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("amountRef")
const precisionRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("precisionRef")

// The contents root spanning both columns, used as mainEl and by onFocusOut.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")

// Auto-detect precision from the amount whenever the amount changes
// AND the amount is the authoritative side. The amountAuthoritative
// guard prevents looping with the precision watcher below.
watch(model, (value) => {
  if (!amountAuthoritative.value) return
  if (value.trim() === "") {
    // The amount was cleared: an amount with no value is not a valid claim, so the
    // slot must be removed. Reset the leftover precision (auto-detected, or one the
    // user deliberately picked - it does not matter) to the entry's empty default
    // ("") so it does not keep the value-less entry comparing non-empty. Otherwise
    // commit() takes the Set/Add path and pushes an invalid, value-less claim to
    // the server instead of the Remove path. An unparseable amount ("abc") is not
    // empty, so it falls through and keeps its precision.
    if (precision.value !== "") {
      precision.value = ""
    }
    return
  }
  const parsed = parseAmount(value.trim())
  if (!parsed) return
  const newPrecision = detectedPrecisionString(parsed.decimals)
  if (newPrecision !== precision.value) {
    precision.value = newPrecision
  }
})

// Reformat the amount to match the new precision whenever the user is
// the authoritative side for precision. Mirrors NewAmount: round and
// format with the right number of decimals.
watch(precision, (value) => {
  if (amountAuthoritative.value) return
  const p = parsePrecision(value.trim())
  if (p === null) return
  const next = applyPrecision(model.value, p)
  if (next !== model.value) {
    model.value = next
  }
})

// Cross-field re-validation: the precision rule depends on three signals
// outside precision itself - amount's emptiness, amount's validity
// (AMOUNT_RE) when non-empty, and whether amount has surfaced its own
// "required" error. The precision input only re-runs its own validator
// when precision changes, so we watch each of those signals and force
// a precision re-validation whenever they flip. In particular, the
// third source means that when the user blurs amount with both fields
// empty, amountValidator sets amount's "required" error and the
// resulting flip drives precision through its empty branch to mirror
// the missing-pair state.
watch(
  // TODO: Use standard codes.
  [() => model.value.trim() === "", () => AMOUNT_RE.test(model.value.trim()), () => (amountRef.value?.errors ?? []).some((e: ValidationError) => e.code === "required")],
  async () => {
    await precisionRef.value?.validate()
  },
  // The validator canonicalizes the precision model from the inner input's value, so it
  // must not run before that input has re-rendered: at "pre" timing an external update
  // which sets amount and precision together (a claim arriving from another editor)
  // would validate against the inner input's stale empty value and write it back,
  // wiping the just-arrived precision.
  { flush: "post" },
)

// Route user updates through these handlers so we can flip the
// authoritative side. Programmatic mutations (validator side effects,
// watcher-driven reformat) also emit update:modelValue, but those go
// through the same path - in those cases the flag is already in the
// correct state for the side that triggered the mutation.
function onAmountUpdate(v: string) {
  amountAuthoritative.value = true
  model.value = v
}

function onPrecisionUpdate(v: string) {
  amountAuthoritative.value = false
  precision.value = v
}

// Two columns: the amount (which grows to fill, capped since numbers are never
// that long) and the precision.
const columns = computed<InputColumn[]>(() => [
  { label: t("common.labels.amount"), el: () => document.getElementById(amountInputId), width: "24rem" },
  { label: t("common.labels.precision"), el: () => document.getElementById(precisionInputId) },
])

const validatedInput: ValidatedInput = {
  validate: async (signal, options) => {
    triggered.value = true
    await validateChildAll(signal, options)
  },
  reset: () => {
    resetChildAll()
    model.value = ""
    precision.value = ""
    amountAuthoritative.value = true
    amountAuthoritativeCheckpoint.value = true
    triggered.value = false
  },
  revert: () => {
    revertChildAll()
    amountAuthoritative.value = amountAuthoritativeCheckpoint.value
  },
  // inputEl is the amount input - precision is secondary and auto-detected
  // by default, so external focus should land on the primary field.
  inputEl: () => document.getElementById(amountInputId),
  // mainEl is the contents root spanning both the amount and precision
  // columns, for containment checks.
  mainEl: () => rootRef.value,
  isDirty: anyChildDirty,
  // Empty for InputAmount means the user has not provided an amount
  // string. "0" is intentionally NOT empty - it is a valid value.
  isEmpty: computed<boolean>(() => !model.value),
  errors: childErrors,
  columns,
  checkpoint: () => {
    checkpointChildAll()
    amountAuthoritativeCheckpoint.value = amountAuthoritative.value
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

defineExpose(validatedInput)

// Trip the "triggered" gate (and validate) once focus actually leaves
// the whole widget. focusout bubbles, so a single root handler catches
// every internal blur; if the new focus target is still inside us, it
// is just inter-input navigation (amount <-> precision) and we skip.
// A null relatedTarget (focus moved to body or a non-focusable element)
// counts as leaving.
async function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  await validatedInput.validate()
}
</script>

<template>
  <!--
    display:contents so the amount and precision inputs become direct grid items
    of the enclosing component, each in its own column.
  -->
  <div ref="rootRef" class="pd-inputamount contents" @focusout="onFocusOut">
    <!-- Fall-through attrs (e.g. aria-describedby pointing at InputField's error) go on the primary input, the focusable control. -->
    <InputText
      :id="amountInputId"
      ref="amountRef"
      v-bind="$attrs"
      :model-value="model"
      :readonly="readonly"
      :invalid="invalid"
      :validator="amountValidator"
      spellcheck="false"
      autocorrect="off"
      autocapitalize="none"
      @update:model-value="onAmountUpdate"
    />

    <!--
      size="1" collapses the <input>'s intrinsic max-content so it does not drag
      the precision column wide; min-w-24 then sets the actual width, enough for
      typical precisions like 0.0001 without ellipsis.
    -->
    <InputText
      :id="precisionInputId"
      ref="precisionRef"
      :model-value="precision"
      :readonly="readonly"
      :invalid="invalid"
      :validator="precisionValidator"
      spellcheck="false"
      autocorrect="off"
      autocapitalize="none"
      size="1"
      class="min-w-24"
      @update:model-value="onPrecisionUpdate"
    />
  </div>
</template>
