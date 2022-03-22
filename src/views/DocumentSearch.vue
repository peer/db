<script setup lang="ts">
import { ref, watch, readonly, onMounted, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import { GlobeIcon } from "@heroicons/vue/outline"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import { makeSearch, doSearch } from "@/search"

const route = useRoute()
const router = useRouter()
const formProgress = ref(0)
const dataProgress = ref(0)
const form = ref()

const _results = ref()
const _total = ref(0)
const _moreThanTotal = ref(false)
const results = import.meta.env.DEV ? readonly(_results) : _results
const total = import.meta.env.DEV ? readonly(_total) : _total
const moreThanTotal = import.meta.env.DEV ? readonly(_moreThanTotal) : _moreThanTotal.value

async function onSubmit() {
  await makeSearch(router, formProgress, form.value)
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
    const data = await doSearch(router, dataProgress, query, controller.signal)
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

const navbar = ref()
const position = ref("absolute")
const navbarTop = ref(0)
let lastScrollPosition = 0
const animateNavbar = ref(false)
const supportPageOffset = window.pageYOffset !== undefined

function onScroll() {
  const currentScrollPosition = supportPageOffset ? window.pageYOffset : document.documentElement.scrollTop
  if (currentScrollPosition <= 0) {
    position.value = "absolute"
    navbarTop.value = 0
    lastScrollPosition = 0
    return
  }

  if (currentScrollPosition > lastScrollPosition) {
    if (position.value !== "absolute") {
      animateNavbar.value = false
      let { top } = navbar.value.getBoundingClientRect()
      position.value = "absolute"
      if (currentScrollPosition - lastScrollPosition < 10) {
        // Scroll speed is small enough for lastScrollPosition to be probably a better value
        // so that navbar appears at the location where the user started scrolling.
        navbarTop.value = lastScrollPosition + top
      } else {
        navbarTop.value = currentScrollPosition + top
      }
    }
  } else if (currentScrollPosition < lastScrollPosition) {
    if (position.value !== "fixed") {
      const { top, height } = navbar.value.getBoundingClientRect()
      if (top >= 0) {
        navbarTop.value = 0
        position.value = "fixed"
      } else if (top < -height) {
        if (lastScrollPosition - currentScrollPosition > 10) {
          // Scroll speed is large so we just do the animation instead.
          navbarTop.value = 0
          position.value = "fixed"
          animateNavbar.value = true
        } else {
          navbarTop.value = currentScrollPosition - height
        }
      }
    }
  }

  lastScrollPosition = currentScrollPosition
}

onMounted(() => {
  window.addEventListener("scroll", onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", onScroll)
})
</script>

<template>
  <Teleport to="header">
    <ProgressBar :progress="dataProgress" class="fixed inset-x-0 top-0 z-50 will-change-transform" />
    <div
      ref="navbar"
      class="z-30 flex w-full flex-grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow will-change-transform sm:gap-x-4 sm:p-4 sm:pl-0"
      :class="{ 'animate-navbar': animateNavbar }"
      :style="{ position: position, top: navbarTop + 'px' }"
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
  <div class="mt-12 flex flex-col border-t border-transparent sm:mt-[4.5rem]">
    <div v-for="result in results" :key="result._id">a</div>
  </div>
</template>
