<script setup lang="ts" generic="T">
import type { Metadata } from "@/types"

import { computed, DeepReadonly, getCurrentInstance, onMounted, onUnmounted, onUpdated, readonly, ref, Ref, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { FetchError, getURL } from "@/api"
import { getRootProgress } from "@/progress"
import { encodeQuery } from "@/utils"

const props = defineProps<{
  id: string
  name: string
  // When set, the document is fetched at this version ("changeset-revision" or "changeset")
  // instead of the latest one. Absent means the latest version.
  version?: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// We use root progress for loading data.
const rootProgress = getRootProgress()

const _doc = ref<T | null>(null)
const _metadata = ref<Metadata>({})
const _error = ref<string | null>(null)
// accessDenied marks that the document exists but is not available to the caller (HTTP 403), an expected
// outcome (for example a document hidden at the caller's visibility level), not a loading failure. It is
// surfaced through the error slot via the message and accessDenied slot props.
const _accessDenied = ref(false)
const _url = ref<string | null>(null)
const doc = process.env.NODE_ENV !== "production" ? readonly(_doc) : (_doc as DeepReadonly<Ref<T | null>>)
const metadata = process.env.NODE_ENV !== "production" ? readonly(_metadata) : (_metadata as DeepReadonly<Ref<Metadata>>)
const error = process.env.NODE_ENV !== "production" ? readonly(_error) : _error
const accessDenied = process.env.NODE_ENV !== "production" ? readonly(_accessDenied) : _accessDenied
const url = process.env.NODE_ENV !== "production" ? readonly(_url) : _url

// message is the user-facing, translated text for the error slot: the document is not available to the caller
// on access denied (HTTP 403), or a generic load failure otherwise.
const message = computed(() => (_accessDenied.value ? t("common.status.dataNotAvailable") : t("common.status.loadingDataFailed")))

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
  () => ({ id: props.id, name: props.name, version: props.version }),
  async (params, oldParams, onCleanup) => {
    const abortController = new AbortController()
    onCleanup(() => abortController.abort())

    const newURL = router.apiResolve({
      name: params.name,
      params: {
        id: params.id,
      },
      query: encodeQuery({ version: params.version }),
    }).href
    _url.value = newURL

    // We want to eagerly remove any old doc and show loading again.
    _doc.value = null
    _metadata.value = {}
    // We want to eagerly remove any error.
    _error.value = null
    _accessDenied.value = false

    try {
      const response = await getURL<T>(newURL, el, abortController.signal, rootProgress)
      if (abortController.signal.aborted) {
        return
      }

      _doc.value = response.doc
      _metadata.value = response.metadata
    } catch (err) {
      if (abortController.signal.aborted) {
        return
      }
      if (err instanceof FetchError && err.status === 403) {
        // The document is not available to the caller (access denied), an expected state, not a failure.
        _accessDenied.value = true
        return
      }
      console.error("WithDocument", err)
      // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
      _error.value = `${err}`
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
  accessDenied,
  url,
})

defineSlots<{
  default(props: { doc: DeepReadonly<T>; metadata: DeepReadonly<Metadata>; url: string }): unknown
  error(props: { error: string | null; message: string; accessDenied: boolean; url: string | null }): unknown
  loading(props: { url: string | null }): unknown
}>()
</script>

<template>
  <slot v-if="doc" :doc="doc as DeepReadonly<T>" :metadata="metadata" :url="url!"></slot>
  <slot v-else-if="error || accessDenied" name="error" :error="error" :message="message" :access-denied="accessDenied" :url="url">
    <i :class="['pd-withdocument-error', accessDenied ? 'text-gray-500' : 'text-error-600']" :data-url="url">{{ message }}</i>
  </slot>
  <slot v-else name="loading" :url="url"></slot>
</template>
