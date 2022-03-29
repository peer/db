<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from "vue"
import { useRoute, useRouter } from "vue-router"
import SearchResult from "@/components/SearchResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import Button from "@/components/Button.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import { useSearch } from "@/search"
import { useVisibilityTracking } from "@/visibility"

const router = useRouter()
const route = useRoute()

const dataProgress = ref(0)
const { docs, total, results, moreThanTotal, hasMore, loadMore } = useSearch(dataProgress, async (query) => {
  await router.replace({
    name: "DocumentSearch",
    // Maybe route.query has "at" parameter which we want to keep.
    query: { ...route.query, ...query },
  })
})

const idToIndex = computed(() => {
  const map = new Map<string, number>()
  for (const [i, doc] of docs.value.entries()) {
    map.set(doc._id, i)
  }
  return map
})

const { track, visibles } = useVisibilityTracking()

const initialRouteName = route.name
watch(
  () => {
    const sorted = Array.from(visibles)
    sorted.sort((a, b) => (idToIndex.value.get(a) ?? Infinity) - (idToIndex.value.get(b) ?? Infinity))
    return sorted[0]
  },
  async (topId) => {
    // Watch can continue to run for some time after the route changes.
    if (initialRouteName !== route.name) {
      return
    }
    // Initial data has not yet been loaded, so we wait.
    if (!topId && total.value < 0) {
      return
    }
    // We set "s", "at", and "q" here to undefined so that we control their order in the query string.
    const query: { s?: string; at?: string; q?: string } = { s: undefined, at: undefined, q: undefined, ...route.query }
    if (!topId) {
      delete query.at
    } else {
      query.at = topId
    }
    await router.replace({
      name: route.name as string,
      params: route.params,
      query: query,
      hash: route.hash,
    })
  },
  { immediate: true },
)

const moreButton = ref()
const supportPageOffset = window.pageYOffset !== undefined

function onScroll() {
  if (!moreButton.value) {
    return
  }

  const viewportHeight = document.documentElement.clientHeight || document.body.clientHeight
  const scrollHeight = Math.max(
    document.body.scrollHeight,
    document.documentElement.scrollHeight,
    document.body.offsetHeight,
    document.documentElement.offsetHeight,
    document.body.clientHeight,
    document.documentElement.clientHeight,
  )
  const currentScrollPosition = supportPageOffset ? window.pageYOffset : document.documentElement.scrollTop
  if (currentScrollPosition > scrollHeight - 2 * viewportHeight) {
    moreButton.value.$el.click()
  }
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
    <NavBar :progress="dataProgress">
      <NavBarSearch />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <div v-if="total === 0">
      <div class="my-1 sm:my-4">
        <div class="text-center text-sm">No results found.</div>
      </div>
    </div>
    <template v-else-if="total > 0">
      <template v-for="(doc, i) in docs" :key="doc._id">
        <div v-if="i === 0 && moreThanTotal" class="my-1 sm:my-4">
          <div class="text-center text-sm">Showing first {{ results.length }} of more than {{ total }} results found.</div>
          <div class="h-1 w-full bg-slate-200"></div>
        </div>
        <div v-if="i === 0 && results.length < total && !moreThanTotal" class="my-1 sm:my-4">
          <div class="text-center text-sm">Showing first {{ results.length }} of {{ total }} results found.</div>
          <div class="h-1 w-full bg-slate-200"></div>
        </div>
        <div v-if="i === 0 && results.length == total && !moreThanTotal" class="my-1 sm:my-4">
          <div class="text-center text-sm">Found {{ total }} results.</div>
          <div class="h-1 w-full bg-slate-200"></div>
        </div>
        <div v-else-if="i > 0 && i % 10 === 0" class="my-1 sm:my-4">
          <div v-if="results.length < total" class="text-center text-sm">{{ i }} of {{ results.length }} shown results.</div>
          <div v-else-if="results.length == total" class="text-center text-sm">{{ i }} of {{ results.length }} results.</div>
          <div class="relative h-1 w-full bg-slate-200">
            <div class="absolute inset-y-0 bg-secondary-400 opacity-60" style="left: 0" :style="{ width: (i / results.length) * 100 + '%' }"></div>
          </div>
        </div>
        <SearchResult :ref="(track(doc._id) as any)" :doc="doc" />
      </template>
      <Button v-if="hasMore" ref="moreButton" :progress="dataProgress" class="w-1/4 self-center" @click="loadMore">Load more</Button>
      <div v-else class="my-1 sm:my-4">
        <div v-if="moreThanTotal" class="text-center text-sm">All of first {{ results.length }} shown of more than {{ total }} results found.</div>
        <div v-else-if="results.length < total && !moreThanTotal" class="text-center text-sm">All of first {{ results.length }} shown of {{ total }} results found.</div>
        <div v-else-if="results.length == total && !moreThanTotal" class="text-center text-sm">All of {{ results.length }} results found.</div>
        <div class="relative h-1 w-full bg-slate-200">
          <div class="absolute inset-y-0 bg-secondary-400 opacity-60" style="left: 0" :style="{ width: 100 + '%' }"></div>
        </div>
      </div>
    </template>
  </div>
  <Teleport v-if="(total > 0 && !hasMore) || total === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
