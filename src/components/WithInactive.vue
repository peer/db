<!--
WithInactive is to the inactive channel what WithLock is to the lock one: a
thin template wrapper that provides a caller-supplied count to its slot
subtree. It does not create the count, the caller owns the ref (typically via
useInactive or counterScope at script level) and decides what parent to chain
on. WithInactive only calls setParentInactive so descendants inside the slot
inject this ref via useInactivated / getParentInactive.

It is what a component which makes its subtree inactive uses to keep one part
of that subtree out of it: the controls which put the subtree in that state in
the first place have to stay usable, or there would be no way back out.

The inactive prop is a getter () => Ref<number> rather than the ref itself
because Vue's template binding auto-unwraps top-level refs. A plain
:inactive="someRef" would arrive here as a number and we would lose the ability
to provide it. A function passes through unchanged.

The slot exposes the unwrapped read-only count as inactive prop.
-->

<script setup lang="ts">
import type { Ref } from "vue"

import { setParentInactive } from "@/progress"

const props = defineProps<{
  inactive: () => Ref<number>
}>()

const inactive = props.inactive()
setParentInactive(inactive)
</script>

<template>
  <slot :inactive="inactive" />
</template>
