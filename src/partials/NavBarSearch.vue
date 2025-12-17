<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClientSearchSession } from "@/types"

import { onBeforeUnmount, ref, watchEffect } from "vue"
import { useRouter } from "vue-router"
import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"

import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { createSearchSession } from "@/search"
import { injectProgress } from "@/progress"

const props = withDefaults(
  defineProps<{
    searchSession?: DeepReadonly<ClientSearchSession> | ClientSearchSession | null
    updateSearchSessionProgress?: number
  }>(),
  {
    searchSession: undefined,
    updateSearchSessionProgress: 0,
  },
)

const $emit = defineEmits<{
  queryChange: [change: string]
}>()

const router = useRouter()

const progress = injectProgress()

const abortController = new AbortController()

const searchQuery = ref("")

watchEffect((onCleanup) => {
  if (abortController.signal.aborted) {
    return
  }

  if (!props.searchSession) {
    return
  }

  // We update the search query in one direction only when search session changes.
  searchQuery.value = props.searchSession.query
})

onBeforeUnmount(() => {
  abortController.abort()
})

async function onSubmit() {
  if (abortController.signal.aborted) {
    return
  }

  // If searchSession is provided, we do not create a new search session but notify
  // the parent component that the query has changed.
  if (props.searchSession) {
    $emit("queryChange", searchQuery.value)
    return
  }

  progress.value += 1
  try {
    await createSearchSession(
      router,
      {
        query: searchQuery.value,
      },
      abortController.signal,
      progress,
    )
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("NavBarSearch.onSubmit", err)
  } finally {
    progress.value -= 1
  }
}
</script>

<template>
  <form class="flex flex-grow gap-x-1 sm:gap-x-4" novalidate @submit.prevent="onSubmit()">
    <InputText id="search-input-text" v-model="searchQuery" :progress="progress + updateSearchSessionProgress" class="max-w-xl flex-grow" />
    <Button :progress="progress + updateSearchSessionProgress" type="submit" primary class="!px-3.5">
      <MagnifyingGlassIcon class="h-5 w-5 sm:hidden" alt="Search" />
      <span class="hidden sm:inline">Search</span>
    </Button>

    <div id="navbarsearch-teleport-end" class="flex gap-x-1 sm:gap-x-4" />
  </form>
</template>
