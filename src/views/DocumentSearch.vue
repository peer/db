<script setup lang="ts">
import { ref } from "vue"
import { useRouter, useRoute } from "vue-router"
import { SearchIcon } from "@heroicons/vue/solid"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import SearchResult from "@/components/SearchResult.vue"
import NavBar from "@/components/NavBar.vue"
import Footer from "@/components/Footer.vue"
import { postSearch, useSearch } from "@/search"

const route = useRoute()

const dataProgress = ref(0)
const { docs, total, moreThanTotal, hasMore, loadMore } = useSearch(dataProgress)

const router = useRouter()
const form = ref()
const formProgress = ref(0)

async function onSubmit() {
  await postSearch(router, form.value, formProgress)
}
</script>

<template>
  <Teleport to="header">
    <NavBar :progress="dataProgress">
      <form ref="form" :disabled="formProgress > 0" class="flex flex-grow gap-x-1 sm:gap-x-4" @submit.prevent="onSubmit">
        <input type="hidden" name="s" :value="route.query.s" />
        <InputText :progress="formProgress" name="q" class="max-w-xl flex-grow" :value="route.query.q" />
        <Button :progress="formProgress" type="submit" class="px-3.5">
          <SearchIcon class="h-5 w-5 sm:hidden" />
          <span class="hidden sm:inline">Search</span>
        </Button>
      </form>
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4">
    <SearchResult v-for="doc in docs" :key="doc._id" :doc="doc" />
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
