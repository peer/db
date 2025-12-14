<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useRouter } from "vue-router"

import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import Footer from "@/partials/Footer.vue"
import { postSearch } from "@/search"
import { injectProgress } from "@/progress"
import siteContext from "@/context"

const router = useRouter()

const progress = injectProgress()

const abortController = new AbortController()

const searchQuery = ref("")

onBeforeUnmount(() => {
  abortController.abort()
})

onMounted(() => {
  document.getElementById("home-input-search")?.focus()
})

async function onSubmit() {
  if (abortController.signal.aborted) {
    return
  }

  const form = new FormData()
  form.set("q", searchQuery.value)

  progress.value += 1
  try {
    await postSearch(router, form, abortController.signal, progress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("Home.onSubmit", err)
  } finally {
    progress.value -= 1
  }
}
</script>

<template>
  <form class="flex flex-grow flex-col" novalidate @submit.prevent="onSubmit()">
    <div class="flex flex-grow flex-col basis-0 justify-end">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">{{ siteContext.title }}</h1>
    </div>
    <div class="flex flex-row justify-center gap-x-1 sm:gap-x-4 px-1 sm:px-4">
      <InputText id="home-input-search" v-model="searchQuery" class="w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" :progress="progress" />
      <Button type="submit" primary :progress="progress">Search</Button>
    </div>
    <div class="flex flex-grow basis-0">
    </div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
