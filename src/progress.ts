import type { InjectionKey, Ref } from "vue"

import { computed, inject, ref } from "vue"

export const progressKey: InjectionKey<Ref<number>> = Symbol()

// injectProgress returns a reactive and mutable local view of the
// main progress (as injected with progressKey). It starts at 0
// but increasing or decreasing it increases or decreases
// the main progress for the same amount.
//
// If you need both local progress and main progress you should not
// use this function but use injectMainProgress in combination with
// localProgress. The reason is that if progressKey has not been
// provided, injectProgress and injectMainProgress create a new main
// progress every time they are called, but you want local progress
// to be connected to the same main progress.
export function injectProgress(): Ref<number> {
  return localProgress(injectMainProgress())
}

// injectMainProgress returns the main progress (as injected with progressKey).
export function injectMainProgress(): Ref<number> {
  return inject(progressKey, ref(0))
}

// localProgress returns a reactive and mutable local view of the
// provided main progress. It starts at 0 but increasing or decreasing
// it increases or decreases the main progress for the same amount.
export function localProgress(mainProgress: Ref<number>): Ref<number> {
  // This has to be a reactive variable otherwise things do not work
  // as expected and mainProgress can become negative for some reason.
  const progress = ref(0)
  return computed({
    get() {
      return progress.value
    },
    set(newValue) {
      mainProgress.value += newValue - progress.value
      progress.value = newValue
    },
  })
}
