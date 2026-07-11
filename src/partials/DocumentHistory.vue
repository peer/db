<script setup lang="ts">
import type { DocumentHistoryItem } from "@/types"

import { computed, onMounted, onUnmounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
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
    if (abortController.signal.aborted) {
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

function formatAuthors(item: DocumentHistoryItem): string {
  if (!item.authors || item.authors.length === 0) {
    return t("views.DocumentGet.history.anonymous")
  }
  return item.authors.map((author) => author.id).join(", ")
}
</script>

<template>
  <div ref="el" :data-url="url">
    <i v-if="error" class="pd-documenthistory-error text-error-600">{{ t("common.status.loadingDataFailed") }}</i>
    <div v-else-if="history === null" class="pd-documenthistory-loading text-center">{{ t("common.status.loading") }}</div>
    <i v-else-if="history.length === 0" class="pd-documenthistory-empty text-gray-500">{{ t("views.DocumentGet.history.empty") }}</i>
    <table v-else class="w-full table-auto border-collapse">
      <thead>
        <tr>
          <th class="w-1/2 border-r border-slate-200 px-2 py-1 text-left font-bold">{{ t("common.labels.time") }}</th>
          <th class="w-1/2 border-l border-slate-200 px-2 py-1 text-left font-bold">{{ t("views.DocumentGet.history.author") }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="item in history" :key="item.changeset" class="border-t border-slate-200">
          <td class="border-r border-slate-200 px-2 py-1 align-top">
            <RouterLink class="link" :to="{ name: 'DocumentGet', params: { id }, query: encodeQuery({ version: item.version }) }"
              ><TimeDisplay :timestamp="timeString(item.at)" precision="s" :toggle="false"
            /></RouterLink>
          </td>
          <td class="border-l border-slate-200 px-2 py-1 align-top">{{ formatAuthors(item) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
