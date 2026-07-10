<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { SearchSession } from "@/types"

import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"
import { nextTick, onBeforeUnmount, ref, useTemplateRef, watch, watchEffect } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import { useNavbarCollapse, useNavbarSearchQuery } from "@/navbar"
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

const { t, locale } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading and controls for data loading.
const busy = useBusy()

const abortController = new AbortController()

const searchQuery = ref("")

const navbarSearchQuery = useNavbarSearchQuery()

const formRef = useTemplateRef<HTMLFormElement>("formRef")

// The search shrinks to just its button when the navbar can no longer fit the input at its minimum usable
// width (pd-searchinput carries that min-width while inline, set in the stylesheet); the input only appears
// once the user expands it. useNavbarCollapse measures the room left for the input and toggles the
// pd-navbar-search-collapsible marker on the form, so the search collapses sooner when other items sit next
// to it (for example a filter toggle) and stays inline longer when the navbar is otherwise empty. It is
// suspended while expanded so the full-width overlay is left alone (focusout closes it).
const expanded = ref(false)
const collapsible = useNavbarCollapse(() => formRef.value, "pd-navbar-search-collapsible", () => expanded.value)

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

// We publish the current (possibly uncommitted) input value so that sibling navbar search shortcut
// buttons and the SearchGet view can submit it when a shortcut is clicked, the same query the search
// button submits via onSubmit.
watch(
  searchQuery,
  (query) => {
    navbarSearchQuery.value = query
  },
  { immediate: true, flush: "sync" },
)

onBeforeUnmount(() => {
  abortController.abort()
})

// While collapsed the button expands the input (and focuses it) instead of submitting; the browser's
// implicit form submission is suppressed for that first click. Once expanded (or when the search is not
// collapsible at all) the button behaves as a normal submit button.
function onButtonClick(event: MouseEvent): void {
  if (collapsible.value && !expanded.value) {
    event.preventDefault()
    expanded.value = true
    void nextTick(() => {
      formRef.value?.querySelector<HTMLInputElement>("input")?.focus()
    })
  }
}

// Moving focus out of the search (rather than to another element within it, such as the submit button)
// collapses the input again. This is a plain dismissal: it does not submit, so a stray tap elsewhere
// never triggers a search.
function onFocusOut(event: FocusEvent): void {
  if (!collapsible.value || !expanded.value) {
    return
  }
  const next = event.relatedTarget as Node | null
  if (next && formRef.value?.contains(next)) {
    return
  }
  expanded.value = false
}

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
    await createSearchSession(router, searchQuery.value, locale.value, abortController.signal, busy)
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
    input shrink (min-w-0) and grow, and the trailing buttons stay ordinary navbar flex items.

    When there is no longer room for the input the search is compact: only the button shows, and expanding
    it fills the whole navbar width (every other navbar item, including the menu, is hidden via the
    pd-navbar-search-expanded state). See the pd-navbar-search rules in the stylesheet.
  -->
  <form
    id="navbarsearch-teleport-end"
    ref="formRef"
    class="pd-navbar-search contents"
    :class="{ 'pd-navbar-search-collapsible': collapsible, 'pd-navbar-search-expanded': expanded }"
    novalidate
    @submit.prevent="onSubmit()"
    @focusout="onFocusOut"
  >
    <InputText id="search-input-text" v-model="searchQuery" class="pd-searchinput max-w-xl min-w-0 grow" />
    <Button type="submit" primary class="pd-navbar-search-button" @click="onButtonClick">
      <MagnifyingGlassIcon class="size-5 sm:hidden" :alt="t('common.buttons.search')" />
      <span class="hidden sm:inline">{{ t("common.buttons.search") }}</span>
    </Button>
  </form>
</template>
