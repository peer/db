<script setup lang="ts">
import type { Filter } from "@/types"

import { Identifier } from "@tozd/identifier"
import { onBeforeUnmount, onMounted } from "vue"
import { useRoute, useRouter } from "vue-router"

import { useProgress } from "@/progress"
import { createSearchSession } from "@/search"

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
    await createSearchSession(
      router,
      async (base) => {
        // Query parameters are interpreted as ref filters where key is the
        // property ID and value is the value ID, matching the backend behavior.
        // The "reverse" query parameter is special: it scopes the session to
        // documents referencing that ID via any property.
        const filters: Filter[] = []
        let reverse: string | undefined
        const query = route.query
        for (const [prop, values] of Object.entries(query)) {
          if (values == null) {
            continue
          }
          if (prop === "reverse") {
            const arr = Array.isArray(values) ? values : [values]
            const first = arr.find((v): v is string => v != null)
            if (first != null) {
              reverse = first
            }
            continue
          }
          const arr = Array.isArray(values) ? values : [values]
          const toValues = arr.filter((v): v is string => v != null).map((v) => ({ id: v }))
          if (toValues.length > 0) {
            const filterBase = [...base, "FILTER", Identifier.new().toString()]
            const id = (await Identifier.from(...filterBase)).toString()
            filters.push({
              id: id,
              base: filterBase,
              prop: [prop],
              ref: { to: toValues },
            })
          }
        }
        return {
          ...(filters.length > 0 ? { filters } : {}),
          ...(reverse ? { reverse } : {}),
        }
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
