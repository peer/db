<script setup lang="ts">
import { ref } from "vue"
import { useRouter } from "vue-router"
import SearchResult from "@/components/SearchResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import NavBarSearch from "@/components/NavBarSearch.vue"
import { useSearch } from "@/search"

const router = useRouter()

const dataProgress = ref(0)
const { docs, total, moreThanTotal, hasMore, loadMore } = useSearch(dataProgress, async (query) => {
  await router.replace({
    name: "DocumentSearch",
    query,
  })
})
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="dataProgress">
      <NavBarSearch />
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <SearchResult v-for="doc in docs" :key="doc._id" :doc="doc" />
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
