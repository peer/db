<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { createShortcutSession } from "@/search"

const { locale } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()

// Data loading only, no controls.
const progress = useProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

onMounted(async () => {
  if (abortController.signal.aborted) {
    return
  }

  progress.value += 1
  try {
    await createShortcutSession(router, route.query, locale.value, abortController.signal, progress)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("SearchShortcut", err)
  } finally {
    progress.value -= 1
  }
})
</script>

<template>
  <div></div>
</template>
