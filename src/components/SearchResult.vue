<script setup lang="ts">
import { ref, readonly, onBeforeUnmount } from "vue"
import { useRouter } from "vue-router"
import { getDocument } from "@/search"

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  // See: https://github.com/vuejs/composition-api/issues/317
  progressFn: {
    type: Function,
    required: true,
  },
})

const _doc = ref()
const doc = import.meta.env.DEV ? readonly(_doc) : _doc

const router = useRouter()
const controller = new AbortController()
onBeforeUnmount(() => controller.abort())
getDocument(router, props.id, props.progressFn(), controller.signal).then((data) => {
  _doc.value = data
})
</script>

<template>
  <div>a {{ id }} {{ doc }}</div>
</template>
