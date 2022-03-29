<script setup lang="ts">
import { ref, computed, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import SearchResult from "@/components/SearchResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import { useSearch } from "@/search"
import { useVisibilityTracking } from "@/visibility"

const router = useRouter()
const route = useRoute()

const dataProgress = ref(0)
const { docs, total, moreThanTotal, hasMore, loadMore } = useSearch(dataProgress, async (query) => {
  await router.replace({
    name: "DocumentSearch",
    query,
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
    <template v-for="(doc, i) in docs" :key="doc._id">
      <div v-if="i === 0 && moreThanTotal" class="my-1 sm:my-4">
        <div class="text-center text-sm">Found more than {{ total }} results.</div>
        <div class="h-1 w-full bg-slate-200"></div>
      </div>
      <div v-else-if="i === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">Found {{ total }} results.</div>
        <div class="h-1 w-full bg-slate-200"></div>
      </div>
      <div v-else-if="i % 10 === 0" class="my-1 sm:my-4">
        <div class="text-center text-sm">{{ i }} of {{ total }} results.</div>
        <div class="relative h-1 w-full bg-slate-200">
          <div class="absolute inset-y-0 bg-secondary-400 opacity-60" style="left: 0" :style="{ width: (i / total) * 100 + '%' }"></div>
        </div>
      </div>
      <SearchResult :ref="(track(doc._id) as any)" :doc="doc" />
    </template>
  </div>
  <Teleport v-if="(total > 0 && !hasMore) || total === 0" to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
