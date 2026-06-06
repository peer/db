<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { SearchSession } from "@/types"

import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import { useBusy } from "@/progress"
import { createSearchSession } from "@/search"

const props = withDefaults(
  defineProps<{
    searchSession?: DeepReadonly<SearchSession> | SearchSession | null
  }>(),
  {
    searchSession: undefined,
  },
)

const $emit = defineEmits<{
  queryChange: [change: string]
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading and controls for data loading.
const busy = useBusy()

const abortController = new AbortController()

const searchQuery = ref("")

watchEffect(() => {
  if (abortController.signal.aborted) {
    return
  }

  if (!props.searchSession) {
    return
  }

  // We update the search query in one direction only when search session changes.
  searchQuery.value = props.searchSession.query || ""
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

  busy.value += 1
  try {
    await createSearchSession(router, searchQuery.value, abortController.signal, busy)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("NavBarSearch.onSubmit", err)
  } finally {
    busy.value -= 1
  }
}
</script>

<template>
  <!--
    display: contents so the input, the search button, and the teleported buttons participate
    directly in the navbar's flex layout rather than being shielded inside the form box. That lets the
    input shrink (min-w-0) and the search button compress to its floor and fade, the same as the other
    navbar buttons, instead of the navbar overflowing.
  -->
  <form id="navbarsearch-teleport-end" class="pd-navbar-search contents" novalidate @submit.prevent="onSubmit()">
    <InputText id="search-input-text" v-model="searchQuery" class="pd-searchinput max-w-xl min-w-0 grow" />
    <Button type="submit" primary>
      <MagnifyingGlassIcon class="size-5 sm:hidden" :alt="t('common.buttons.search')" />
      <span class="hidden sm:inline">{{ t("common.buttons.search") }}</span>
    </Button>
  </form>
</template>
