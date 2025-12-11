<script setup lang="ts">
import type { RelFilterState, RelSearchResult } from "@/types"
import type { PeerDBDocument } from "@/document"

import { ref, computed, onBeforeUnmount } from "vue"
import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import CheckBox from "@/components/CheckBox.vue"
import { useRelFilterValues, NONE, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE } from "@/search"
import { equals, getName, useLimitResults, loadingWidth, useInitialLoad } from "@/utils"
import { injectProgress } from "@/progress"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = defineProps<{
  s: string
  searchTotal: number
  result: RelSearchResult
  state: RelFilterState
  updateProgress: number
}>()

const emit = defineEmits<{
  (e: "update:state", state: RelFilterState): void
}>()

const el = ref(null)

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

const progress = injectProgress()
const {
  results,
  total,
  error,
  url: resultsUrl,
} = useRelFilterValues(
  computed(() => props.s),
  computed(() => props.result),
  el,
  progress,
)
const { laterLoad } = useInitialLoad(progress)

const { limitedResults, hasMore, loadMore } = useLimitResults(results, FILTERS_INITIAL_LIMIT, FILTERS_INCREASE)

const limitedResultsWithNone = computed(() => {
  // We cannot add "none" result without knowing other results because the "none" result might not be
  // shown initially at all if other results have higher counts. If were to add "none" result always,
  // it could happen that it flashes initially and then is hidden once other results load.
  if (!limitedResults.value.length) {
    return limitedResults.value
  } else if (props.result.count >= props.searchTotal) {
    return limitedResults.value
  }
  const res = [...limitedResults.value, { count: props.searchTotal - props.result.count }]
  res.sort((a, b) => b.count - a.count)
  return res
})

const checkboxState = computed({
  get(): RelFilterState {
    return props.state
  },
  set(value: RelFilterState) {
    if (abortController.signal.aborted) {
      return
    }

    // TODO: Remove workaround for Vue not supporting Symbols for checkbox values.
    //       See: https://github.com/vuejs/core/issues/10597
    value = value.map((v) => (v === "__NONE__" ? NONE : v))

    if (!equals(props.state, value)) {
      emit("update:state", value)
    }
  },
})

const WithPeerDBDocument = WithDocument<PeerDBDocument>
</script>

<template>
  <div class="flex flex-col rounded border bg-white p-4 shadow" :class="{ 'data-reloading': laterLoad }" :data-url="resultsUrl">
    <div class="flex items-baseline gap-x-1">
      <DocumentRefInline :id="result.id" class="mb-1.5 text-lg leading-none" />
      ({{ result.count }})
    </div>
    <ul ref="el">
      <li v-if="error">
        <i class="text-error-600">loading data failed</i>
      </li>
      <template v-else-if="total === null">
        <li v-for="i in 3" :key="i" class="flex animate-pulse items-baseline gap-x-1">
          <div class="my-1.5 h-2 w-4 rounded bg-slate-200"></div>
          <div class="my-1.5 h-2 rounded bg-slate-200" :class="[loadingWidth(`${result.id}/${i}`)]"></div>
          <div class="my-1.5 h-2 w-8 rounded bg-slate-200"></div>
        </li>
      </template>
      <template v-else>
        <li v-for="res in limitedResultsWithNone" :key="'id' in res ? res.id : NONE" class="flex items-baseline gap-x-1">
          <template v-if="'id' in res && (res.count != searchTotal || state.includes(res.id))">
            <CheckBox :id="'rel/' + result.id + '/' + res.id" v-model="checkboxState" :progress="updateProgress" :value="res.id" class="my-1 self-center" />
            <WithPeerDBDocument :id="res.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <label
                  :for="'rel/' + result.id + '/' + res.id"
                  class="my-1 leading-none"
                  :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
                  :data-url="url"
                  v-html="getName(doc.claims) || '<i>no name</i>'"
                ></label>
              </template>
              <template #loading="{ url }">
                <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(res.id)]"></div>
              </template>
            </WithPeerDBDocument>
            <label :for="'rel/' + result.id + '/' + res.id" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
            <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="'id' in res && res.count == searchTotal">
            <div class="my-1 inline-block h-4 w-4 shrink-0 self-center border border-transparent"></div>
            <WithPeerDBDocument :id="res.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <div class="my-1 inline-block leading-none" :data-url="url" v-html="getName(doc.claims) || '<i>no name</i>'"></div>
              </template>
              <template #loading="{ url }">
                <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(res.id)]"></div>
              </template>
            </WithPeerDBDocument>
            <div class="my-1 inline-block leading-none">({{ res.count }})</div>
            <RouterLink :to="{ name: 'DocumentGet', params: { id: res.id } }" class="link"
              ><ArrowTopRightOnSquareIcon alt="Link" class="inline h-5 w-5 align-text-top"
            /></RouterLink>
          </template>
          <template v-else-if="!('id' in res)">
            <CheckBox :id="'rel/' + result.id + '/none'" v-model="checkboxState" :progress="updateProgress" value="__NONE__" class="my-1 self-center" />
            <label :for="'rel/' + result.id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              ><i>none</i></label
            >
            <label :for="'rel/' + result.id + '/none'" class="my-1 leading-none" :class="updateProgress > 0 ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
              >({{ res.count }})</label
            >
          </template>
        </li>
      </template>
    </ul>
    <Button v-if="total !== null && hasMore" primary class="mt-2 w-1/2 min-w-fit self-center" @click="loadMore">{{ total - limitedResults.length }} more</Button>
    <div v-else-if="total !== null && total > limitedResults.length" class="mt-2 text-center text-sm">{{ total - limitedResults.length }} values not shown.</div>
  </div>
</template>
