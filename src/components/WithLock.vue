<!--
WithLock is a thin template wrapper that provides a caller-supplied lock
ref to its slot subtree. It does not create the lock, the caller owns
the ref (typically via lockScope at script level) and decides what
parent to chain on. WithLock only calls setParentLock so descendants
inside the slot inject this ref via useLocked / getParentLock.

The lock prop is a getter () => Ref<number> rather than the ref
itself because Vue's template binding auto-unwraps top-level refs.
A plain :lock="someRef" would arrive here as a number and we would
lose the ability to provide it. A function passes through unchanged.

The slot exposes the unwrapped read-only count as lock prop.
-->

<script setup lang="ts">
import type { Ref } from "vue"

import { setParentLock } from "@/progress"

const props = defineProps<{
  lock: () => Ref<number>
}>()

const lock = props.lock()
setParentLock(lock)
</script>

<template>
  <slot :lock="lock" />
</template>
