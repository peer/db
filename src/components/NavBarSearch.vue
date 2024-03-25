<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"
import { FunnelIcon } from "@heroicons/vue/20/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { postSearch } from "@/search"
import { injectProgress } from "@/progress"

const props = withDefaults(
  defineProps<{
    filtersEnabled?: boolean | null
  }>(),
  {
    filtersEnabled: null,
  },
)
const emit = defineEmits<{
  (e: "update:filtersEnabled", value: boolean): void
}>()

const route = useRoute()

const router = useRouter()

const abortController = new AbortController()

const progress = injectProgress()

const form = ref()

onBeforeUnmount(() => {
  abortController.abort()
})

async function onSubmit() {
  if (abortController.signal.aborted) {
    return
  }

  progress.value += 1
  try {
    await postSearch(router, form.value, abortController.signal, progress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onSubmit", err)
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
  <form ref="form" :disabled="progress > 0" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
    <InputText id="search-input-text" :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.s ? route.query.q : null" />
    <input v-if="route.query.s" type="hidden" name="s" :value="route.query.s" />
    <Button :progress="progress" type="submit" primary class="px-3.5">
      <MagnifyingGlassIcon class="h-5 w-5 sm:hidden" alt="Search" />
      <span class="hidden sm:inline">Search</span>
    </Button>
    <Button v-if="filtersEnabled != null" primary class="px-3.5 sm:hidden" type="button" @click="onFilters">
      <FunnelIcon class="h-5 w-5" alt="Filters" />
    </Button>
  </form>
</template>
