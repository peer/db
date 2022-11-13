<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, watch, readonly, onMounted, onUpdated, onUnmounted, getCurrentInstance } from "vue"
import { getDocument } from "@/api"
import { useRouter } from "@/utils"

const props = defineProps<{
  id: string
}>()

const router = useRouter()

const _doc = ref<PeerDBDocument | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : _doc
const _error = ref<string | null>(null)
const error = import.meta.env.DEV ? readonly(_error) : _error

const el = ref<HTMLElement | null>(null)

onMounted(() => {
  el.value = getCurrentInstance()?.proxy?.$el
})

onUnmounted(() => {
  el.value = null
})

onUpdated(() => {
  const el = getCurrentInstance()?.proxy?.$el
  if (el !== el.value) {
    el.value = el
  }
})

watch(
  () => props.id,
  async (id, oldId, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())

    try {
      _doc.value = await getDocument(router, id, el, controller.signal)
      _error.value = null
    } catch (err) {
      if (controller.signal.aborted) {
        return
      }
      _doc.value = null
      _error.value = `${err}`
    }
  },
  {
    immediate: true,
  },
)

defineExpose({
  doc,
  error,
})
</script>

<template>
  <slot v-if="doc" :doc="doc"></slot>
  <slot v-else-if="error" :error="error" name="error"></slot>
  <slot v-else name="loading"></slot>
</template>
