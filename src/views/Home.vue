<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
import { useRouter } from "vue-router"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import Footer from "@/components/Footer.vue"
import { postSearch } from "@/search"
import { injectProgress } from "@/progress"
import siteContext from "@/context"

const router = useRouter()

const progress = injectProgress()

const abortController = new AbortController()

const form = ref()

onBeforeUnmount(() => {
  abortController.abort()
})

async function onSubmit() {
  if (abortController.signal.aborted) {
    return
  }

  progress.value += 1
  try {
    await postSearch(router, form.value, abortController.signal, progress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("onSubmit", err)
  } finally {
    progress.value -= 1
  }
}
</script>

<template>
  <form ref="form" class="flex flex-grow flex-col" novalidate @submit.prevent="onSubmit">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">{{ siteContext.title }}</h1>
    </div>
    <div class="flex justify-center">
      <InputText name="q" class="mx-4 w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" :progress="progress" required />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <Button type="submit" class="mx-4" primary :progress="progress">Search</Button>
    </div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
