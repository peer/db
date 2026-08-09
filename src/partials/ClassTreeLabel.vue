<script setup lang="ts">
import type { D } from "@/document"
import type { ClassCreateTreeNode } from "@/types"

import { ref } from "vue"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"

const props = defineProps<{
  node: ClassCreateTreeNode
  onCreate: (classId: string) => Promise<void>
}>()

// Creating a document locks every class button at once (the lock cascades from DocumentCreate), which
// on its own does not tell which class the user picked. Counting the creation this button started
// drives its own progress bar, so the button which locked all the others is the one which looks busy.
// The count feeds the visual only: the lock and the global progress bar are already driven by the
// creation itself.
const creating = ref(0)

async function onClick() {
  creating.value += 1
  try {
    await props.onCreate(props.node.res.id)
  } finally {
    creating.value -= 1
  }
}

const WithDocumentD = WithDocument<D>
</script>

<template>
  <WithDocumentD :id="node.res.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <Button
        v-if="node.res.creatable"
        type="button"
        class="pd-classtreelabel pd-classtreelabel-button"
        :class="`pd-classtreelabel-button-${node.res.id}`"
        :data-url="url"
        :progress="creating"
        @click.prevent="onClick"
      >
        <DisplayLabel :doc="doc" />
      </Button>
      <!-- A class a document cannot be created for (abstract, or without fields) is shown only as a structural heading. -->
      <h2 v-else class="pd-classtreelabel pd-classtreelabel-title text-xl leading-none font-medium" :data-url="url"><DisplayLabel :doc="doc" /></h2>
    </template>
    <template #loading="{ url }">
      <div
        class="pd-withdocument-loading my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
        :data-url="url"
        :class="[loadingWidth(node.res.id)]"
        aria-hidden="true"
      ></div>
    </template>
  </WithDocumentD>
</template>
