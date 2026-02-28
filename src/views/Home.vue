<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import siteContext from "@/context"
import Footer from "@/partials/Footer.vue"
import { injectProgress } from "@/progress"
import { createSearchSession } from "@/search"

const { t } = useI18n()
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

  progress.value += 1
  try {
    await createSearchSession(
      router,
      {
        query: searchQuery.value,
      },
      abortController.signal,
      progress,
    )
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
  <form class="pd-home flex grow flex-col" novalidate @submit.prevent="onSubmit()">
    <div class="flex grow basis-0 flex-col justify-end">
      <img src="/logo.svg" :alt="siteContext.title" :title="siteContext.title" class="logo mb-10 h-48" />
    </div>
    <div class="flex flex-row justify-center gap-x-1 px-1 sm:gap-x-4 sm:px-4">
      <InputText id="home-input-search" v-model="searchQuery" class="pd-searchinput w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" :progress="progress" />
      <Button type="submit" primary :progress="progress">{{ t("common.buttons.search") }}</Button>
    </div>
    <div class="flex grow basis-0"></div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
