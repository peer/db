<script setup lang="ts">
import { ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { GlobeIcon } from "@heroicons/vue/outline"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { doSearch } from "@/search"

const route = useRoute()
const router = useRouter()
const progress = ref(false)
const form = ref()

async function onSubmit() {
  await doSearch(router, progress, form.value)
}
</script>

<template>
  <Teleport to="header">
    <div class="flex flex-grow gap-x-4 border-b border-slate-400 bg-slate-300 p-4 pl-0 shadow">
      <router-link :to="{ name: 'HomeGet' }" class="group -my-4 border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200">
        <GlobeIcon class="m-4 h-10 w-10 rounded group-focus:ring-2 group-focus:ring-primary-500" />
      </router-link>
      <form ref="form" :readonly="progress" class="flex flex-grow gap-x-4" @submit.prevent="onSubmit">
        <input type="hidden" name="s" :value="route.query.s" />
        <InputText :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.q" />
        <Button :progress="progress" type="submit" class="px-3.5">
          <SearchIcon class="h-5 w-5 sm:hidden" />
          <span class="hidden sm:inline">Search</span>
        </Button>
      </form>
    </div>
  </Teleport>
</template>
