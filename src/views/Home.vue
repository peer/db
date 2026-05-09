<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import InputText from "@/components/InputText.vue"
import siteContext from "@/context"
import Footer from "@/partials/Footer.vue"
import HomeNavBar from "@/partials/HomeNavBar.vue"
import { useProgress } from "@/progress"
import { getHomeComponent } from "@/registry/home"
import { createSearchSession } from "@/search"

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const progress = useProgress()

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
      false,
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

const homeComponent = getHomeComponent()
</script>

<template>
  <Teleport to="header">
    <HomeNavBar />
  </Teleport>
  <form class="pd-home flex grow flex-col" novalidate @submit.prevent="onSubmit()">
    <div class="flex grow basis-0 flex-col justify-end">
      <!--
        We use here "w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" which is the same the input box below it,
        but because we do not add also width of the button next to it, we can make logo max-w-full and get
        it slightly smaller than the input box, which is what we want (~80% of the input box width).
      -->
      <RouterLink :to="{ name: 'SearchShortcut' }" class="mb-10 flex w-full max-w-2xl justify-center self-center p-4 sm:w-4/5 md:w-2/3 lg:w-1/2">
        <img v-if="siteContext.logo" :src="siteContext.logo" :alt="siteContext.title" :title="siteContext.title" class="logo max-h-48 max-w-full" />
        <h1 v-else class="text-5xl font-bold">{{ siteContext.title }}</h1>
      </RouterLink>
    </div>
    <div class="flex flex-row justify-center gap-x-1 px-1 sm:gap-x-4 sm:px-4">
      <InputText id="home-input-search" v-model="searchQuery" class="pd-searchinput w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" :progress="progress" />
      <Button id="home-button-search" type="submit" primary :progress="progress">{{ t("common.buttons.search") }}</Button>
    </div>
    <div class="flex grow basis-0"><component :is="homeComponent" v-if="homeComponent" /></div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
