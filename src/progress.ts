import type { ComputedRef, InjectionKey, Ref } from "vue"

import { computed, inject, provide, ref } from "vue"

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const progressKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-progress") : Symbol()
export const lockKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-lock") : Symbol()
export const rootProgressKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-root-progress") : Symbol()

// Progress and lock are two orthogonal channels used together by typical
// "doing async work, lock inputs controls" patterns:
//
//   - Progress bubbles upward. A descendant write feeds into the nearest
//     ancestor's progress counter, which feeds into its ancestor, and so
//     on up to the root. The root progress is the global loading bar.
//     Each useProgress() boundary exposes a per-subtree counter that is
//     the sum of all loading happening inside it.
//
//   - Lock cascades downward. A useLock() boundary exposes a counter to
//     its descendants. Descendants read it via useLocked()/getParentLock()
//     and disable themselves when it is > 0. Each input control can also
//     create its own useLock boundary so its validation locks only itself
//     (its lock is the combined parent + local, so the input control also
//     locks when the surrounding inputs controls do). Outer useLock
//     boundaries belong to operations that should freeze all inputs
//     controls (save/submit handlers, anything where partial state would
//     be incoherent).
//
// useBusy() is the convenience wrapper for the common case where a
// component wants both effects.

// getParentProgress returns the parent progress (as provided with progressKey).
export function getParentProgress(): Ref<number> {
  return inject(progressKey, ref(0))
}

// setParentProgress sets the provided progress as the parent progress for
// descendants of the current component.
export function setParentProgress(progress: Ref<number>) {
  provide(progressKey, progress)
}

// getParentLock returns the parent lock (as provided with lockKey).
export function getParentLock(): Ref<number> {
  return inject(lockKey, ref(0))
}

// setParentLock sets the provided lock as the parent lock for descendants
// of the current component.
export function setParentLock(lock: Ref<number>) {
  provide(lockKey, lock)
}

// getRootProgress returns the root progress (as provided with rootProgressKey).
export function getRootProgress(): Ref<number> {
  return inject(rootProgressKey, ref(0))
}

// localProgress returns a reactive sub-counter chained into the provided
// parentProgress. Reads return its own local count, writes update local and
// bubble the same delta into parent. Several siblings chained on the same
// parent independently track their own operations while all of them
// contribute to the shared parent's counter.
export function localProgress(parentProgress: Ref<number>): Ref<number> {
  // This has to be a reactive variable otherwise things do not work
  // as expected and parent can become negative for some reason.
  const own = ref(0)
  return computed({
    get() {
      return own.value
    },
    set(newValue) {
      parentProgress.value += newValue - own.value
      own.value = newValue
    },
  })
}

// useProgress creates a progress boundary at the current component. It
// returns a reactive sub-counter chained on the inherited parent progress:
// writes bubble up the chain to the root progress (the global loading bar),
// and the returned counter is provided as the parent progress for
// descendants so descendant useProgress calls stack on top of it and
// descendant operations can sum into it.
//
// You should not call useProgress multiple times inside the same component
// because the parent progress for descendants can be set only once. To hold
// several independent per-operation counters, use localProgress in combination
// with getParentProgress yourself.
export function useProgress(): Ref<number> {
  const progress = localProgress(getParentProgress())
  setParentProgress(progress)
  return progress
}

// lockScope returns a reactive ref whose value is the combined
// parentLock + own count and whose writes land on the own counter only.
// It is the building block for lock boundaries: useLock wraps this and
// also publishes the result to descendants. Call lockScope directly
// when you need a combined ref that is provided in a different way (for
// example by a WithLock wrapper that scopes only part of the template).
export function lockScope(parentLock: Ref<number>): Ref<number> {
  // This has to be a reactive variable otherwise the combined computed
  // does not stay coherent.
  const own = ref(0)
  return computed({
    get() {
      return parentLock.value + own.value
    },
    set(newValue) {
      own.value = newValue - parentLock.value
    },
  })
}

// useLock creates a lock boundary at the current component. It returns a
// ref whose value is the combined parent + local count, which is what
// descendants see via useLocked / getParentLock. Writes through this ref
// land on the boundary's local counter only; they do not propagate further
// up, so sibling components of this one are not affected.
//
// You should not call useLock multiple times inside the same component
// because the lock provided to descendants can be set only once. To hold
// several independent per-operation counters, use lockScope in combination
// with getParentLock yourself.
export function useLock(): Ref<number> {
  const lock = lockScope(getParentLock())
  setParentLock(lock)
  return lock
}

// useBusy returns a writable counter that updates both the progress and
// lock channels in lockstep at this component's boundary.
//
// It is the convenience for the use case where a component wants its own
// work to both (a) show in the global progress bar via the progress channel
// and (b) lock its subtree's inputs controls via the lock channel. Reach for
// useProgress or useLock directly when you specifically want only one of
// the two effects.
//
// Like useProgress and useLock, useBusy creates both boundaries, so it
// should be called at most once per component.
export function useBusy(): Ref<number> {
  const progress = useProgress()
  const lock = useLock()
  return pairCounters(progress, lock)
}

// useLocked returns a boolean computed that is true when the nearest useLock
// ancestor's count > 0 and is permanently false when no useLock ancestor exists.
//
// They can be used for inputs controls to decide whether they should render in
// their disabled/read-only state.
export function useLocked(): ComputedRef<boolean> {
  const lock = getParentLock()
  return computed(() => lock.value > 0)
}

// pairCounters returns a writable counter that, when written, increments
// the provided refs in lockstep. Reads return the underlying first count.
//
// Use this when a per-operation sub-counter needs to drive both channels
// (e.g. a search that should both show in the navbar and lock the form).
export function pairCounters(first: Ref<number>, second: Ref<number>): Ref<number> {
  return computed({
    get() {
      return first.value
    },
    set(newValue) {
      const delta = newValue - first.value
      first.value += delta
      second.value += delta
    },
  })
}
