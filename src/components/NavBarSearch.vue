<script setup lang="ts">
import { ref } from "vue"
import { useRouter, useRoute } from "vue-router"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { postSearch } from "@/search"

const route = useRoute()

const router = useRouter()
const form = ref()
const progress = ref(0)

async function onSubmit() {
  await postSearch(router, form.value, progress)
}
</script>

<template>
  <form ref="form" :disabled="progress > 0" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
    <input v-if="route.query.s" type="hidden" name="s" :value="route.query.s" />
    <InputText :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.s ? route.query.q : null" />
    <Button :progress="progress" type="submit" class="px-3.5">
      <SearchIcon class="h-5 w-5 sm:hidden" alt="Search" />
      <span class="hidden sm:inline">Search</span>
    </Button>
  </form>
</template>
