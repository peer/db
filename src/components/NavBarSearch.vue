<script setup lang="ts">
import { ref } from "vue"
import { useRouter, useRoute } from "vue-router"
import { MagnifyingGlassIcon } from "@heroicons/vue/20/solid"
import { FunnelIcon } from "@heroicons/vue/20/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { postSearch } from "@/search"

const props = withDefaults(
  defineProps<{
    filtersEnabled?: boolean | null
  }>(),
  { filtersEnabled: null },
)
const emit = defineEmits<{
  (e: "update:filtersEnabled", value: boolean): void
}>()

const route = useRoute()

const router = useRouter()
const form = ref()
const progress = ref(0)

async function onSubmit() {
  await postSearch(router, form.value, progress)
}

function onFilters() {
  emit("update:filtersEnabled", !props.filtersEnabled)
}
</script>

<template>
  <form ref="form" :disabled="progress > 0" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
    <InputText id="search-input-text" :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.s ? route.query.q : null" />
    <input v-if="route.query.s" type="hidden" name="s" :value="route.query.s" />
    <Button :progress="progress" type="submit" class="px-3.5">
      <MagnifyingGlassIcon class="h-5 w-5 sm:hidden" alt="Search" />
      <span class="hidden sm:inline">Search</span>
    </Button>
    <Button v-if="filtersEnabled != null" class="px-3.5 sm:hidden" type="button" @click="onFilters">
      <FunnelIcon class="h-5 w-5" alt="Filters" />
    </Button>
  </form>
</template>
