<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue"
import { stringifyQuery, useRoute, useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { createShortcutSession } from "@/search"

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
    // Serialize the route's query back to a query string.
    await createShortcutSession(router, stringifyQuery(route.query), abortController.signal, progress)
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
