<!--
WithProgress provides a progress counter to its slot subtree. Descendants'
useProgress chains land on this counter instead of bubbling further up.

By default it creates a fresh ref(0) so progress writes inside the slot
stay local (e.g. the navbar bar progress bar is not shown).
Pass progress (a getter () => Ref<number>) when the caller already owns
the ref and just wants WithProgress to provide it. The getter pattern
avoids Vue auto-unwrapping the ref through the template binding.

The slot exposes the unwrapped read-only count as progress prop.
-->

<script setup lang="ts">
import type { Ref } from "vue"

import { ref } from "vue"

import { setParentProgress } from "@/progress"

const props = defineProps<{
  progress?: () => Ref<number>
}>()

const progress = props.progress ? props.progress() : ref(0)
setParentProgress(progress)
</script>

<template>
  <slot :progress="progress" />
</template>
