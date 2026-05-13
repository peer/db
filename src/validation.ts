import type { InjectionKey, MaybeRefOrGetter, Ref, WritableComputedRef } from "vue"

import type { ValidatedInput, ValidateFn, ValidationError, ValidatorFn } from "@/types"

import { computed, inject, onBeforeUnmount, onMounted, provide, ref, toRef, watch } from "vue"

import { anySignal, raceWithSignal } from "@/utils"

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const registerForValidationKey: InjectionKey<(instance: ValidatedInput) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-validation-register") : Symbol()
export const unregisterForValidationKey: InjectionKey<(instance: ValidatedInput) => void> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-validation-unregister") : Symbol()

// useMergedErrors allows defining a computed which can be passed to input's v-model:errors
// and combines errors from the parent and child validation.
//
// It should not be used inline in a template but stored or cached to be able to keep the state.
export function useMergedErrors(parentErrors: MaybeRefOrGetter<ValidationError[]>): WritableComputedRef<ValidationError[]> {
  const parentErrorsRef = toRef(parentErrors)
  const childErrors = ref<ValidationError[]>([])
  return computed<ValidationError[]>({
    get() {
      return [...parentErrorsRef.value, ...childErrors.value]
    },
    set(value) {
      childErrors.value = value
    },
  })
}

// useRegisterForValidation is called by an input to make itself discoverable
// by the nearest ancestor (one that called useValidationRegistry). It is
// a no-op when there is no such ancestor, so inputs can be used without them.
export function useRegisterForValidation(input: ValidatedInput): void {
  const register = inject(registerForValidationKey, null)
  const unregister = inject(unregisterForValidationKey, null)
  onMounted(() => {
    register?.(input)
  })
  onBeforeUnmount(() => {
    unregister?.(input)
  })
}

// useValidationRegistry is called to collect validated inputs from all
// descendant inputs that called useRegisterForValidation. validateAll runs
// every input's validator in parallel and returns the flat list of errors.
//
// Validation registries nest transparently: if el getter is provided,
// the registry self-registers as a ValidatedInput, so an outer validation
// registry sees inner one as a single input whose validate is its validateAll.
// useRegisterForValidation is a no-op when there is no outer registry.
export function useValidationRegistry(el?: () => HTMLElement | null): {
  validateAll: ValidateFn
} {
  const inputs = new Set<ValidatedInput>()

  provide(registerForValidationKey, (input: ValidatedInput) => {
    inputs.add(input)
  })
  provide(unregisterForValidationKey, (input: ValidatedInput) => {
    inputs.delete(input)
  })

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

  if (el) {
    useRegisterForValidation({
      el,
      validate: validateAll,
    })
  }

  return { validateAll }
}

// focusFirstInvalid focuses the error whose el appears earliest in the
// document. Errors without an el are skipped. Pairs that compareDocumentPosition
// reports as disconnected or identical leave the running winner unchanged.
export function focusFirstInvalid(errors: ValidationError[]) {
  let earliest: HTMLElement | null = null
  for (const error of errors) {
    if (!error.el) {
      continue
    }
    if (!earliest) {
      earliest = error.el
      continue
    }
    if (earliest.compareDocumentPosition(error.el) & Node.DOCUMENT_POSITION_PRECEDING) {
      earliest = error.el
    }
  }
  earliest?.focus()
}

class ValidationAbortedError extends Error {}

export function useValidation<T>(
  model: Ref<T>,
  errors: Ref<ValidationError[]>,
  progress: Ref<number>,
  validatorGetter: () => ValidatorFn<T> | undefined,
  el: () => HTMLElement | null,
): {
  runValidation: (options?: { signal?: AbortSignal; eager?: boolean }) => Promise<void>
  validatedInput: ValidatedInput
} {
  let validateAbortController: AbortController | null = null
  let inFlight: { value: T; validator: ValidatorFn<T>; eager: boolean; promise: Promise<void> } | null = null
  let lastValidated: { value: T; validator: ValidatorFn<T>; eager: boolean } | null = null

  onBeforeUnmount(() => {
    validateAbortController?.abort()
  })

  // Only treat an entry as covering a request when the value, validator, and
  // mode all match. We don't assume a lazy result is strictly stronger than
  // an eager one, because a validator's behavior under each mode is opaque to
  // us; a mode mismatch always re-runs.
  function entryCovers(entry: { value: T; validator: ValidatorFn<T>; eager: boolean } | null, value: T, validator: ValidatorFn<T>, eager: boolean): boolean {
    if (!entry) return false
    return entry.value === value && entry.validator === validator && entry.eager === eager
  }

  // internalValidation uses abort-and-restart: every call aborts any prior in-flight
  // one and starts a new validator invocation. On successful completion it writes the
  // result to errors.value and records lastValidated as a cache marker. On abort the
  // IIFE throws ValidationAbortedError so callers awaiting inFlight.promise can
  // distinguish validator aborts from real validator errors.
  function internalValidation(options?: { signal?: AbortSignal; eager?: boolean }): Promise<void> | null {
    const validator = validatorGetter()
    if (!validator) return null
    const initialValue = model.value
    const eager = options?.eager ?? false

    // Already have a result for this exact (value, validator, mode) in errors.value.
    if (entryCovers(lastValidated, initialValue, validator, eager)) {
      return null
    }
    // Already running the validator for this exact (value, validator, mode): join the in-flight call.
    if (entryCovers(inFlight, initialValue, validator, eager)) {
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
      progress.value++
      try {
        let value = initialValue
        // Validators may mutate model.value as a side effect (ideally gated on
        // !eager). If that happens, the cached errors and lastValidated marker
        // would be for the pre-mutation value while the model now holds something
        // that has not been validated, so re-run with the new value until model
        // stabilises. A validator that keeps mutating model never terminates
        // here - that is a validator bug.
        while (true) {
          let result: ValidationError[]
          try {
            // We do not reuse passed options object, but reconstruct it so that
            // it is a new object and we control exactly what is being passed.
            result = await validator(value, { signal, eager })
          } catch (err) {
            if (signal.aborted) {
              throw new ValidationAbortedError()
            }
            throw err
          }
          if (signal.aborted) {
            throw new ValidationAbortedError()
          }

          errors.value = result
          lastValidated = { value, validator, eager }

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
        progress.value--
        if (inFlight?.promise === promise) {
          inFlight = null
        }
      }
    })()

    inFlight = { value: initialValue, validator, eager, promise }

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

      const validator = validatorGetter()
      if (!validator) {
        return errors.value
      }
      const value = model.value

      // validate() is always lazy (eager=false) - it represents a caller
      // asking for the final state, including model-mutating side effects.
      if (entryCovers(lastValidated, value, validator, false)) {
        return errors.value
      }

      const waitFor = internalValidation({ signal: additionalSignal })
      if (!waitFor) {
        // internalValidation declined: lastValidated already covers the
        // request (state shifted between our cache check above and its own),
        // or the validator disappeared. Loop to re-evaluate.
        continue
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
  // side effects (e.g. while the user is mid-typing).
  async function runValidation(options?: { signal?: AbortSignal; eager?: boolean }): Promise<void> {
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

  // Lazy by default; once invalid, re-validate eagerly on every model change so
  // the error clears the moment the user fixes it. Once errors empty again the
  // guard falls back to false and we return to lazy. The eager flag is passed
  // to the validator so it can skip model-mutating side effects (e.g. trimming
  // whitespace while the user is still typing).
  watch(model, async () => {
    if (errors.value.length > 0) {
      await runValidation({ eager: true })
    }
  })

  const validatedInput: ValidatedInput = {
    validate,
    el,
  }

  useRegisterForValidation(validatedInput)

  return { runValidation, validatedInput }
}
