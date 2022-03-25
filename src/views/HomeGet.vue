<script setup lang="ts">
import { ref } from "vue"
import { useRouter } from "vue-router"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import { postSearch } from "@/search"

const router = useRouter()
const progress = ref(0)
const form = ref()

async function onSubmit() {
  await postSearch(router, progress, form.value)
}
</script>

<template>
  <form ref="form" :disabled="progress > 0" class="flex flex-grow flex-col" @submit.prevent="onSubmit">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">Wikipedia Search</h1>
    </div>
    <div class="flex justify-center">
      <InputText :progress="progress" name="q" class="mx-4 w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <Button :progress="progress" type="submit" class="mx-4">Search</Button>
    </div>
  </form>
  <Teleport to="footer">
    <div class="flex justify-between px-4 py-4 leading-none">
      <ul class="flex gap-x-4">
        <!-- <li><router-link :to="{ name: 'HomeGet' }" class="link">About</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Help</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Privacy</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Terms</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">API</router-link></li> -->
      </ul>
      <ul class="flex gap-x-4">
        <li class="text-neutral-500" title="build ">Powered by <a href="https://gitlab.com/peerdb/search" class="link">PeerDB Search</a></li>
      </ul>
    </div>
  </Teleport>
</template>
