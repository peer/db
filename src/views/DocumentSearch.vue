<script setup lang="ts">
import { ref, watch, readonly } from "vue"
import { useRoute, useRouter } from "vue-router"
import { GlobeIcon } from "@heroicons/vue/outline"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { makeSearch, doSearch } from "@/search"

const route = useRoute()
const router = useRouter()
const progress = ref(0)
const form = ref()
const _results = ref()
const _total = ref(0)
const _moreThanTotal = ref(false)
const results = readonly(_results)
const total = readonly(_total)
const moreThanTotal = readonly(_moreThanTotal)

async function onSubmit() {
  await makeSearch(router, progress, form.value)
}

watch(
  () => {
    let q: string
    let s: string
    if (Array.isArray(route.query.q)) {
      q = route.query.q[0] || ""
    } else {
      q = route.query.q || ""
    }
    if (Array.isArray(route.query.s)) {
      s = route.query.s[0] || ""
    } else {
      s = route.query.s || ""
    }
    return new URLSearchParams({ q, s }).toString()
  },
  async (query, oldQuery, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const data = await doSearch(router, progress, query, controller.signal)
    if (data === null) {
      return
    }
    _results.value = data.results
    if (data.total.endsWith("+")) {
      _moreThanTotal.value = true
      _total.value = parseInt(data.total.substring(0, data.total.length - 2))
    } else {
      _moreThanTotal.value = false
      _total.value = parseInt(data.total)
    }
  },
  {
    immediate: true,
  },
)
</script>

<template>
  <Teleport to="header">
    <div class="flex flex-grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow sm:gap-x-4 sm:p-4 sm:pl-0">
      <router-link :to="{ name: 'HomeGet' }" class="group -my-4 hidden border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200 sm:block">
        <GlobeIcon class="m-4 h-10 w-10 rounded group-focus:ring-2 group-focus:ring-primary-500" />
      </router-link>
      <form ref="form" :readonly="progress" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
        <input type="hidden" name="s" :value="route.query.s" />
        <InputText :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.q" />
        <Button :progress="progress" type="submit" class="px-3.5">
          <SearchIcon class="h-5 w-5 sm:hidden" />
          <span class="hidden sm:inline">Search</span>
        </Button>
      </form>
    </div>
  </Teleport>
  <div class="flex flex-col">
    <div v-for="result in results" :key="result._id">a</div>
  </div>
</template>
