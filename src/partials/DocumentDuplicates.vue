<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Result } from "@/types"

import { onUnmounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { anySignal } from "@/utils"

const props = defineProps<{
  doc: DeepReadonly<D>
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const duplicates = ref<Result[]>([])

const abortController = new AbortController()
// Aborts the previous in-flight search when a new one starts, so a slow response cannot
// overwrite the results of a newer search.
let refreshController = new AbortController()

onUnmounted(() => {
  abortController.abort()
})

// refresh re-runs the structural duplicate search for the document's current claims. It is called on
// every blur of the fields form. The backend compares the document's claims (identifier, string, link,
// reference, amount, time, has) against the index and returns the closest matches; a document with no
// matchable claims yields an empty list.
async function refresh() {
  refreshController.abort()
  refreshController = new AbortController()
  const signal = anySignal(abortController.signal, refreshController.signal)

  try {
    const results = await postJSON<Result[]>(router.apiResolve({ name: "DocumentFindDuplicates" }).href, { doc: props.doc }, signal, null)
    if (signal.aborted) {
      return
    }
    duplicates.value = results
  } catch (err) {
    if (signal.aborted) {
      return
    }
    console.error("DocumentDuplicates.refresh", err)
  }
}

defineExpose({
  refresh,
})
</script>

<template>
  <div v-if="duplicates.length > 0" class="pd-documentduplicates mt-4 rounded-sm border border-slate-200 bg-slate-50 p-4">
    <h2 class="text-sm font-semibold text-slate-700">{{ t("views.DocumentEdit.potentialDuplicates") }}</h2>
    <ul class="mt-2 flex flex-col gap-y-1">
      <li v-for="duplicate in duplicates" :key="duplicate.id">
        <DocumentRefInline :id="duplicate.id" />
      </li>
    </ul>
  </div>
</template>
