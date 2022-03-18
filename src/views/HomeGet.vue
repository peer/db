<script setup lang="ts">
import { ref } from "vue"
import { useRouter } from "vue-router"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"

const router = useRouter()
const progress = ref(false)
const form = ref()

async function onSubmit() {
  progress.value = true
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentSearch",
      }).href,
      {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
        },
        // Have to cast to "any". See: https://github.com/microsoft/TypeScript/issues/30584
        body: new URLSearchParams(new FormData(form.value) as any),
        mode: "same-origin",
        credentials: "omit",
        redirect: "error",
        referrer: document.location.href,
        referrerPolicy: "strict-origin-when-cross-origin",
      },
    )
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    router.push({
      name: "DocumentSearch",
      query: await response.json(),
    })
  } finally {
    progress.value = false
  }
}
</script>

<template>
  <form id="xxx" ref="form" :readonly="progress" class="flex flex-grow flex-col" @submit.prevent="onSubmit">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">Wikipedia Search</h1>
    </div>
    <div class="flex justify-center">
      <InputText :progress="progress" name="q" />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <Button :progress="progress" type="submit">Search</Button>
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
