import type { ComputedRef, InjectionKey, Ref } from "vue"

import type { ValidatedInput, ValidateFn, ValidationError, ValidatorFn } from "@/types"

import { computed, inject, onBeforeUnmount, onMounted, provide, ref, shallowReactive, shallowReadonly, watch } from "vue"

import { anySignal, equals, raceWithSignal } from "@/utils"

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
//
// registerForValidationKey returns a registration callback which in turn, when an
// input registers, returns an input-scoped notifier the input can call when the user
// interacts with it. The registry forwards that notification to its onInteraction
// handler with the input's identity.
export const registerForValidationKey: InjectionKey<(instance: ValidatedInput) => () => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-validation-register") : Symbol()
export const unregisterForValidationKey: InjectionKey<(instance: ValidatedInput) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-validation-unregister") : Symbol()

// useRegisterForValidation is called by an input to make itself discoverable
// by the nearest ancestor (one that called useValidationRegistry). It is
// a no-op when there is no such ancestor, so inputs can be used without them.
//
// The returned onInteraction callback should be called when the user interacts
// with this input.
export function useRegisterForValidation(input: ValidatedInput): {
  onInteraction: () => void
} {
  const register = inject(registerForValidationKey, null)
  const unregister = inject(unregisterForValidationKey, null)
  // The notifier is returned by register at mount time. Capture it in a
  // closure variable so calls before mount (or after unmount) are no-ops.
  let notify: (() => void) | null = null
  onMounted(() => {
    notify = register?.(input) ?? null
  })
  onBeforeUnmount(() => {
    unregister?.(input)
    notify = null
  })
  return {
    onInteraction: () => {
      notify?.()
    },
  }
}

// allErrors builds a flat, decorated list of every input's current
// errors. Each error keeps its own el if the validator set one. The rest
// are filled in with the source input's el().
//
// Pass an iterable that is reactive on membership and per-input errors
// (e.g. a useValidationRegistry's inputs) so the computed updates.
export function allErrors(inputs: Iterable<ValidatedInput>): ComputedRef<ValidationError[]> {
  return computed(() => {
    const result: ValidationError[] = []
    for (const input of inputs) {
      for (const error of input.errors.value) {
        result.push(error.el ? error : { ...error, el: input.el() ?? undefined })
      }
    }
    return result
  })
}

// useValidationRegistry is called to collect validated inputs from all
// descendant inputs that called useRegisterForValidation. validateAll runs
// every input's validator in parallel and returns the flat list of errors.
// resetAll restores every registered input to its initial state, revertAll
// restores every registered input to its recorded baseline.
//
// onInteraction is called whenever a registered input notifies that the user
// has interacted with it (e.g. to clear top-level errors). The triggering
// input is passed through so callers can react per-input if needed.
//
// Validation registries nest transparently: if el getter is provided,
// the registry self-registers as a ValidatedInput, so an outer validation
// registry sees inner one as a single input whose validate is its validateAll,
// whose reset is its resetAll and onInteraction callbacks bubble up.
export function useValidationRegistry(
  onInteraction?: (input: ValidatedInput) => void,
  el?: () => HTMLElement | null,
): {
  validateAll: ValidateFn
  resetAll: () => void
  revertAll: () => void
  snapshotBaselines: () => void
  firstEl: () => HTMLElement | null
  // Read-only view over the registered inputs.
  inputs: ReadonlySet<ValidatedInput>
  anyDirty: Readonly<Ref<boolean>>
  allEmpty: Readonly<Ref<boolean>>
  anyError: Readonly<Ref<boolean>>
} {
  // shallow-reactive Set so iteration inside computeds (e.g. anyDirty)
  // registers membership as a dependency and add/delete trigger
  // re-evaluation when inputs mount/unmount across tab switches. Shallow
  // because deep reactive would unwrap nested refs (e.g. input.isDirty,
  // which is a Ref<boolean>) on access, breaking the type.
  const inputs = shallowReactive(new Set<ValidatedInput>())

  const validateAll: ValidateFn = async function (signal?: AbortSignal): Promise<ValidationError[]> {
    const list = Array.from(inputs)
    try {
      const batches = await Promise.all(list.map((input) => input.validate(signal)))
      return batches.flatMap((errors, i) => errors.map((error) => (error.el ? error : { ...error, el: list[i].el() ?? undefined })))
    } catch (err) {
      if (signal?.aborted) {
        return []
      }
      throw err
    }
  }

  function resetAll(): void {
    for (const input of inputs) {
      input.reset()
    }
  }

  function revertAll(): void {
    for (const input of inputs) {
      input.revert()
    }
  }

  const allEmpty = computed<boolean>(() => {
    for (const input of inputs) {
      if (!input.isEmpty.value) return false
    }
    return true
  })

  function snapshotBaselines(): void {
    for (const input of inputs) {
      input.setBaseline()
    }
  }

  const anyDirty = computed<boolean>(() => {
    for (const input of inputs) {
      if (input.isDirty.value) return true
    }
    return false
  })

  const anyError = computed<boolean>(() => {
    for (const input of inputs) {
      if (input.errors.value.length > 0) return true
    }
    return false
  })

  // When el is provided, self-register so this sub-registry appears as one
  // ValidatedInput in the outer registry (its validate/reset combine its
  // descendants', its onInteraction notifier forwards inner interactions
  // upward, its isDirty/setBaseline cascade through anyDirty/snapshotBaselines).
  // In sink mode (no el), descendant interactions do not bubble out of this
  // registry automatically - if the caller still wants forwarding, they
  // register manually and call the returned onInteraction themselves.
  let notifyUp: (() => void) | null = null
  if (el) {
    const { onInteraction: up } = useRegisterForValidation({
      el,
      validate: validateAll,
      reset: resetAll,
      revert: revertAll,
      isDirty: anyDirty,
      isEmpty: allEmpty,
      errors: allErrors(inputs),
      setBaseline: snapshotBaselines,
    })
    notifyUp = up
  }

  provide(registerForValidationKey, (input: ValidatedInput) => {
    inputs.add(input)
    return () => {
      onInteraction?.(input)
      notifyUp?.()
    }
  })
  provide(unregisterForValidationKey, (input: ValidatedInput) => {
    inputs.delete(input)
  })

  // firstEl is the earliest focusable element across registered inputs.
  function firstEl(): HTMLElement | null {
    return pickEarliestFocusable(Array.from(inputs, (i) => i.el()))
  }

  return { validateAll, resetAll, revertAll, snapshotBaselines, firstEl, inputs: shallowReadonly(inputs), anyDirty, allEmpty, anyError }
}

// isFocusable returns true if calling .focus() on el can meaningfully move
// keyboard focus to it. Disabled form controls (input/button/select/
// textarea/fieldset) silently swallow .focus(), so we skip them and try
// the next candidate instead.
function isFocusable(el: HTMLElement): boolean {
  if ("disabled" in el && Boolean((el as HTMLInputElement).disabled)) {
    return false
  }
  return true
}

// pickEarliestFocusable returns the element from els that appears earliest
// in document order and is focusable. Used by focusFirstInvalid and
// focusFirstInput. Pairs that compareDocumentPosition reports as
// disconnected or identical leave the running winner unchanged.
function pickEarliestFocusable(els: Iterable<HTMLElement | null | undefined>): HTMLElement | null {
  let earliest: HTMLElement | null = null
  for (const el of els) {
    if (!el || !isFocusable(el)) {
      continue
    }
    if (!earliest) {
      earliest = el
      continue
    }
    if (earliest.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_PRECEDING) {
      earliest = el
    }
  }
  return earliest
}

// focusFirstInvalid focuses the error whose el appears earliest in the
// document and is focusable. Errors without an el, or with an el that is
// disabled, are skipped.
export function focusFirstInvalid(errors: ValidationError[]) {
  pickEarliestFocusable(errors.map((e) => e.el))?.focus()
}

class ValidationAbortedError extends Error {}

// useValidation is the high-level wrapper for the common "reactive ref +
// ValidatorFn" shape of input. On top of useRegisterForValidation it owns the
// validation machinery around the validator (abort-and-restart on every new
// call, mode-aware caching for eager/initial, in-flight join, model-mutation
// re-validation loop), watches model and writes results into errors.value,
// and triggers onInteraction on every model change.
//
// Use useValidation for simple inputs with a single ValidatorFn<T> over one
// model ref. For composite inputs whose validate/reset do not fit that shape,
// for example inputs aggregating a sub-registry's validateAll/resetAll, or
// inputs that have no validator at all, drop down to useRegisterForValidation
// directly.
export function useValidation<T>(
  model: Ref<T>,
  errors: Ref<ValidationError[]>,
  lock: Ref<number>,
  validatorGetter: () => ValidatorFn<T> | undefined,
  el: () => HTMLElement | null,
  reset: () => void,
  // Optional custom emptiness ref. When provided it is exposed as the
  // validated input's isEmpty. Otherwise !model.value is used.
  isEmpty?: Readonly<Ref<boolean>> | null,
): {
  runValidation: (options?: { signal?: AbortSignal; eager?: boolean; initial?: boolean }) => Promise<void>
  validatedInput: ValidatedInput
} {
  let validateAbortController: AbortController | null = null
  let inFlight: { value: T; validator: ValidatorFn<T>; eager: boolean; initial: boolean; promise: Promise<void> } | null = null
  let lastValidated: { value: T; validator: ValidatorFn<T>; eager: boolean; initial: boolean } | null = null

  onBeforeUnmount(() => {
    validateAbortController?.abort()
  })

  // Only treat an entry as covering a request when the value, validator, and
  // mode flags all match. We don't assume a result from one mode is strictly
  // stronger than another, because a validator's behavior under each mode is
  // opaque to us; any mode mismatch always re-runs.
  function entryCovers(
    entry: { value: T; validator: ValidatorFn<T>; eager: boolean; initial: boolean } | null,
    value: T,
    validator: ValidatorFn<T>,
    eager: boolean,
    initial: boolean,
  ): boolean {
    if (!entry) return false
    return entry.value === value && entry.validator === validator && entry.eager === eager && entry.initial === initial
  }

  // internalValidation uses abort-and-restart: every call aborts any prior in-flight
  // one and starts a new validator invocation. On successful completion it writes the
  // result to errors.value and records lastValidated as a cache marker. On abort the
  // IIFE throws ValidationAbortedError so callers awaiting inFlight.promise can
  // distinguish validator aborts from real validator errors.
  function internalValidation(options?: { signal?: AbortSignal; eager?: boolean; initial?: boolean }): Promise<void> | null {
    const validator = validatorGetter()
    if (!validator) return null
    const initialValue = model.value
    const eager = options?.eager ?? false
    const initial = options?.initial ?? false

    // Already have a result for this exact (value, validator, mode) in errors.value.
    if (entryCovers(lastValidated, initialValue, validator, eager, initial)) {
      return null
    }
    // Already running the validator for this exact (value, validator, mode): join the in-flight call.
    if (entryCovers(inFlight, initialValue, validator, eager, initial)) {
      // The non-null assertion is safe: entryCovers returns false when entry is null.
      return inFlight!.promise
    }

    validateAbortController?.abort()
    validateAbortController = new AbortController()
    const additionalSignal = options?.signal
    const signal = additionalSignal ? anySignal(validateAbortController.signal, additionalSignal) : validateAbortController.signal

    // Pre-declared so the IIFE's finally can compare inFlight.promise against
    // its own promise to decide whether the cleanup is still relevant. The
    // assertion is safe because the IIFE only reads promise after its first
    // await, by which time the assignment below has run.
    let promise!: Promise<void>
    // eslint-disable-next-line prefer-const
    promise = (async (): Promise<void> => {
      lock.value++
      try {
        let value = initialValue
        // Validators may mutate model.value as a side effect (ideally gated on
        // !eager && !initial). If that happens, the cached errors and
        // lastValidated marker would be for the pre-mutation value while the
        // model now holds something that has not been validated, so re-run
        // with the new value until model stabilises. A validator that keeps
        // mutating model never terminates here - that is a validator bug.
        while (true) {
          let result: ValidationError[]
          try {
            // We do not reuse passed options object, but reconstruct it so that
            // it is a new object and we control exactly what is being passed.
            result = await validator(value, { signal, eager, initial })
          } catch (err) {
            if (signal.aborted) {
              throw new ValidationAbortedError()
            }
            throw err
          }
          if (signal.aborted) {
            throw new ValidationAbortedError()
          }

          errors.value = result.map((error) => (error.el ? error : { ...error, el: el() ?? undefined }))
          lastValidated = { value, validator, eager, initial }

          if (model.value === value) {
            break
          }
          value = model.value
          // Keep in-flight tracking current so concurrent callers can match
          // against the value the loop is now validating. The non-null
          // assertion is safe: we just passed the signal.aborted check, so
          // nobody can have replaced inFlight (only an aborting call does),
          // and the synchronous code below leaves no window for that to
          // change before the next iteration's await.
          inFlight!.value = value
        }
      } finally {
        lock.value--
        if (inFlight?.promise === promise) {
          inFlight = null
        }
      }
    })()

    inFlight = { value: initialValue, validator, eager, initial, promise }

    return promise
  }

  // validate is the registry-facing entry point. errors.value is treated as
  // the cache: if lastValidated matches the current (value, validator), the
  // result is already in errors.value and we return it without re-invoking the
  // validator. Otherwise we either await an in-flight validation for the same
  // pair, or trigger one ourselves, then loop and re-check. Aborts caused by
  // validator aborts (changing model during await) loop transparently;
  // real validator errors propagate.
  async function validate(additionalSignal?: AbortSignal): Promise<ValidationError[]> {
    while (true) {
      if (additionalSignal?.aborted) {
        // Return last known errors. Caller should check its own
        // additionalSignal before using the result.
        return errors.value
      }

      // internalValidation handles validator-missing and cache-hit by
      // returning null, and joins an in-flight matching call (eager=false,
      // since validate represents a caller asking for the final state
      // including model-mutating side effects) by returning its promise.
      const waitFor = internalValidation({ signal: additionalSignal })
      if (!waitFor) {
        // No work to do: validator absent or lastValidated already covers the
        // current (value, validator). errors.value reflects current state.
        return errors.value
      }

      try {
        // Race additionalSignal against waitFor so an external abort during the wait returns
        // immediately, even when the in-flight validation was not started with additionalSignal
        // wired in (e.g. one started by the model watcher).
        if (additionalSignal) {
          await raceWithSignal(waitFor, additionalSignal)
        } else {
          await waitFor
        }
      } catch (err) {
        if (additionalSignal?.aborted) {
          // Return last known errors. Caller should check its own
          // additionalSignal before using the result.
          return errors.value
        }
        if (err instanceof ValidationAbortedError) {
          // Validator abort.
          continue
        }
        throw err
      }
      if (additionalSignal?.aborted) {
        // raceWithSignal resolved because additionalSignal aborted.
        // We return last known errors and let the caller bail.
        return errors.value
      }
    }
  }

  // Passing a component-level abort controller's signal as additional
  // options.signal is generally not needed because useValidation already has
  // its own abort controller tied to the component's lifecycle.
  // options.eager forwards to the validator so it can skip model-mutating
  // side effects (e.g. while the user is mid-typing). options.initial
  // forwards to the validator so it can skip both side effects and the
  // required check on the first run before any user interaction.
  async function runValidation(options?: { signal?: AbortSignal; eager?: boolean; initial?: boolean }): Promise<void> {
    const waitFor = internalValidation(options)
    if (!waitFor) {
      // runValidation declined to run.
      return
    }
    try {
      await waitFor
    } catch (err) {
      if (options?.signal?.aborted) {
        return
      }
      if (err instanceof ValidationAbortedError) {
        return
      }
      throw err
    }
  }

  // Immediate so the very first invocation runs on mount with initial=true,
  // letting validators surface structural errors on a pre-populated value
  // (e.g. an invalid URL) without yelling "required" before the user has
  // interacted. After that, lazy by default; once invalid, re-validate
  // eagerly on every model change so the error clears the moment the user
  // fixes it. Once errors empty again the guard falls back to false and we
  // return to lazy. The eager/initial flags are passed to the validator so
  // it can skip model-mutating side effects.
  let firstRun = true
  watch(
    model,
    async () => {
      if (firstRun) {
        firstRun = false
        await runValidation({ initial: true })
        return
      }
      if (errors.value.length > 0) {
        await runValidation({ eager: true })
      }
    },
    { immediate: true },
  )

  // Captured at setup time so an input whose v-model parent already holds
  // a value at mount registers that value as the baseline (isDirty=false
  // until the user actually types).
  const baselineValue = ref<T>(model.value) as Ref<T>

  const validatedInput: ValidatedInput = {
    validate,
    // The component-supplied reset typically clears errors.value
    // externally. Without invalidating lastValidated, the next validate()
    // call's entryCovers check would still match (same value, same
    // validator, same mode) and return the now-empty errors.value as a
    // false cache hit - the validator never re-runs. Clear the cache key
    // so subsequent validates rerun against the post-reset state.
    reset: () => {
      lastValidated = null
      reset()
    },
    // Built-in revert: restore the model to whatever setBaseline last
    // captured. We do not clear errors - the model watcher re-runs the
    // validator on every model change, and a pre-populated baseline value
    // may have legitimate errors (initial validation runs on mount) that
    // should stay until validation says otherwise.
    revert: () => {
      model.value = baselineValue.value
    },
    el,
    isDirty: computed(() => !equals(model.value, baselineValue.value)),
    isEmpty: isEmpty ?? computed<boolean>(() => !model.value),
    errors: shallowReadonly(errors),
    setBaseline: () => {
      baselineValue.value = model.value
    },
  }

  const { onInteraction } = useRegisterForValidation(validatedInput)

  // Treat every model change as a user interaction. Without immediate the
  // watcher only fires on real changes, so the initial mount value is
  // excluded. Programmatic mutations (validator normalization, reset()) also
  // flow through here, which is intentional - they represent state moving
  // on, which should also clear stale errors.
  watch(model, () => {
    onInteraction()
  })

  return { runValidation, validatedInput }
}
