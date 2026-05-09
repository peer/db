<script setup lang="ts">
import type { FiltersState, RefFilterState } from "@/types"

import { onBeforeUnmount, onMounted } from "vue"
import { useRoute, useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { createSearchSession } from "@/search"

const route = useRoute()
const router = useRouter()

const progress = useProgress()

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

onMounted(async () => {
  if (abortController.signal.aborted) {
    return
  }

  // Query parameters are interpreted as ref filters where key is the
  // property ID and value is the value ID, matching the backend behavior.
  const refFilters: Record<string, RefFilterState> = {}
  const query = route.query
  for (const [prop, values] of Object.entries(query)) {
    if (values == null) {
      continue
    }
    const arr = Array.isArray(values) ? values : [values]
    refFilters[prop] = arr.filter((v): v is string => v != null)
  }

  const filters: FiltersState | undefined =
    Object.keys(refFilters).length > 0
      ? {
          ref: refFilters,
          amount: {},
          time: {},
        }
      : undefined

  progress.value += 1
  try {
    await createSearchSession(
      router,
      {
        query: "",
        filters,
      },
      abortController.signal,
      progress,
      true,
    )
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
