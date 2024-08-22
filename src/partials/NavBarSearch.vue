<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"
import { SparklesIcon } from "@heroicons/vue/20/solid"
import { FunnelIcon } from "@heroicons/vue/20/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { postSearch } from "@/search"
import { injectProgress } from "@/progress"

const props = withDefaults(
  defineProps<{
    s?: string
    filtersEnabled?: boolean | null
  }>(),
  {
    s: "",
    filtersEnabled: null,
  },
)
const emit = defineEmits<{
  (e: "update:filtersEnabled", value: boolean): void
}>()

const route = useRoute()

const router = useRouter()

const progress = injectProgress()

const abortController = new AbortController()

const searchQuery = ref("")

onBeforeUnmount(() => {
  abortController.abort()
})

async function onSubmit(isPrompt: boolean) {
  if (abortController.signal.aborted) {
    return
  }

  const form = new FormData();
  if (isPrompt) {
    form.set("p", searchQuery.value)
  } else {
    form.set("q", searchQuery.value)
  }

  progress.value += 1
  try {
    await postSearch(router, form, abortController.signal, progress)
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

function onFilters() {
  if (abortController.signal.aborted) {
    return
  }

  emit("update:filtersEnabled", !props.filtersEnabled)
}
</script>

<template>
  <form class="flex flex-grow gap-x-1 sm:gap-x-4" novalidate @submit.prevent="onSubmit(!!route.query.p)">
    <InputText
      id="search-input-text"
      v-model="searchQuery"
      :progress="progress"
      class="max-w-xl flex-grow"
    />
    <Button :progress="progress" type="button" primary class="!px-3.5" @click="onSubmit(false)">
      <MagnifyingGlassIcon class="h-5 w-5 sm:hidden" alt="Search" />
      <span class="hidden sm:inline">Search</span>
    </Button>
    <Button :progress="progress" type="button" primary class="!px-3.5" @click="onSubmit(true)">
      <SparklesIcon class="h-5 w-5 sm:hidden" alt="Prompt" />
      <span class="hidden sm:inline">Prompt</span>
    </Button>
    <Button v-if="filtersEnabled != null" primary class="!px-3.5 sm:hidden" type="button" @click="onFilters">
      <FunnelIcon class="h-5 w-5" alt="Filters" />
    </Button>
  </form>
</template>
