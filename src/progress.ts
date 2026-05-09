import type { InjectionKey, Ref } from "vue"

import { computed, inject, provide, ref } from "vue"

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const progressKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-progress") : Symbol()
export const rootProgressKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-root-progress") : Symbol()

// useProgress returns a reactive and mutable local progress stacked on
// top of the parent progress (as provided with progressKey). It starts
// at 0 but increasing or decreasing it increases or decreases
// the parent progress for the same amount. It also sets the local progress
// as the parent progress for child components of the current component.
//
// If you need both local progress and the parent progress you should not
// use this function but use getParentProgress in combination with
// localProgress and setParentProgress. The reason is that if progressKey
// has not been provided, useProgress and getParentProgress create a new
// parent progress every time they are called, but you want local progress
// to be connected to the same parent progress.
//
// You should not call useProgress multiple times inside the same component,
// because setting the parent progress for child components can be set only
// once. Instead, you should use getParentProgress in combination with
// localProgress and setParentProgress and combine them yourself.
export function useProgress(): Ref<number> {
  const progress = localProgress(getParentProgress())
  setParentProgress(progress)
  return progress
}

// getParentProgress returns the parent progress (as provided with progressKey).
export function getParentProgress(): Ref<number> {
  return inject(progressKey, ref(0))
}

// localProgress returns a reactive and mutable local progress stacked on top
// of the provided parent progress. It starts at 0 but increasing or decreasing
// it increases or decreases the parent progress for the same amount.
export function localProgress(parentProgress: Ref<number>): Ref<number> {
  // This has to be a reactive variable otherwise things do not work
  // as expected and parentProgress can become negative for some reason.
  const progress = ref(0)
  return computed({
    get() {
      return progress.value
    },
    set(newValue) {
      parentProgress.value += newValue - progress.value
      progress.value = newValue
    },
  })
}

// setParentProgress sets the provided progress as the parent progress for
// child components of the current component.
export function setParentProgress(progress: Ref<number>) {
  provide(progressKey, progress)
}

// getRootProgress returns the root progress (as provided with rootProgressKey).
export function getRootProgress(): Ref<number> {
  return inject(rootProgressKey, ref(0))
}
