import type { ComputedRef, InjectionKey, Ref } from "vue"

import { computed, inject, provide, ref } from "vue"

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const progressKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-progress") : Symbol()
export const lockKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-lock") : Symbol()
export const inactiveKey: InjectionKey<Ref<number>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-inactive") : Symbol()
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
//
// Inactive is a third channel which cascades downward the way lock does and
// is read the same way, and it is what a component reaches for when it makes
// a descendant inactive for a reason which does not pass by itself: a bound
// marked as having no value takes no value for as long as the mark is on it,
// which is a state of the form and not an operation running. Keeping it apart
// from the lock is what lets a control say which of the two it is in, because
// both are drawn the same shade of grey.

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

// getParentInactive returns the parent inactive count (as provided with inactiveKey).
export function getParentInactive(): Ref<number> {
  return inject(inactiveKey, ref(0))
}

// setParentInactive sets the provided count as the parent inactive count for
// descendants of the current component.
export function setParentInactive(inactive: Ref<number>) {
  provide(inactiveKey, inactive)
}

// getRootProgress returns the root progress (as provided with rootProgressKey).
export function getRootProgress(): Ref<number> {
  return inject(rootProgressKey, ref(0))
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
// several independent per-operation counters, use localCounter in combination
// with getParentProgress yourself.
export function useProgress(): Ref<number> {
  const progress = localCounter(getParentProgress())
  setParentProgress(progress)
  return progress
}

// counterScope returns a reactive ref whose value is the combined
// parent + own count and whose writes land on the own counter only.
// It is the building block for the boundaries of the channels which cascade
// downward: useLock and useInactive wrap this and also publish the result to
// descendants. Call counterScope directly when you need a combined ref that is
// provided in a different way (for example by a WithLock wrapper that scopes
// only part of the template).
export function counterScope(parent: Ref<number>): Ref<number> {
  // This has to be a reactive variable otherwise the combined computed
  // does not stay coherent.
  const own = ref(0)
  return computed({
    get() {
      return parent.value + own.value
    },
    set(newValue) {
      own.value = newValue - parent.value
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
// several independent per-operation counters, use counterScope in combination
// with getParentLock yourself.
export function useLock(): Ref<number> {
  const lock = counterScope(getParentLock())
  setParentLock(lock)
  return lock
}

// useInactive creates an inactive boundary at the current component, the way
// useLock creates a lock boundary: the returned ref is the combined parent +
// local count, writes land on the local counter only, and the result is what
// descendants see through useInactivated / getParentInactive.
//
// It should be called at most once per component, for the same reason useLock
// should be: the count provided to descendants can be set only once.
export function useInactive(): Ref<number> {
  const inactive = counterScope(getParentInactive())
  setParentInactive(inactive)
  return inactive
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
//
// An inputs control which renders that state also carries the pd-locked class
// while it does. The class marks the state as one which passes: a control is
// locked while something is being done and is usable again afterwards, unlike
// one which is inactive because it was given the readonly or disabled prop or
// because an ancestor made it so. That is what tells a screenshot taken of a
// form mid-operation from one taken of the form at rest, which is otherwise
// only visible as a shade of grey.
export function useLocked(): ComputedRef<boolean> {
  const lock = getParentLock()
  return computed(() => lock.value > 0)
}

// useInactivated returns a boolean computed that is true when the nearest
// useInactive ancestor's count > 0 and is permanently false when no such
// ancestor exists. It is the other half of what an inputs control renders its
// inactive state from, next to its own readonly or disabled prop.
//
// A control inactive for either of those reasons carries the pd-inactive class
// rather than pd-locked, because neither passes by itself: it stays until the
// prop changes or the ancestor stops asking for it.
export function useInactivated(): ComputedRef<boolean> {
  const inactive = getParentInactive()
  return computed(() => inactive.value > 0)
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

// localCounter returns a reactive sub-counter chained into the provided
// parent counter. Reads return its own local count, writes update local and
// bubble the same delta into parent. Several siblings chained on the same
// parent independently track their own operations while all of them
// contribute to the shared parent's counter.
export function localCounter(parent: Ref<number>): Ref<number> {
  return pairCounters(ref(0), parent)
}
