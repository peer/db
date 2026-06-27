<script setup lang="ts">
import type { D } from "@/document"
import type { ClassCreateTreeNode } from "@/types"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { loadingWidth } from "@/utils"

defineProps<{
  node: ClassCreateTreeNode
  onCreate: (classId: string) => void
}>()

const WithDocumentD = WithDocument<D>
</script>

<template>
  <WithDocumentD :id="node.res.id" name="DocumentGet">
    <template #default="{ doc, url }">
      <Button v-if="node.res.canCreate" type="button" :data-url="url" @click.prevent="onCreate(node.res.id)">
        <DisplayLabel :doc="doc" />
      </Button>
      <!-- A class a document cannot be created for (abstract, or without fields) is shown only as a structural heading. -->
      <h2 v-else class="text-xl leading-none font-medium" :data-url="url"><DisplayLabel :doc="doc" /></h2>
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
