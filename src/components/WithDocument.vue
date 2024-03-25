<script setup lang="ts">
import type { PeerDBDocument } from "@/types"

import { ref, watch, readonly, onMounted, onUpdated, onUnmounted, getCurrentInstance } from "vue"
import { useRouter } from "vue-router"
import { getURL } from "@/api"
import { injectMainProgress } from "@/progress"

const props = defineProps<{
  id: string
}>()

const router = useRouter()

const mainProgress = injectMainProgress()

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
    // We want to eagerly remove any error.
    _error.value = null

    let data
    try {
      data = await getURL(newURL, el, controller.signal, mainProgress)
    } catch (err) {
      if (controller.signal.aborted) {
        return
      }
      console.error("WithDocument", newURL, err)
      _error.value = `${err}`
      return
    }
    _doc.value = data.doc as PeerDBDocument
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
  <slot v-else-if="error" name="error" :error="error" :url="url">
    <i class="text-error-600" :data-url="url">loading data failed</i>
  </slot>
  <slot v-else name="loading" :url="url"></slot>
</template>
