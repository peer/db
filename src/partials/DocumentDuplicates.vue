<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { D } from "@/document"
import type { Result } from "@/types"

import { onUnmounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import { NAME } from "@/core"
import { getClaimsOfTypeWithConfidence } from "@/document"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { anySignal } from "@/utils"

const props = defineProps<{
  doc: DeepReadonly<D>
  // The ID of the document being created, so it is never listed as its own duplicate.
  excludeId: string
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

// The query is the document's name(s); without a name there is nothing to match duplicates against.
function nameQuery(): string {
  return getClaimsOfTypeWithConfidence(props.doc.claims, "string", NAME)
    .map((claim) => claim.string)
    .join(" ")
    .trim()
}

// refresh re-runs the duplicate search for the document's current name. It is called on every blur
// of the fields form.
async function refresh() {
  refreshController.abort()
  refreshController = new AbortController()
  const signal = anySignal(abortController.signal, refreshController.signal)

  const query = nameQuery()
  if (!query) {
    duplicates.value = []
    return
  }

  try {
    const results = await postJSON<Result[]>(router.apiResolve({ name: "DocumentFindDuplicates" }).href, { query, exclude: props.excludeId }, signal, null)
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
