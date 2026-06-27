<script setup lang="ts">
import type { ClassCreateTreeNode } from "@/types"

import { computed } from "vue"

import ClassTreeLabel from "@/partials/ClassTreeLabel.vue"

const props = defineProps<{
  nodes: ClassCreateTreeNode[]
  onCreate: (classId: string) => void
}>()

// A run of consecutive childless nodes (creatable leaf classes shown as buttons) shares one row so the
// buttons flow horizontally and wrap; a node with children (a heading, or a creatable class that also has
// sub-classes) is its own item with its sub-list nested under it. The list itself stays vertical.
type Segment = { kind: "buttons"; key: string; nodes: ClassCreateTreeNode[] } | { kind: "group"; key: string; node: ClassCreateTreeNode }

const segments = computed((): Segment[] => {
  const out: Segment[] = []
  let run: ClassCreateTreeNode[] | null = null
  for (const node of props.nodes) {
    if (node.children.length === 0) {
      if (run === null) {
        run = []
        out.push({ kind: "buttons", key: "buttons:" + node.key, nodes: run })
      }
      run.push(node)
    } else {
      run = null
      out.push({ kind: "group", key: node.key, node })
    }
  }
  return out
})
</script>

<template>
  <ul class="flex flex-col gap-y-2 sm:gap-y-4">
    <li v-for="segment in segments" :key="segment.key" :class="segment.kind === 'buttons' ? 'flex flex-row flex-wrap gap-1 sm:gap-4' : undefined">
      <template v-if="segment.kind === 'buttons'">
        <ClassTreeLabel v-for="node in segment.nodes" :key="node.key" :node="node" :on-create="onCreate" />
      </template>
      <template v-else>
        <ClassTreeLabel :node="segment.node" :on-create="onCreate" />
        <ClassTreeList :nodes="segment.node.children" :on-create="onCreate" class="mt-2 pl-6 sm:mt-4" />
      </template>
    </li>
  </ul>
</template>
