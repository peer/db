<script setup lang="ts">
import type { ValidatedInput, ValidationError } from "@/types"

import { computed, ref, useTemplateRef, watch } from "vue"

import { useLocked } from "@/progress"
import { useRegisterForValidation, useValidationRegistry } from "@/validation"

const props = withDefaults(
  defineProps<{
    // Minimum number of inputs the user must provide. Rows are NOT
    // required by position. Required is satisfied as long as at least
    // effectiveMin rows are non-empty. When fewer are filled, the deficit
    // is allocated to the first N empty rows in registration order so
    // the user sees exactly which slots need a value.
    min?: number
    // Maximum number of inputs. null = unbounded. We will keep growing
    // the list as long as the user fills the trailing row.
    max?: number | null
    // Bumps effectiveMin to at least 1, so callers can express "at least
    // one value" without having to know about min specifically.
    required?: boolean
    // Presentational override forwarded to every row.
    invalid?: boolean
    // Initial row count override. Use when the caller has external data
    // to pre-populate into the rows (e.g. existing claims): pass the
    // count and the caller will seed each row's models via the
    // useRepeatedInput's setFor as inputs register. Falls back to
    // max(1, effectiveMin) when omitted. Clamped to max.
    initial?: number
  }>(),
  {
    min: 0,
    max: null,
    required: false,
    invalid: false,
    initial: undefined,
  },
)

// The effective minimum: the explicit min, bumped to 1 when required.
const effectiveMin = computed<number>(() => (props.required ? Math.max(props.min, 1) : props.min))

// Reads the ambient lock so we can suppress the missing-required check
// when an ancestor has locked us out (e.g. InputMissing's none/unknown
// checkbox marks the wrapped input as intentionally "no value"). When
// locked the user cannot fill the rows anyway, and the form-level
// semantics is "this field has no value on purpose," so showing red on
// empty rows would be misleading.
const locked = useLocked()

const emit = defineEmits<{ errors: [ValidationError[]] }>()

defineOptions({
  inheritAttrs: false,
})

// Each row is identified by a unique numeric id, used as the v-for key.
// Ids are never reused across the component's lifetime so row identity
// stays stable as rows are added.
let nextId = 0
function newId(): number {
  return nextId++
}

// initialRowCount: the explicit initial prop, or effectiveMin with a floor
// of 1 so the user always has at least one row to type into. We do NOT add
// a trailing row up front - the user has not interacted yet, so an extra
// empty row would just clutter the UI. The trailing row appears the first
// time the user types into the last row (via adjustRows on interaction).
// initial is what callers pass when they have N existing values to seed
// into rows on mount (e.g. FieldsFormField's existing-claim rows).
function initialRowCount(): number {
  let count = Math.max(1, props.initial ?? effectiveMin.value)
  if (props.max !== null) count = Math.min(count, props.max)
  return count
}

const rows = ref<number[]>(Array.from({ length: initialRowCount() }, newId))

// Sub-registry: each row's input registers here instead of bubbling up
// to the ancestor form. We expose ourselves to the ancestor as a single
// ValidatedInput that aggregates all rows.
let forwardInteraction: (() => void) | null = null
const {
  validateAll: validateChildAll,
  resetAll: resetChildAll,
  revertAll: revertChildAll,
  firstEl: firstChildEl,
  inputs: childInputs,
  anyDirty: anyChildDirty,
  allEmpty: allChildEmpty,
  checkpointAll: checkpointChildAll,
} = useValidationRegistry(() => {
  adjustRows()
  forwardInteraction?.()
})

// Registered inputs in document order, so flags/errors align with the
// rendered v-for order regardless of registration order. We sort by DOM
// position rather than trusting Set insertion order because a wrapper
// component between us and the registered input can perturb that order.
const orderedInputs = computed<ValidatedInput[]>(() => {
  return Array.from(childInputs).sort((a, b) => {
    const ea = a.el()
    const eb = b.el()
    if (!ea || !eb) return 0
    const pos = ea.compareDocumentPosition(eb)
    if (pos & Node.DOCUMENT_POSITION_FOLLOWING) return -1
    if (pos & Node.DOCUMENT_POSITION_PRECEDING) return 1
    return 0
  })
})

// Keep rows.length at max(initialRowCount, non_empty + 1), clamped to
// max. Grow and shrink are symmetric around this target:
//
// Grow: when every row has a value, append a fresh empty trailing row
// so the user always has somewhere to enter the next value. Order-
// independent on purpose: it only pushes when there is nothing left to
// fill, so an unfilled slot somewhere keeps us at the current count
// instead of producing an extra trailing slot.
//
// Shrink: while rows.length is above the target, pop the trailing row -
// provided it is empty (we never destroy user data, so a trailing
// non-empty row stops the loop) and not focused (otherwise we would
// yank the user out of the field they are in). Collapses any
// accumulation of trailing empties (and a redundant trailing empty
// when there are empty slots earlier in the list) in a single call.
// We snapshot orderedInputs because childInputs does not synchronously
// reflect a pop - unmount/unregister is async - and track a logical
// len so remaining rows.value[i] still maps to arr[i].
function adjustRows(): void {
  if (props.max === null || rows.value.length < props.max) {
    const arr = orderedInputs.value
    // arr.every on an empty array vacuously returns true, which is the
    // behaviour we want post-removeRow: when every row was a delete-on-empty
    // claim that has been dismissed, we still want one trailing empty row so
    // the user can immediately type a fresh value.
    if (arr.length === rows.value.length && arr.every((i) => !i.isEmpty.value)) {
      rows.value.push(newId())
      return
    }
  }
  const arr = orderedInputs.value
  // Wait for mount/unmount to settle before shrinking; otherwise our
  // index-based reasoning over arr would not match rows.value.
  if (arr.length !== rows.value.length) return
  const nonEmpty = arr.filter((i) => !i.isEmpty.value).length
  let desired = Math.max(initialRowCount(), nonEmpty + 1)
  if (props.max !== null) desired = Math.min(desired, props.max)
  let len = rows.value.length
  while (len > desired) {
    const last = arr[len - 1]
    if (!last.isEmpty.value) break
    const lastEl = last.el()
    const focused = document.activeElement
    if (focused && lastEl?.contains(focused)) break
    rows.value.pop()
    len--
  }
}

// Reactively re-run adjustRows whenever any row's emptiness changes. The
// onInteraction path already covers interactive typing in inputs whose
// isEmpty flips synchronously with model (InputText et al.). But inputs
// like InputHTML expose an isEmpty derived from a separate state ref that
// is updated AFTER useValidation's model watcher fires - so by the time
// onInteraction-driven adjustRows runs, isEmpty is still stale. A direct
// watch on the per-row isEmpty values catches that asynchronous transition
// and grows/shrinks rows correctly.
watch(
  () => orderedInputs.value.map((i) => i.isEmpty.value),
  () => adjustRows(),
  { flush: "post" },
)

// Required-violation computation, aligned with rendered (document) order
// via orderedInputs, so flags[idx] is the violation flag for rows[idx].
// If fewer than effectiveMin rows are non-empty, the deficit is allocated
// to the first N empty rows; those rows render invalid and contribute
// one "required" error each to ourErrors.
const missing = computed<{ flags: boolean[]; ourErrors: ValidationError[] }>(() => {
  if (locked.value) return { flags: [], ourErrors: [] }
  const arr = orderedInputs.value
  let need = effectiveMin.value - arr.filter((i) => !i.isEmpty.value).length
  const flags: boolean[] = []
  const ourErrors: ValidationError[] = []
  for (const input of arr) {
    if (need <= 0 || !input.isEmpty.value) {
      flags.push(false)
      continue
    }
    flags.push(true)
    // TODO: Use standard codes.
    ourErrors.push({ code: "required", el: input.el() ?? undefined })
    need--
  }
  return { flags, ourErrors }
})

// Gate that controls when missing-required is surfaced. Off at mount
// so a freshly-opened form does not yell "required" before the user
// has interacted. Flipped on only when the user blurs out (or submit
// runs validate) AND there is an actual violation, so we never go
// "eager" without an error to display. Auto-clears the moment the user
// resolves every missing slot, so a subsequent empty-while-typing does
// not flash red back at them mid-edit - they have to blur again to
// re-engage the check.
const triggered = ref(false)
watch(
  () => missing.value.ourErrors.length === 0,
  (cleared) => {
    if (cleared) triggered.value = false
  },
)

// Combined error list: own required violations (when triggered) plus
// every row input's own structural errors (decorated with input.el()
// when the validator did not set an el).
const errors = computed<ValidationError[]>(() => {
  const ourErrors = triggered.value ? missing.value.ourErrors : []
  const childErrors: ValidationError[] = []
  for (const input of orderedInputs.value) {
    for (const error of input.errors.value) {
      childErrors.push(error.el ? error : { ...error, el: input.el() ?? undefined })
    }
  }
  return [...ourErrors, ...childErrors]
})

watch(errors, (v) => emit("errors", v), { flush: "sync" })

// Checkpoint row count for our own dirty tracking. Each row's input
// keeps its own checkpoint through the sub-registry.
const checkpointRowCount = ref<number>(rows.value.length)

const validatedInput: ValidatedInput = {
  validate: async (signal) => {
    await validateChildAll(signal)
    if (missing.value.ourErrors.length > 0) {
      triggered.value = true
    }
  },
  reset: () => {
    resetChildAll()
    rows.value = Array.from({ length: initialRowCount() }, newId)
    triggered.value = false
  },
  revert: () => {
    revertChildAll()
    // Trim/grow to the checkpoint length. Existing rows up to the
    // checkpoint length keep their (now-reverted) inputs; any rows beyond
    // are dropped, and any missing ones are re-issued with fresh ids.
    if (rows.value.length > checkpointRowCount.value) {
      rows.value = rows.value.slice(0, checkpointRowCount.value)
    } else {
      while (rows.value.length < checkpointRowCount.value) {
        rows.value.push(newId())
      }
    }
  },
  el: firstChildEl,
  // Dirty state is driven purely by the rows' own inputs. We deliberately
  // do NOT also compare rows.length against the checkpoint, because seed-
  // driven setups (e.g. FieldsFormField pre-populating N existing values)
  // trigger adjustRows to auto-push a trailing empty during mount; that
  // bumps rows.length to N+1 while checkpointRowCount stays at the
  // initial-prop's N, which would falsely mark the cardinality dirty from
  // the start. anyChildDirty already captures every user-driven change
  // because the user cannot add or remove a row without typing something
  // into it (or clearing something previously typed), and clearing is
  // exactly what flips an inner input to dirty.
  isDirty: anyChildDirty,
  isEmpty: allChildEmpty,
  errors,
  checkpoint: () => {
    checkpointRowCount.value = rows.value.length
    checkpointChildAll()
  },
}

const { onInteraction: notifyOuter } = useRegisterForValidation(validatedInput)
forwardInteraction = notifyOuter

// removeRow drops the row whose input identity matches `input` from the
// rows array. Used by callers that map rows to external identity (e.g. a
// claim ID) and want a full row removal when the underlying value is
// deleted - the auto-shrink logic only pops empty trailing rows, so a
// middle row that the user emptied would otherwise stay behind.
function removeRow(input: ValidatedInput): void {
  const idx = orderedInputs.value.indexOf(input)
  if (idx === -1) return
  rows.value.splice(idx, 1)
}

// Expose the registered child inputs (in DOM order) alongside the standard
// ValidatedInput so a caller can pair each row with external metadata, e.g.
// a claim ID, that the cardinality itself does not know about. removeRow
// lets the caller dismiss a specific row after committing its deletion.
defineExpose({
  ...validatedInput,
  inputs: orderedInputs,
  removeRow,
})

// Trigger the required check when focus leaves the entire
// InputCardinality (the rows plus anything else inside us). focusout
// bubbles, so a single root handler catches all internal blurs; if the
// new focus target is still inside us, this is just inter-row navigation
// and we skip. A null relatedTarget (focus moved to body or a
// non-focusable element) is treated as leaving.
const rootRef = useTemplateRef<HTMLDivElement>("rootRef")
function onFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  if (next && rootRef.value?.contains(next)) return
  if (missing.value.ourErrors.length > 0) {
    triggered.value = true
  }
}
</script>

<template>
  <div ref="rootRef" class="flex min-w-0 grow flex-col gap-y-2" @focusout="onFocusOut">
    <template v-for="(id, idx) in rows" :key="id">
      <!--
        $attrs is forwarded so things like aria-describedby flow to
        each row's input. We do NOT pass per-row "required" - the
        required check is at the container level (count of non-empty
        vs effectiveMin), not per position. The slot's "invalid"
        combines the explicit invalid prop (forces invalid on every
        row) with the per-row violation flag from missing.flags (the
        first N empty rows when below effectiveMin), so the wrapped
        input renders red on exactly the rows that count toward the
        deficit.

        ":input" exposes the row's registered ValidatedInput so the
        slot consumer can pair it with useRepeatedInput's modelFor.
        Until the row's wrapped input has mounted and registered,
        this is null - modelFor handles that as a no-op. The consumer
        should destructure ({ input, ...rest }) so it does not flow
        to DOM via v-bind="cardinalityProps".
      -->
      <slot v-bind="$attrs" :invalid="invalid || (triggered && missing.flags[idx]) || false" :input="orderedInputs[idx] ?? null" />
    </template>
  </div>
</template>
