<script setup lang="ts">
import { ref, watch, readonly } from "vue"
import { useRoute, useRouter } from "vue-router"
import { GlobeIcon } from "@heroicons/vue/outline"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import SearchResult from "@/components/SearchResult.vue"
import { postSearch, getSearch } from "@/search"
import { useNavbar } from "@/navbar"

const { ref: navbar, attrs: navbarAttrs } = useNavbar()

const route = useRoute()
const router = useRouter()
const form = ref()
const formProgress = ref(0)
const dataProgress = ref(0)
// See: https://github.com/vuejs/composition-api/issues/317
const dataProgressFn = () => dataProgress

const _results = ref()
const _total = ref(0)
const _moreThanTotal = ref(false)
const results = import.meta.env.DEV ? readonly(_results) : _results
const total = import.meta.env.DEV ? readonly(_total) : _total
const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal.value

async function onSubmit() {
  await postSearch(router, form.value, formProgress)
}

const initialRouteName = route.name
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
    // Watch can continue to run for some time after the route changes.
    if (initialRouteName !== route.name) {
      return
    }
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const data = await getSearch(router, query, dataProgress, controller.signal)
    if (data === null) {
      return
    }
    _results.value = data.results.slice(0, 50)
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
    <ProgressBar :progress="dataProgress" class="fixed inset-x-0 top-0 z-50 will-change-transform" />
    <div
      ref="navbar"
      class="z-30 flex w-full flex-grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow will-change-transform sm:gap-x-4 sm:p-4 sm:pl-0"
      v-bind="navbarAttrs"
    >
      <router-link :to="{ name: 'HomeGet' }" class="group -my-4 hidden border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200 sm:block">
        <GlobeIcon class="m-4 h-10 w-10 rounded group-focus:ring-2 group-focus:ring-primary-500" />
      </router-link>
      <form ref="form" :disabled="formProgress > 0" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
        <input type="hidden" name="s" :value="route.query.s" />
        <InputText :progress="formProgress" name="q" class="max-w-xl flex-grow" :value="route.query.q" />
        <Button :progress="formProgress" type="submit" class="px-3.5">
          <SearchIcon class="h-5 w-5 sm:hidden" />
          <span class="hidden sm:inline">Search</span>
        </Button>
      </form>
    </div>
  </Teleport>
  <div class="mt-12 flex flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <SearchResult v-for="result in results" :id="result._id" :key="result._id" :progress-fn="dataProgressFn" />
  </div>
</template>
