<script setup lang="ts">
import type { DocumentHistoryItem } from "@/types"

import { computed, onMounted, onUnmounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import IdentityInline from "@/partials/IdentityInline.vue"
import ListFormat from "@/partials/ListFormat.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import { getRootProgress } from "@/progress"
import { encodeQuery, timeStringFromFloat64 } from "@/utils"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// We use root progress for loading data.
const rootProgress = getRootProgress()

const url = computed(() => router.apiResolve({ name: "DocumentHistory", params: { id: props.id } }).href)

const el = ref<HTMLElement | null>(null)
const history = ref<DocumentHistoryItem[] | null>(null)
const error = ref<string | null>(null)

const abortController = new AbortController()
onUnmounted(() => {
  abortController.abort()
})

// The backend returns changesets newest first.
onMounted(async () => {
  try {
    const response = await getURL<DocumentHistoryItem[]>(url.value, el, abortController.signal, rootProgress)
    // getURL returns null only on abort.
    if (abortController.signal.aborted || response === null) {
      return
    }
    history.value = response.doc
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentHistory", err)
    // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
    error.value = `${err}`
  }
})

// Convert the changeset timestamp (an ISO timestamp) to the Time-claim string TimeDisplay renders, at second precision.
function timeString(at: string): string {
  return timeStringFromFloat64(new Date(at).getTime() / 1000, "s")
}
</script>

<template>
  <div ref="el" class="pd-documenthistory" :data-url="url">
    <i v-if="error" class="pd-documenthistory-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
    <div v-else-if="history === null" class="pd-documenthistory-loading text-center">{{ t("common.status.loading") }}</div>
    <i v-else-if="history.length === 0" class="pd-documenthistory-empty text-gray-500">{{ t("views.DocumentGet.history.empty") }}</i>
    <table v-else class="pd-documenthistory-list w-full table-auto border-collapse">
      <thead class="pd-documenthistory-header">
        <tr class="pd-documenthistory-row-header">
          <th class="pd-documenthistory-column-time w-1/2 border-r border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.time") }}</th>
          <th class="pd-documenthistory-column-author w-1/2 border-l border-slate-200 px-2 py-1 text-left font-bold">{{ t("views.DocumentGet.history.author") }}</th>
        </tr>
      </thead>
      <tbody class="pd-documenthistory-body">
        <tr v-for="item in history" :key="item.changeset" class="pd-documenthistory-item border-t border-slate-200">
          <td class="pd-documenthistory-text-time border-r border-slate-200 px-2 py-1 align-top">
            <RouterLink class="pd-documenthistory-link-version link" :to="{ name: 'DocumentGet', params: { id }, query: encodeQuery({ version: item.version }) }"
              ><TimeDisplay :timestamp="timeString(item.at)" precision="s" :toggle="false"
            /></RouterLink>
          </td>
          <td class="pd-documenthistory-text-author border-l border-slate-200 px-2 py-1 align-top">
            <!-- A changeset made by nobody signed in has no authors. The authors of one all made it, so they are listed as a conjunction. -->
            <template v-if="!item.authors?.length">{{ t("views.DocumentGet.history.anonymous") }}</template>
            <ListFormat v-else v-slot="{ index }" :count="item.authors.length" type="conjunction"><IdentityInline :subject="item.authors[index].id" /></ListFormat>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
