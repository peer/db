import type { ValidatedInput } from "@/types"

import { shallowReactive } from "vue"

type RepeatedInputOptions<T> = { default?: T }

// v-model binding shape that modelFor returns. For a literal N (e.g.
// "modelValue" or "precision") it resolves to the precise pair of
// keys defineModel<N>() expects on the wrapped input - both the prop
// (T) and the event listener ((v: T) => void). When N is the
// unconstrained string, which happens only inside this function's
// impl signature (the public overloads always pin N to a specific
// literal, so callers never see this branch), it relaxes to a Record
// so the impl can construct the dynamically-keyed object without
// per-key type gymnastics.
type ModelBinding<T, N extends string> = string extends N ? Record<string, T | ((v: T) => void)> : { [K in N]: T } & { [K in `onUpdate:${N}`]: (v: T) => void }

export type RepeatedInput<T, N extends string = string> = {
  // The model name (used also as a key in combineRepeatedInputs output).
  name: N
  // Compacted, non-empty values in DOM order.
  values: () => T[]
  // Same as values, but each entry paired with its owning input. Useful
  // when callers need to zip by input identity rather than by index.
  entries: () => [ValidatedInput, T][]
  // v-model binding object to be spread onto the wrapped input via
  // v-bind. The key shape (name vs onUpdate:name) matches the slot
  // input's declared v-model.
  modelFor: (input: ValidatedInput | null | undefined) => ModelBinding<T, N>
  // Reads the model's value for the given input. Returns the default
  // when the input is empty (consistent with values and entries,
  // which filter empty inputs out); otherwise the stored value, or
  // the default if nothing has been stored (yet) for the input.
  valueFor: (input: ValidatedInput) => T
}

// useRepeatedInput maintains a per-input value store for ONE model on a
// repeated input (e.g. each row of an InputCardinality). The slot
// consumer calls modelFor(input) and spreads the result onto the wrapped
// input alongside v-bind="...rest". The returned object's shape matches
// the v-model the slot's input declares (modelValue + onUpdate:modelValue
// by default, or <name> + onUpdate:<name> when the model is named, the
// same way defineModel works).
//
// Arguments mirror defineModel: an optional name followed by an options
// object with { default }.
//
//   useRepeatedInput<T>()                          // default v-model
//   useRepeatedInput<T>({ default: ... })          // default with options
//   useRepeatedInput<T>("name")                    // named v-model
//   useRepeatedInput<T>("name", { default: ... })  // named with options
//
// values returns a compacted view containing only the rows where
// input.isEmpty.value is false, sorted by the DOM position of each
// input's el(). entries is the same list but paired with the owning
// input. valueFor(input) reads the model's value for a single input.
// Used by combineRepeatedInputs to fetch the non-primary models for
// each retained row.
//
// To collect multiple models from the same row (e.g. an InputTime's
// time + precision), instantiate one useRepeatedInput per model. They
// stay in sync because both filter by the same input.isEmpty, so
// pair-wise indexing reconstructs the original tuples; combineRepeatedInputs
// does this for you and keys each tuple field by the model name passed
// to useRepeatedInput.
//
// Usage:
//
//   const time = useRepeatedInput<string>({ default: "" })
//   const precision = useRepeatedInput<TimePrecision>("precision", { default: "y" })
//
//   <InputCardinality>
//     <template #default="{ input, ...rest }">
//       <InputTime
//         v-bind="rest"
//         v-bind="time.modelFor(input)"
//         v-bind="precision.modelFor(input)"
//       />
//     </template>
//   </InputCardinality>
//
//   const claims = combineRepeatedInputs(time, precision)
//   // claims: [{ modelValue: "2024", precision: "y" }, ...]
export function useRepeatedInput<T>(options?: RepeatedInputOptions<T>): RepeatedInput<T, "modelValue">
export function useRepeatedInput<T, N extends string>(name: N, options?: RepeatedInputOptions<T>): RepeatedInput<T, N>
export function useRepeatedInput<T>(nameOrOptions?: string | RepeatedInputOptions<T>, maybeOptions?: RepeatedInputOptions<T>): RepeatedInput<T, string> {
  let name = "modelValue"
  let options: RepeatedInputOptions<T> | undefined
  if (typeof nameOrOptions === "string") {
    name = nameOrOptions
    options = maybeOptions
  } else {
    options = nameOrOptions
  }
  const initial = options?.default as T
  const eventKey = `onUpdate:${name}`

  const store = shallowReactive(new Map<ValidatedInput, T>())

  function stored(input: ValidatedInput): T {
    if (!store.has(input)) return initial
    return store.get(input) as T
  }

  function modelFor(input: ValidatedInput | null | undefined): ModelBinding<T, string> {
    if (!input) {
      // Pre-registration window: the row's wrapped input has mounted
      // but its ValidatedInput has not registered yet. defineModel
      // initializes with initial and the user cannot interact before
      // registration completes, so a no-op onUpdate here is safe.
      return { [name]: initial, [eventKey]: () => {} }
    }
    return {
      [name]: stored(input),
      [eventKey]: (v: T) => {
        store.set(input, v)
      },
    }
  }

  function valueFor(input: ValidatedInput): T {
    // When called by combineRepeatedInputs, isEmpty should be in sync across models
    // for the same input, but just in case and to support other callers, we check.
    if (input.isEmpty.value) return initial
    return stored(input)
  }

  function entries(): [ValidatedInput, T][] {
    const list: [ValidatedInput, T][] = []
    for (const [input, value] of store) {
      if (input.isEmpty.value) continue
      list.push([input, value])
    }
    list.sort(([a], [b]) => {
      const ea = a.el()
      const eb = b.el()
      if (!ea || !eb) return 0
      const pos = ea.compareDocumentPosition(eb)
      if (pos & Node.DOCUMENT_POSITION_FOLLOWING) return -1
      if (pos & Node.DOCUMENT_POSITION_PRECEDING) return 1
      return 0
    })
    return list
  }

  function values(): T[] {
    return entries().map(([, v]) => v)
  }

  return { name, values, entries, modelFor, valueFor }
}

// Combined row: one record per non-empty primary entry, keyed by each
// participating useRepeatedInput's name.
type CombinedRow<R extends readonly RepeatedInput<unknown, string>[]> = {
  [I in keyof R as R[I] extends RepeatedInput<unknown, infer N> ? N : never]: R[I] extends RepeatedInput<infer V, string> ? V : never
}

// combineRepeatedInputs joins multiple useRepeatedInput instances into
// a single array of records, keyed by each instance's name. Row
// ordering is driven by the FIRST argument's entries (which are
// filtered by isEmpty and sorted by DOM order); for each retained
// input, the other repeated inputs' values are looked up by input
// identity via valueFor, so a model that never emitted (and thus has
// no stored value) falls back to its default rather than dropping the
// row.
//
// Useful when a row carries multiple models that should be
// reconstructed together (e.g. InputTime's time + precision):
//
//   const time = useRepeatedInput<string>({ default: "" })
//   const precision = useRepeatedInput<TimePrecision>("precision", { default: "y" })
//   const claims = combineRepeatedInputs(time, precision)
//   // claims: [{ modelValue: "2024", precision: "y" }, ...]
export function combineRepeatedInputs<R extends readonly RepeatedInput<unknown, string>[]>(...inputs: R): CombinedRow<R>[] {
  if (inputs.length === 0) return [] as CombinedRow<R>[]
  const [primary, ...rest] = inputs
  const result: Record<string, unknown>[] = []
  for (const [input, primaryValue] of primary.entries()) {
    const row: Record<string, unknown> = { [primary.name]: primaryValue }
    for (const r of rest) {
      row[r.name] = r.valueFor(input)
    }
    result.push(row)
  }
  return result as CombinedRow<R>[]
}
