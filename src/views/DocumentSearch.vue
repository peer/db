<script setup lang="ts">
import { ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { doSearch } from "@/search"

const route = useRoute()
const router = useRouter()
const progress = ref(false)
const form = ref()

async function onSubmit() {
  await doSearch(router, progress, form.value)
}
</script>

<template>
  <Teleport to="header">
    <div class="flex flex-grow border-b border-slate-400 bg-slate-300 py-1 px-4 shadow">
      <form ref="form" :readonly="progress" class="flex flex-grow gap-x-1" @submit.prevent="onSubmit">
        <input type="hidden" name="s" :value="route.query.s" />
        <InputText :progress="progress" name="q" class="max-w-xl flex-grow" :value="route.query.q" />
        <Button :progress="progress" type="submit">Search</Button>
      </form>
    </div>
  </Teleport>
</template>
