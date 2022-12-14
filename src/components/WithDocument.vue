<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, watch, readonly, onMounted, onUpdated, onUnmounted, getCurrentInstance } from "vue"
import { getURL } from "@/api"
import { useRouter } from "@/utils"

const props = defineProps<{
  id: string
}>()

const router = useRouter()

const _doc = ref<PeerDBDocument | null>(null)
const _error = ref<string | null>(null)
const _url = ref<string | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : _doc
const error = import.meta.env.DEV ? readonly(_error) : _error
const url = import.meta.env.DEV ? readonly(_url) : _url

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

    const newURL = router.apiResolve({
      name: "DocumentGet",
      params: {
        id,
      },
    }).href
    _url.value = newURL

    // We want to eagerly remove any old doc and show loading again.
    _doc.value = null
    _error.value = null

    try {
      const res = await getURL(newURL, el, controller.signal)
      _doc.value = res.doc as PeerDBDocument
    } catch (err) {
      if (controller.signal.aborted) {
        return
      }
      console.error("WithDocument", id, err)
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
  url,
})
</script>

<template>
  <slot v-if="doc" :doc="doc" :url="url"></slot>
  <slot v-else-if="error" name="error" :error="error" :url="url"></slot>
  <slot v-else name="loading" :url="url"></slot>
</template>
