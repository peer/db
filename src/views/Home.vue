<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
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

async function onSubmit(isPrompt: boolean) {
  if (abortController.signal.aborted) {
    return
  }

  const form = new FormData()
  if (isPrompt) {
    form.set("p", searchQuery.value)
  } else {
    form.set("q", searchQuery.value)
  }

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
  <form class="flex flex-grow flex-col" novalidate @submit.prevent="onSubmit(false)">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">{{ siteContext.title }}</h1>
    </div>
    <div class="flex justify-center">
      <InputText v-model="searchQuery" class="mx-4 w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" :progress="progress" tabindex="1" />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <Button type="button" class="mx-4" primary tabindex="3" :progress="progress" @click="onSubmit(false)">Search</Button>
      <Button type="button" class="mx-4" primary tabindex="2" :progress="progress" :disabled="searchQuery.length === 0" @click="onSubmit(true)">Prompt</Button>
    </div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
