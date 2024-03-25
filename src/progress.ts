import type { InjectionKey, Ref } from "vue"

import { ref, inject, watch } from "vue"

export const progressKey = Symbol() as InjectionKey<Ref<number>>

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

export function localProgress(mainProgress: Ref<number>): Ref<number> {
  const progress = ref(0)
  watch(
    progress,
    (newProgress, oldProgress) => {
      mainProgress.value += newProgress - oldProgress
    },
    {
      flush: "sync",
    },
  )
  return progress
}
