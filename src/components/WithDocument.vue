<script setup lang="ts" generic="T">
import type { Metadata } from "@/types"

import { DeepReadonly, getCurrentInstance, onMounted, onUnmounted, onUpdated, readonly, ref, Ref, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import { injectMainProgress } from "@/progress"

const props = defineProps<{
  id: string
  name: string
}>()

const { t } = useI18n()
const router = useRouter()

const mainProgress = injectMainProgress()

const _doc = ref<T | null>(null)
const _metadata = ref<Metadata>({})
const _error = ref<string | null>(null)
const _url = ref<string | null>(null)
const doc = import.meta.env.DEV ? readonly(_doc) : (_doc as DeepReadonly<Ref<T | null>>)
const metadata = import.meta.env.DEV ? readonly(_metadata) : (_metadata as DeepReadonly<Ref<Metadata>>)
const error = import.meta.env.DEV ? readonly(_error) : _error
const url = import.meta.env.DEV ? readonly(_url) : _url

const el = ref<HTMLElement | null>(null)

onMounted(() => {
  // TODO: Make sure $el is really a HTMLElement and not for example a text node.
  //       We can search for the first sibling element? Or element with data-url attribute.
  el.value = getCurrentInstance()?.proxy?.$el
})

onUnmounted(() => {
  el.value = null
})

onUpdated(() => {
  // TODO: Make sure $el is really a HTMLElement and not for example a text node.
  //       We can search for the first sibling element? Or element with data-url attribute.
  const el = getCurrentInstance()?.proxy?.$el
  if (el !== el.value) {
    el.value = el
  }
})

watch(
  () => ({ id: props.id, name: props.name }),
  async (params, oldParams, onCleanup) => {
    const abortController = new AbortController()
    onCleanup(() => abortController.abort())

    const newURL = router.apiResolve({
      name: params.name,
      params: {
        id: params.id,
      },
    }).href
    _url.value = newURL

    // We want to eagerly remove any old doc and show loading again.
    _doc.value = null
    _metadata.value = {}
    // We want to eagerly remove any error.
    _error.value = null

    try {
      const response = await getURL<T>(newURL, el, abortController.signal, mainProgress)
      if (abortController.signal.aborted) {
        return
      }

      _doc.value = response.doc
      _metadata.value = response.metadata
    } catch (error) {
      if (abortController.signal.aborted) {
        return
      }
      console.error("WithDocument", error)
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      _error.value = `${error}`
      return
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

defineSlots<{
  default(props: { doc: DeepReadonly<T>; metadata: DeepReadonly<Metadata>; url: string }): unknown
  error(props: { error: string; url: string | null }): unknown
  loading(props: { url: string | null }): unknown
}>()
</script>

<template>
  <slot v-if="doc" :doc="doc as DeepReadonly<T>" :metadata="metadata" :url="url!"></slot>
  <slot v-else-if="error" name="error" :error="error" :url="url">
    <i class="text-error-600" :data-url="url">{{ t("common.status.loadingDataFailed") }}</i>
  </slot>
  <slot v-else name="loading" :url="url"></slot>
</template>
